// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package pppoe

import (
	"bytes"
	"net"
	"time"

	"github.com/veesix-networks/osvbng/internal/ra"
	"github.com/veesix-networks/osvbng/pkg/allocator"
	"github.com/veesix-networks/osvbng/pkg/config/ip"
	"github.com/veesix-networks/osvbng/pkg/dhcp"
	"github.com/veesix-networks/osvbng/pkg/dhcp6"
	"github.com/veesix-networks/osvbng/pkg/ppp"
)

// handleDHCPv6 receives an in-band DHCPv6 datagram (UDP/547) from the PPP 0x0057
// path. It runs under s.mu (held by handlePPP): it records the client DUID and
// reply destination, then hands the provider I/O to a bounded worker so the
// upstream relay/proxy or lease locks never run on the receive path under the
// session lock. If no DHCPv6 provider is configured, the packet is dropped
// (never Protocol-Rejected).
func (s *SessionState) handleDHCPv6(peer net.IP, payload []byte) error {
	c := s.component
	if len(c.dhcp6Providers) == 0 {
		return nil
	}
	msg, err := dhcp6.ParseMessage(payload)
	if err != nil {
		return nil
	}
	if len(s.DHCPv6DUID) != 0 && len(msg.Options.ClientID) != 0 &&
		!bytes.Equal(s.DHCPv6DUID, msg.Options.ClientID) {
		c.logger.Warn("DHCPv6 client DUID changed on established session, ignoring",
			"session_id", s.SessionID)
		return nil
	}
	if len(msg.Options.ClientID) != 0 {
		s.DHCPv6DUID = append([]byte(nil), msg.Options.ClientID...)
	}
	if len(peer) != 0 {
		s.ClientLinkLocal = peer
	}
	c.dispatchDHCPv6(s, msg)
	return nil
}

// dispatchDHCPv6 runs the provider exchange off the session lock on a bounded
// worker, dropping (with a log) when the pool is saturated rather than blocking
// the receive path or spawning an unbounded number of goroutines.
func (c *Component) dispatchDHCPv6(s *SessionState, msg *dhcp6.Message) {
	select {
	case c.dhcp6Sem <- struct{}{}:
	default:
		c.logger.Warn("DHCPv6 worker pool saturated, dropping packet", "session_id", s.SessionID)
		return
	}
	c.Go(func() {
		defer func() { <-c.dhcp6Sem }()
		c.forwardDHCPv6(s, msg)
	})
}

func (c *Component) forwardDHCPv6(s *SessionState, msg *dhcp6.Message) {
	s.mu.Lock()
	allocCtx := s.AllocCtx
	swIdx := s.SwIfIndex
	encap := s.EncapIfIndex
	mac := s.MAC.String()
	svlan, cvlan := s.OuterVLAN, s.InnerVLAN
	duid := s.DHCPv6DUID
	clientLL := s.ClientLinkLocal
	open := s.ipv6cpOpen
	phase := s.Phase
	s.mu.Unlock()

	if !open || phase == ppp.PhaseLACTunneled {
		return
	}

	profile := c.resolveIPv6Profile(allocCtx)
	provider := c.getDHCP6Provider(profile)
	if provider == nil {
		return
	}

	bngMAC, _ := s.bngSourceMAC()

	// local/server mode needs the profile-resolved pools; relay/proxy mode
	// builds its own Relay-Forward and takes Profile/PeerAddr instead.
	var resolved *dhcp.ResolvedDHCPv6
	if mode := profileMode(profile); mode == "" || mode == "server" || mode == "local" {
		resolved = c.resolveDHCPv6(allocCtx)
	}

	var ifaceName string
	if c.ifMgr != nil {
		if iface := c.ifMgr.Get(encap); iface != nil {
			ifaceName = iface.Name
		}
	}

	pkt := &dhcp6.Packet{
		SessionID: s.SessionID,
		MAC:       mac,
		SVLAN:     svlan,
		CVLAN:     cvlan,
		DUID:      duid,
		Raw:       msg.Raw,
		Resolved:  resolved,
		SwIfIndex: encap,
		Interface: ifaceName,
		PeerAddr:  clientLL,
		Profile:   profile,
		LocalMAC:  bngMAC,
	}

	response, err := provider.HandlePacket(c.Ctx, pkt)
	if err != nil {
		c.logger.Warn("DHCPv6 provider failed", "session_id", s.SessionID, "error", err)
		return
	}
	if response == nil || len(response.Raw) == 0 {
		return
	}

	c.sendDHCPv6Reply(s, bngMAC, clientLL, response.Raw)

	// Only a Reply binds or unbinds; the Advertise to a Solicit is egress only.
	switch msg.MsgType {
	case dhcp6.MsgTypeRequest, dhcp6.MsgTypeRenew, dhcp6.MsgTypeRebind:
		if rm, _ := dhcp6.ParseMessage(response.Raw); rm != nil && rm.MsgType == dhcp6.MsgTypeReply {
			c.bindDHCPv6(s, swIdx, rm)
		}
	case dhcp6.MsgTypeRelease, dhcp6.MsgTypeDecline:
		c.unbindDHCPv6(s, swIdx)
	}
}

// sendDHCPv6Reply wraps the provider's response in IPv6/UDP (server 547 ->
// client 546) sourced from the BNG link-local and sends it over the session.
func (c *Component) sendDHCPv6Reply(s *SessionState, bngMAC net.HardwareAddr, clientLL net.IP, raw []byte) {
	if bngMAC == nil || len(clientLL) == 0 {
		return
	}
	frame := dhcp.BuildIPv6UDPFrame(ra.LinkLocalFromMAC(bngMAC), clientLL, 547, 546, raw)
	if frame == nil {
		return
	}
	s.sendIPv6Packet(frame)
}

// bindDHCPv6 records the IA-NA / IA-PD from a Reply and programs the dataplane.
// If the address or prefix changed, the old route is removed before the new one
// is installed.
func (c *Component) bindDHCPv6(s *SessionState, swIdx uint32, msg *dhcp6.Message) {
	var iana net.IP
	var pd *net.IPNet
	var lease uint32
	if msg.Options.IANA != nil && msg.Options.IANA.Address != nil {
		iana = msg.Options.IANA.Address
		lease = msg.Options.IANA.ValidTime
	}
	if msg.Options.IAPD != nil && msg.Options.IAPD.Prefix != nil {
		pd = &net.IPNet{
			IP:   msg.Options.IAPD.Prefix,
			Mask: net.CIDRMask(int(msg.Options.IAPD.PrefixLen), 128),
		}
	}

	s.mu.Lock()
	oldAddr := s.IPv6Address
	oldPrefix := s.IPv6Prefix
	s.IPv6Address = iana
	s.IPv6Prefix = pd
	s.IPv6LeaseTime = lease
	s.IPv6BoundAt = time.Now()
	s.mu.Unlock()

	if c.vpp != nil && swIdx != 0 {
		if oldAddr != nil && !oldAddr.Equal(iana) {
			c.vpp.PPPoESetSessionIPv6Async(swIdx, oldAddr, false, func(error) {})
		}
		if iana != nil {
			c.vpp.PPPoESetSessionIPv6Async(swIdx, iana, true, func(err error) {
				if err != nil {
					c.logger.Debug("Failed to bind IPv6 to PPPoE session", "session_id", s.SessionID, "error", err)
				}
			})
		}
		if oldPrefix != nil && (pd == nil || !oldPrefix.IP.Equal(pd.IP)) {
			c.vpp.PPPoESetDelegatedPrefixAsync(swIdx, *oldPrefix, net.IPv6zero, false, func(error) {})
		}
		if pd != nil {
			nextHop := iana
			if nextHop == nil {
				nextHop = net.IPv6zero
			}
			c.vpp.PPPoESetDelegatedPrefixAsync(swIdx, *pd, nextHop, true, func(err error) {
				if err != nil {
					c.logger.Debug("Failed to set delegated prefix on PPPoE session", "session_id", s.SessionID, "error", err)
				}
			})
		}
	}

	c.checkpointSession(s)
	c.logger.Debug("PPPoE DHCPv6 bound", "session_id", s.SessionID, "ipv6", iana, "prefix", pd)
}

// unbindDHCPv6 clears the IA-NA / IA-PD on a Release/Decline, frees the allocator
// reservations, and removes the dataplane routes.
func (c *Component) unbindDHCPv6(s *SessionState, swIdx uint32) {
	s.mu.Lock()
	addr := s.IPv6Address
	prefix := s.IPv6Prefix
	s.IPv6Address = nil
	s.IPv6Prefix = nil
	s.IPv6LeaseTime = 0
	s.mu.Unlock()

	if addr != nil {
		allocator.GetGlobalRegistry().ReleaseIANAByIP(addr)
	}
	if prefix != nil {
		allocator.GetGlobalRegistry().ReleasePDByPrefix(prefix)
	}
	if c.vpp != nil && swIdx != 0 {
		if addr != nil {
			c.vpp.PPPoESetSessionIPv6Async(swIdx, addr, false, func(error) {})
		}
		if prefix != nil {
			c.vpp.PPPoESetDelegatedPrefixAsync(swIdx, *prefix, net.IPv6zero, false, func(error) {})
		}
	}
	c.checkpointSession(s)
	c.logger.Debug("PPPoE DHCPv6 released", "session_id", s.SessionID)
}

// releaseDHCPv6Lease frees the provider lease keyed by the session's DUID on
// normal PPP teardown when the client did not send a Release.
func (c *Component) releaseDHCPv6Lease(s *SessionState) {
	if len(s.DHCPv6DUID) == 0 {
		return
	}
	for _, p := range c.dhcp6Providers {
		p.ReleaseLease(s.DHCPv6DUID)
	}
}

func (c *Component) getDHCP6Provider(profile *ip.IPv6Profile) dhcp6.DHCPProvider {
	mode := "local"
	if m := profileMode(profile); m == "relay" || m == "proxy" {
		mode = m
	}
	return c.dhcp6Providers[mode]
}

func (c *Component) resolveIPv6Profile(ctx *allocator.Context) *ip.IPv6Profile {
	if ctx == nil || ctx.IPv6ProfileName == "" {
		return nil
	}
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil {
		return nil
	}
	return cfg.IPv6Profiles[ctx.IPv6ProfileName]
}

func (c *Component) resolveDHCPv6(ctx *allocator.Context) *dhcp.ResolvedDHCPv6 {
	profile := c.resolveIPv6Profile(ctx)
	if profile == nil {
		return nil
	}
	return dhcp.ResolveV6(ctx, profile)
}

func profileMode(profile *ip.IPv6Profile) string {
	if profile == nil {
		return ""
	}
	return profile.GetMode()
}
