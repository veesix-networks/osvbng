// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ipoe

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/google/gopacket/layers"
	"github.com/google/uuid"
	"github.com/veesix-networks/osvbng/internal/ra"
	"github.com/veesix-networks/osvbng/pkg/aaa"
	"github.com/veesix-networks/osvbng/pkg/allocator"
	aaacfg "github.com/veesix-networks/osvbng/pkg/config/aaa"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/dhcp"
	"github.com/veesix-networks/osvbng/pkg/dhcp6"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/session"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func unwrapInnerReply(raw []byte) *dhcp6.Message {
	if len(raw) == 0 {
		return nil
	}
	if dhcp6.MessageType(raw[0]) == dhcp6.MsgTypeRelayReply {
		return dhcp6.UnwrapRelayReply(raw)
	}
	msg, err := dhcp6.ParseMessage(raw)
	if err != nil {
		return nil
	}
	return msg
}

func splitPendingDHCPv6(raw []byte) ([]byte, *dhcp6.RelayInfo) {
	if len(raw) == 0 {
		return raw, nil
	}
	if dhcp6.MessageType(raw[0]) != dhcp6.MsgTypeRelayForward {
		return raw, nil
	}
	inner, info := dhcp6.UnwrapRelay(raw)
	if inner == nil {
		return raw, info
	}
	return inner.Raw, info
}

func (c *Component) unwrapDHCPv6Relay(rawDHCPv6 []byte) (*dhcp6.Message, *dhcp6.RelayInfo) {
	msg, info := dhcp6.UnwrapRelay(rawDHCPv6)
	if info != nil {
		c.logger.WithGroup(logger.IPoEDHCP6).Debug("DHCPv6 relay message",
			"hop_count", info.HopCount,
			"link_addr", info.LinkAddr,
			"peer_addr", info.PeerAddr,
			"interface_id", string(info.InterfaceID),
			"remote_id", string(info.RemoteID))
	}
	if msg != nil {
		c.logger.WithGroup(logger.IPoEDHCP6).Debug("Unwrapped inner DHCPv6 message",
			"inner_type", msg.MsgType,
			"xid", fmt.Sprintf("0x%x", msg.TransactionID))
	}
	return msg, info
}

func (c *Component) consumeDHCPv6Packets() {
	if c.dhcp6Chan == nil {
		c.logger.Debug("DHCPv6 channel not configured, skipping DHCPv6 consumer")
		return
	}

	for {
		select {
		case <-c.Ctx.Done():
			return
		case pkt := <-c.dhcp6Chan:
			go func(pkt *dataplane.ParsedPacket) {
				if err := c.processDHCPv6Packet(pkt); err != nil {
					c.logger.Error("Error processing DHCPv6 packet", "error", err)
				}
			}(pkt)
		}
	}
}

func (c *Component) processDHCPv6Packet(pkt *dataplane.ParsedPacket) error {
	if pkt.DHCPv6 == nil {
		return fmt.Errorf("no DHCPv6 layer")
	}

	if pkt.OuterVLAN == 0 {
		return fmt.Errorf("packet rejected: S-VLAN required")
	}

	if len(c.dhcp6Providers) == 0 {
		return fmt.Errorf("no DHCPv6 provider configured")
	}

	if c.srgMgr != nil {
		srgName := c.resolveSRGName(pkt.OuterVLAN, pkt.InnerVLAN)
		if !c.srgMgr.IsActive(srgName) {
			return nil
		}
	}

	rawDHCPv6 := append(pkt.DHCPv6.LayerContents(), pkt.DHCPv6.LayerPayload()...)

	c.logger.WithGroup(logger.IPoEDHCP6).Debug("Received DHCPv6 packet",
		"message_type", pkt.DHCPv6.MsgType.String(),
		"mac", pkt.MAC.String(),
		"xid", fmt.Sprintf("0x%x", pkt.DHCPv6.TransactionID))

	if pkt.DHCPv6.MsgType == layers.DHCPv6MsgTypeRelayForward {
		inner, info := c.unwrapDHCPv6Relay(rawDHCPv6)
		if inner == nil {
			return fmt.Errorf("failed to unwrap relay message")
		}
		if !c.allowRelayForward(pkt.OuterVLAN, pkt.InnerVLAN) {
			c.logger.WithGroup(logger.IPoEDHCP6).Info("Rejected DHCPv6 Relay-Forward: subscriber group opted out",
				"svlan", pkt.OuterVLAN, "mac", pkt.MAC.String())
			return nil
		}
		return c.processDHCPv6Message(pkt, inner, info)
	}

	msg, err := dhcp6.ParseMessage(rawDHCPv6)
	if err != nil {
		return fmt.Errorf("parse DHCPv6: %w", err)
	}

	return c.processDHCPv6Message(pkt, msg, nil)
}

func (c *Component) processDHCPv6Message(pkt *dataplane.ParsedPacket, msg *dhcp6.Message, relayInfo *dhcp6.RelayInfo) error {
	switch msg.MsgType {
	case dhcp6.MsgTypeSolicit:
		return c.handleDHCPv6Solicit(pkt, msg, relayInfo)
	case dhcp6.MsgTypeRequest, dhcp6.MsgTypeRenew, dhcp6.MsgTypeRebind:
		return c.handleDHCPv6Request(pkt, msg, relayInfo)
	case dhcp6.MsgTypeRelease, dhcp6.MsgTypeDecline:
		return c.handleDHCPv6Release(pkt, msg, relayInfo)
	}

	return nil
}

func (c *Component) handleDHCPv6Solicit(pkt *dataplane.ParsedPacket, msg *dhcp6.Message, relayInfo *dhcp6.RelayInfo) error {
	var relayInterfaceID, relayRemoteID []byte
	if relayInfo != nil {
		relayInterfaceID = relayInfo.InterfaceID
		relayRemoteID = relayInfo.RemoteID
	}
	lookupKey := c.makeSessionKeyV6(pkt.MAC, pkt.OuterVLAN, pkt.InnerVLAN)

	var sess *SessionState
	if val, ok := c.sessions.Load(lookupKey); ok {
		sess = val.(*SessionState)
	}

	if sess != nil && !c.sessionV6Enabled(sess) {
		ipoeDropFamilyV6.WithLabelValues(sess.GroupName).Inc()
		c.logger.WithGroup(logger.IPoEDHCP6).Debug("DHCPv6 SOLICIT dropped: session v6-disabled",
			"session_id", sess.SessionID, "group", sess.GroupName)
		return nil
	}

	if sess == nil {
		if !c.IsReady() {
			c.logger.WithGroup(logger.IPoEDHCP6).Debug("DHCPv6 Solicit dropped: component not ready",
				"mac", pkt.MAC.String(), "svlan", pkt.OuterVLAN, "cvlan", pkt.InnerVLAN,
				"state", c.ReadyState().String())
			return nil
		}
		match, matched := c.cfgMgr.LookupSubscriberGroup(pkt.OuterVLAN, pkt.InnerVLAN)
		if !matched {
			c.logger.WithGroup(logger.IPoEDHCP6).Debug("DHCPv6 Solicit dropped: no subscriber-group match",
				"mac", pkt.MAC.String(), "svlan", pkt.OuterVLAN, "cvlan", pkt.InnerVLAN)
			return nil
		}
		if !groupV6Enabled(match.Group) {
			ipoeDropFamilyV6.WithLabelValues(match.Name).Inc()
			c.logger.WithGroup(logger.IPoEDHCP6).Debug("DHCPv6 SOLICIT dropped: group v6-disabled",
				"mac", pkt.MAC.String(), "svlan", pkt.OuterVLAN, "cvlan", pkt.InnerVLAN, "group", match.Name)
			return nil
		}
		sessID := session.GenerateID()
		newSess := &SessionState{
			SessionID:     sessID,
			AcctSessionID: session.ToAcctSessionID(sessID),
			MAC:           pkt.MAC,
			OuterVLAN:     pkt.OuterVLAN,
			InnerVLAN:     pkt.InnerVLAN,
			EncapIfIndex:  pkt.SwIfIndex,
			State:         "soliciting",
			GroupName:     match.Name,
		}

		c.sessionIndex.Store(sessID, newSess)
		if actual, loaded := c.sessions.LoadOrStore(lookupKey, newSess); loaded {
			c.sessionIndex.Delete(sessID)
			sess = actual.(*SessionState)
		} else {
			sess = newSess
			c.claimTuple(sess)
		}
	}

	sess.mu.Lock()
	if sess.Closing {
		sess.mu.Unlock()
		return nil
	}
	sess.DHCPv6XID = msg.TransactionID
	sess.DHCPv6DUID = msg.Options.ClientID
	sess.LastSeen = time.Now()
	sess.PendingDHCPv6Solicit = append(pkt.DHCPv6.LayerContents(), pkt.DHCPv6.LayerPayload()...)
	if pkt.IPv6 != nil {
		sess.ClientLinkLocal = pkt.IPv6.SrcIP
	}
	alreadyApproved := sess.AAAApproved
	ipoeCreated := sess.IPoESessionCreated
	circuitID := sess.CircuitID
	remoteID := sess.RemoteID
	aaaInFlight := sess.AAAInFlight
	if !alreadyApproved && !aaaInFlight {
		sess.AAAInFlight = true
	}
	sess.mu.Unlock()
	c.xid6Index.Store(msg.TransactionID, sess)

	if len(circuitID) == 0 && len(relayInterfaceID) > 0 {
		circuitID = relayInterfaceID
		c.logger.WithGroup(logger.IPoEDHCP6).Debug("Using DHCPv6 relay interface-id as circuit-id", "interface_id", string(relayInterfaceID))
	}
	if len(remoteID) == 0 && len(relayRemoteID) > 0 {
		remoteID = relayRemoteID
		c.logger.WithGroup(logger.IPoEDHCP6).Debug("Using DHCPv6 relay remote-id as remote-id", "remote_id", string(relayRemoteID))
	}

	if alreadyApproved && ipoeCreated {
		return c.forwardDHCPv6ToProvider(sess, pkt, msg.Raw, relayInfo)
	}

	if alreadyApproved && !ipoeCreated {
		c.logger.WithGroup(logger.IPoEDHCP6).Debug("DHCPv6 SOLICIT received, AAA approved but IPoE session pending", "session_id", sess.SessionID)
		return nil
	}

	if aaaInFlight {
		c.logger.WithGroup(logger.IPoEDHCP6).Debug("AAA already in flight, skipping duplicate request", "session_id", sess.SessionID)
		return nil
	}

	c.logger.WithGroup(logger.IPoEDHCP6).Debug("DHCPv6 SOLICIT received, requesting AAA", "session_id", sess.SessionID)

	cfg, _ := c.cfgMgr.GetRunning()
	username := pkt.MAC.String()
	var policyName string
	var groupName string
	var accessInterface string
	if cfg != nil {
		accessInterface, _ = cfg.GetAccessInterface()
		if match, ok := c.cfgMgr.LookupSubscriberGroup(pkt.OuterVLAN, pkt.InnerVLAN); ok {
			groupName = match.Name
			if match.VR != nil && match.VR.AAA != nil && match.VR.AAA.Policy != "" {
				policyName = match.VR.AAA.Policy
			} else {
				policyName = match.Group.AAAPolicy
			}
		}
	}
	aaaAttrs := make(map[string]string)
	var usernameFallback bool
	if policyName != "" {
		if policy := cfg.AAA.GetPolicyByType(policyName, aaacfg.PolicyTypeDHCP); policy != nil {
			ctx := &aaacfg.PolicyContext{
				MACAddress: pkt.MAC,
				SVLAN:      pkt.OuterVLAN,
				CVLAN:      pkt.InnerVLAN,
				RemoteID:   string(remoteID),
				CircuitID:  string(circuitID),
			}
			expanded, ok := policy.ExpandFormatChecked(ctx)
			if ok {
				username = expanded
			} else if policy.Format != "" {
				usernameFallback = true
				c.logger.WithGroup(logger.IPoEDHCP6).Warn("AAA policy username unresolved; using MAC fallback",
					"policy", policyName, "group", groupName, "mac", pkt.MAC.String(),
					"svlan", pkt.OuterVLAN, "cvlan", pkt.InnerVLAN, "format", policy.Format,
					"remote_id", string(remoteID), "circuit_id", string(circuitID))
				aaa.UsernameFallbacks.WithLabelValues(policyName, groupName, "ipoe-dhcpv6").Inc()
			}
			if policy.Password != "" {
				aaaAttrs[aaa.AttrPassword] = policy.ExpandPassword(ctx)
			}
			c.logger.WithGroup(logger.IPoEDHCP6).Debug("Built username from policy", "policy", policyName, "format", policy.Format, "username", username)
		}
	}

	sess.Username = username

	requestID := uuid.New().String()

	if len(circuitID) > 0 {
		aaaAttrs[aaa.AttrCircuitID] = string(circuitID)
	}
	if len(remoteID) > 0 {
		aaaAttrs[aaa.AttrRemoteID] = string(remoteID)
	}

	aaaPayload := &models.AAARequest{
		RequestID:        requestID,
		Username:         username,
		MAC:              pkt.MAC.String(),
		AcctSessionID:    sess.AcctSessionID,
		SVLAN:            pkt.OuterVLAN,
		CVLAN:            pkt.InnerVLAN,
		Interface:        accessInterface,
		AccessIfIndex:    sess.EncapIfIndex,
		AccessInterface:  c.accessInterfaceName(sess.EncapIfIndex),
		PolicyName:       policyName,
		UsernameFallback: usernameFallback,
		Attributes:       aaaAttrs,
	}

	c.logger.WithGroup(logger.IPoEDHCP6).Debug("Publishing AAA request for DHCPv6 SOLICIT", "session_id", sess.SessionID, "username", username)

	c.eventBus.Publish(events.TopicAAARequest, events.Event{
		Source: c.Name(),
		Data: &events.AAARequestEvent{
			AccessType: models.AccessTypeIPoE,
			Protocol:   models.ProtocolDHCPv6,
			SessionID:  sess.SessionID,
			Request:    *aaaPayload,
		},
	})
	return nil
}

func (c *Component) handleDHCPv6Request(pkt *dataplane.ParsedPacket, msg *dhcp6.Message, relayInfo *dhcp6.RelayInfo) error {
	lookupKey := c.makeSessionKeyV6(pkt.MAC, pkt.OuterVLAN, pkt.InnerVLAN)

	val, ok := c.sessions.Load(lookupKey)
	if !ok {
		return fmt.Errorf("no session for DHCPv6 REQUEST")
	}
	sess := val.(*SessionState)

	if !c.sessionV6Enabled(sess) {
		ipoeDropFamilyV6.WithLabelValues(sess.GroupName).Inc()
		c.logger.WithGroup(logger.IPoEDHCP6).Debug("DHCPv6 REQUEST dropped: session v6-disabled",
			"session_id", sess.SessionID, "group", sess.GroupName)
		return nil
	}

	sess.mu.Lock()
	sess.DHCPv6XID = msg.TransactionID
	sess.LastSeen = time.Now()
	sess.PendingDHCPv6Request = append(pkt.DHCPv6.LayerContents(), pkt.DHCPv6.LayerPayload()...)
	if pkt.IPv6 != nil && sess.ClientLinkLocal == nil {
		sess.ClientLinkLocal = pkt.IPv6.SrcIP
	}
	alreadyApproved := sess.AAAApproved
	ipoeCreated := sess.IPoESessionCreated
	sess.mu.Unlock()
	c.xid6Index.Store(msg.TransactionID, sess)

	if alreadyApproved && ipoeCreated {
		return c.forwardDHCPv6ToProvider(sess, pkt, msg.Raw, relayInfo)
	}

	c.logger.WithGroup(logger.IPoEDHCP6).Debug("DHCPv6 REQUEST received, session awaiting AAA", "session_id", sess.SessionID)

	return nil
}

func (c *Component) handleDHCPv6Release(pkt *dataplane.ParsedPacket, msg *dhcp6.Message, relayInfo *dhcp6.RelayInfo) error {
	lookupKey := c.makeSessionKeyV6(pkt.MAC, pkt.OuterVLAN, pkt.InnerVLAN)

	val, ok := c.sessions.Load(lookupKey)
	if !ok {
		c.logger.Debug("Received DHCPv6 Release for unknown session", "mac", pkt.MAC.String(), "svlan", pkt.OuterVLAN, "cvlan", pkt.InnerVLAN)
		return nil
	}
	sess := val.(*SessionState)

	sess.mu.Lock()
	sessID := sess.SessionID
	ipv6Address := sess.IPv6Address
	ipv6Prefix := sess.IPv6Prefix
	ipoeSwIfIndex := sess.IPoESwIfIndex
	mac := sess.MAC
	encapIfIndex := sess.EncapIfIndex
	innerVLAN := sess.InnerVLAN
	ipv4 := sess.IPv4
	ipv4Bound := ipv4 != nil
	xid6 := sess.DHCPv6XID
	duid := sess.DHCPv6DUID

	sess.IPv6Bound = false

	sessionMode := c.getSessionMode(pkt.OuterVLAN, pkt.InnerVLAN)
	deleteSession := true
	if sessionMode == subscriber.SessionModeUnified && ipv4Bound {
		deleteSession = false
	}

	sess.IPv6Address = nil
	sess.IPv6Prefix = nil
	if deleteSession {
		sess.IPv4 = nil
		sess.Closing = true
	}
	sess.mu.Unlock()
	c.xid6Index.Delete(xid6)
	if deleteSession {
		c.sessions.Delete(lookupKey)
		c.sessionIndex.Delete(sessID)
		c.removeSessionFromIndexes(sess)
	}

	c.logger.Debug("IPv6 released by client", "session_id", sessID, "delete_session", deleteSession)

	if len(c.dhcp6Providers) > 0 {
		v6Prof := c.resolveIPv6Profile(sess.AllocCtx)
		v6Prov := c.getDHCP6Provider(v6Prof)
		if v6Prov != nil {
			dhcpPkt := &dhcp6.Packet{
				SessionID: sessID,
				MAC:       mac.String(),
				SVLAN:     pkt.OuterVLAN,
				CVLAN:     pkt.InnerVLAN,
				DUID:      duid,
				Raw:       msg.Raw,
				SwIfIndex: sess.EncapIfIndex,
				Interface: c.resolveAccessInterfaceName(sess.EncapIfIndex),
				PeerAddr:  sess.ClientLinkLocal,
				Profile:   v6Prof,
				LocalMAC:  c.getLocalMAC(sess.SRGName, sess.EncapIfIndex),
				RelayInfo: relayInfo,
			}
			response, err := v6Prov.HandlePacket(c.Ctx, dhcpPkt)
			if err != nil {
				c.logger.Warn("DHCPv6 provider failed on Release", "session_id", sessID, "error", err)
			} else if response != nil && len(response.Raw) > 0 {
				if err := c.sendDHCPv6Response(sess, response.Raw); err != nil {
					c.logger.Debug("Failed to send DHCPv6 Release Reply", "session_id", sessID, "error", err)
				}
			}
		}
	}

	if ipv6Address != nil {
		allocator.GetGlobalRegistry().ReleaseIANAByIP(ipv6Address)
	}
	if ipv6Prefix != nil {
		allocator.GetGlobalRegistry().ReleasePDByPrefix(ipv6Prefix)
	}
	if deleteSession {
		if ipv4 != nil {
			allocator.GetGlobalRegistry().ReleaseIP(ipv4)
		}
		for _, p := range c.dhcp4Providers {
			p.ReleaseLease(mac.String())
		}
	}

	if c.vpp != nil && ipoeSwIfIndex != 0 {
		if ipv6Address != nil {
			c.vpp.IPoESetSessionIPv6Async(ipoeSwIfIndex, ipv6Address, false, func(err error) {
				if err != nil {
					c.logger.Debug("Failed to unbind IPv6 from IPoE session", "session_id", sessID, "error", err)
				}
			})
		}
		if ipv6Prefix != nil {
			c.vpp.IPoESetDelegatedPrefixAsync(ipoeSwIfIndex, *ipv6Prefix, net.ParseIP("::"), false, func(err error) {
				if err != nil {
					c.logger.Debug("Failed to unbind delegated prefix from IPoE session", "session_id", sessID, "error", err)
				}
			})
		}
		if deleteSession {
			if ipv4 != nil {
				c.vpp.IPoESetSessionIPv4Async(ipoeSwIfIndex, ipv4, false, func(err error) {
					if err != nil {
						c.logger.Debug("Failed to unbind IPv4 from IPoE session", "session_id", sessID, "error", err)
					}
				})
			}
			c.vpp.DeleteIPoESessionAsync(mac, encapIfIndex, innerVLAN, func(err error) {
				if err != nil {
					c.logger.Warn("Failed to delete IPoE session", "session_id", sessID, "error", err)
				} else {
					c.logger.Debug("Deleted IPoE session", "session_id", sessID, "sw_if_index", ipoeSwIfIndex)
				}
			})
		}
	}

	if deleteSession {
		counterKey := fmt.Sprintf("osvbng:session_count:%s:%d:%d", pkt.MAC.String(), pkt.OuterVLAN, pkt.InnerVLAN)
		newCount, err := c.cache.Decr(c.Ctx, counterKey)
		if err != nil {
			c.logger.Warn("Failed to decrement session counter", "error", err, "key", counterKey)
		} else if newCount <= 0 {
			c.cache.Delete(c.Ctx, counterKey)
		}
		c.deleteSessionCheckpoint(sessID)
	} else if sess != nil {
		c.checkpointSession(sess)
	}

	if sessionMode != subscriber.SessionModeUnified {
		var prefixStr string
		if ipv6Prefix != nil {
			prefixStr = ipv6Prefix.String()
		}

		return c.publishSessionLifecycle(&models.IPoESession{
			SessionID:    sessID,
			State:        models.SessionStateReleased,
			AccessType:   string(models.AccessTypeIPoE),
			Protocol:     string(models.ProtocolDHCPv6),
			MAC:          mac,
			OuterVLAN:    pkt.OuterVLAN,
			InnerVLAN:    pkt.InnerVLAN,
			IfIndex:      ipoeSwIfIndex,
			VRF:          sess.VRF,
			SRGName:      sess.SRGName,
			IPv6Address:  ipv6Address,
			IPv6Prefix:   prefixStr,
			Username:     sess.Username,
			AAASessionID: "",
		})
	}

	return nil
}

func (c *Component) forwardDHCPv6ToProvider(sess *SessionState, pkt *dataplane.ParsedPacket, raw []byte, relayInfo *dhcp6.RelayInfo) error {
	v6Profile := c.resolveIPv6Profile(sess.AllocCtx)
	var resolved *dhcp.ResolvedDHCPv6
	if v6Profile == nil || v6Profile.GetMode() == "server" {
		resolved = c.resolveDHCPv6(sess.AllocCtx)
	}
	dhcpPkt := &dhcp6.Packet{
		SessionID: sess.SessionID,
		MAC:       sess.MAC.String(),
		SVLAN:     sess.OuterVLAN,
		CVLAN:     sess.InnerVLAN,
		DUID:      sess.DHCPv6DUID,
		Raw:       raw,
		Resolved:  resolved,
		SwIfIndex: sess.EncapIfIndex,
		Interface: c.resolveAccessInterfaceName(sess.EncapIfIndex),
		PeerAddr:  sess.ClientLinkLocal,
		Profile:   v6Profile,
		LocalMAC:  c.getLocalMAC(sess.SRGName, sess.EncapIfIndex),
		RelayInfo: relayInfo,
	}

	provider := c.getDHCP6Provider(v6Profile)
	if provider == nil {
		return fmt.Errorf("no DHCPv6 provider available")
	}
	response, err := provider.HandlePacket(c.Ctx, dhcpPkt)
	if err != nil {
		return fmt.Errorf("dhcp6 provider failed: %w", err)
	}

	if response == nil || len(response.Raw) == 0 {
		return nil
	}

	if err := c.sendDHCPv6Response(sess, response.Raw); err != nil {
		return err
	}

	respMsg := unwrapInnerReply(response.Raw)
	if respMsg != nil && respMsg.MsgType == dhcp6.MsgTypeReply {
		return c.handleDHCPv6Reply(sess, respMsg)
	}

	return nil
}

func (c *Component) handleDHCPv6Reply(sess *SessionState, msg *dhcp6.Message) error {
	var ianaAddr net.IP
	var pdPrefix *net.IPNet
	var validTime uint32

	if msg.Options.IANA != nil && msg.Options.IANA.Address != nil {
		ianaAddr = msg.Options.IANA.Address
		validTime = msg.Options.IANA.ValidTime
	}

	if msg.Options.IAPD != nil && msg.Options.IAPD.Prefix != nil {
		pdPrefix = &net.IPNet{
			IP:   msg.Options.IAPD.Prefix,
			Mask: net.CIDRMask(int(msg.Options.IAPD.PrefixLen), 128),
		}
	}

	sess.mu.Lock()
	sess.IPv6Address = ianaAddr
	sess.IPv6Prefix = pdPrefix
	sess.IPv6LeaseTime = validTime
	sess.IPv6BoundAt = time.Now()
	if sess.ActivatedAt.IsZero() {
		sess.ActivatedAt = sess.IPv6BoundAt
	}
	sess.IPv6Bound = true
	ipoeSwIfIndex := sess.IPoESwIfIndex
	v4AlreadyBound := sess.State == "bound" && sess.IPv4 != nil
	snapshotIPv4 := sess.IPv4
	snapshotLeaseTime := sess.LeaseTime
	sess.mu.Unlock()

	c.logger.WithGroup(logger.IPoEDHCP6).Debug("DHCPv6 session bound", "session_id", sess.SessionID, "ipv6", ianaAddr, "prefix", pdPrefix)

	if c.vpp != nil {
		sessID := sess.SessionID
		if ipoeSwIfIndex != 0 {
			if ianaAddr != nil {
				c.vpp.IPoESetSessionIPv6Async(ipoeSwIfIndex, ianaAddr, true, func(err error) {
					if err != nil {
						if errors.Is(err, southbound.ErrUnavailable) {
							c.logger.Debug("VPP unavailable, cannot bind IPv6", "session_id", sessID)
						} else {
							c.logger.Error("Failed to bind IPv6 to IPoE session", "session_id", sessID, "error", err)
						}
					} else {
						c.logger.Debug("Bound IPv6 to IPoE session", "session_id", sessID, "ipv6", ianaAddr.String())
					}
				})
			}

			if pdPrefix != nil {
				nextHop := ianaAddr
				if nextHop == nil {
					nextHop = net.ParseIP("::")
				}
				c.vpp.IPoESetDelegatedPrefixAsync(ipoeSwIfIndex, *pdPrefix, nextHop, true, func(err error) {
					if err != nil {
						if errors.Is(err, southbound.ErrUnavailable) {
							c.logger.Debug("VPP unavailable, cannot set delegated prefix", "session_id", sessID)
						} else {
							c.logger.Error("Failed to set delegated prefix", "session_id", sessID, "error", err)
						}
					} else {
						c.logger.Debug("Set delegated prefix", "session_id", sessID, "prefix", pdPrefix.String())
					}
				})
			}
		} else {
			sess.mu.Lock()
			sess.PendingIPv6Binding = ianaAddr
			sess.PendingPDBinding = pdPrefix
			sess.mu.Unlock()
			c.logger.Debug("IPoE session not ready, queued IPv6 bindings", "session_id", sessID)
		}
	}

	c.checkpointSession(sess)

	var prefixStr string
	if pdPrefix != nil {
		prefixStr = pdPrefix.String()
	}

	sessionMode := c.getSessionMode(sess.OuterVLAN, sess.InnerVLAN)
	if sessionMode == subscriber.SessionModeUnified && !v4AlreadyBound {
		return nil
	}

	ipoeSess := &models.IPoESession{
		SessionID:       sess.SessionID,
		State:           models.SessionStateActive,
		AccessType:      string(models.AccessTypeIPoE),
		Protocol:        string(models.ProtocolDHCPv6),
		MAC:             sess.MAC,
		OuterVLAN:       sess.OuterVLAN,
		InnerVLAN:       sess.InnerVLAN,
		VLANCount:       c.getVLANCount(sess.OuterVLAN, sess.InnerVLAN),
		IfIndex:         ipoeSwIfIndex,
		AccessIfIndex:   sess.EncapIfIndex,
		AccessInterface: c.accessInterfaceName(sess.EncapIfIndex),
		VRF:             sess.VRF,
		ServiceGroup:    sess.ServiceGroup.Name,
		SRGName:         sess.SRGName,
		IPv4Address:     snapshotIPv4,
		LeaseTime:       snapshotLeaseTime,
		IPv6Address:     ianaAddr,
		IPv6Prefix:      prefixStr,
		IPv6LeaseTime:   sess.IPv6LeaseTime,
		DUID:            sess.DHCPv6DUID,
		Username:        sess.Username,
		AAASessionID:    sess.AcctSessionID,
		ActivatedAt:     sess.ActivatedAt,
	}
	if sess.AllocCtx != nil {
		ipoeSess.IPv4Pool = sess.AllocCtx.AllocatedPool
		ipoeSess.IANAPool = sess.AllocCtx.AllocatedIANAPool
		ipoeSess.PDPool = sess.AllocCtx.AllocatedPDPool
	}

	return c.publishSessionLifecycle(ipoeSess)
}

func (c *Component) sendDHCPv6Response(sess *SessionState, rawDHCPv6 []byte) error {
	var parentSwIfIndex uint32
	if c.ifMgr != nil {
		if iface := c.ifMgr.Get(sess.EncapIfIndex); iface != nil {
			parentSwIfIndex = iface.SupSwIfIndex
		}
	}

	srcMACBytes := c.getLocalMAC(sess.SRGName, parentSwIfIndex)
	if srcMACBytes == nil {
		return fmt.Errorf("no source MAC available")
	}

	srcMAC := srcMACBytes.String()
	srcIP := ra.LinkLocalFromMAC(srcMACBytes)
	if srcIP == nil {
		return fmt.Errorf("no IPv6 source address available for S-VLAN %d", sess.OuterVLAN)
	}
	dstIP := sess.ClientLinkLocal
	if dstIP == nil {
		return fmt.Errorf("no client link-local address for session %s", sess.SessionID)
	}

	frame := dhcp.BuildIPv6UDPFrame(srcIP, dstIP, 547, 546, rawDHCPv6)
	if frame == nil {
		return fmt.Errorf("failed to build IPv6/UDP frame")
	}

	egressPayload := &models.EgressPacketPayload{
		DstMAC:    sess.MAC.String(),
		SrcMAC:    srcMAC,
		OuterVLAN: sess.OuterVLAN,
		InnerVLAN: sess.InnerVLAN,
		OuterTPID: c.ifMgr.OuterTPID(sess.EncapIfIndex),
		SwIfIndex: parentSwIfIndex,
		RawData:   frame,
	}

	c.logger.Debug("Sending DHCPv6 response", "session_id", sess.SessionID, "size", len(rawDHCPv6), "dst_ip", dstIP)

	c.eventBus.Publish(events.TopicEgress, events.Event{
		Source: c.Name(),
		Data: &events.EgressEvent{
			Protocol: models.ProtocolDHCPv6,
			Packet:   *egressPayload,
		},
	})
	return nil
}
