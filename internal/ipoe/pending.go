// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ipoe

import (
	"net"

	"github.com/veesix-networks/osvbng/pkg/allocator"
	"github.com/veesix-networks/osvbng/pkg/config/ip"
	"github.com/veesix-networks/osvbng/pkg/dhcp"
	"github.com/veesix-networks/osvbng/pkg/dhcp4"
	"github.com/veesix-networks/osvbng/pkg/dhcp6"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

func (c *Component) forwardPendingDHCPv4(sessID string, mac net.HardwareAddr, svlan, cvlan uint16, encapIfIndex uint32, accessIfName string, v4Profile *ip.IPv4Profile, localMAC net.HardwareAddr, allocCtx *allocator.Context, pendingDiscover, pendingRequest []byte) {
	provider := c.getDHCP4Provider(v4Profile)
	if provider == nil {
		c.logger.Error("No DHCPv4 provider available", "session_id", sessID)
		return
	}

	if pendingDiscover != nil {
		c.logger.Debug("Forwarding pending DHCP DISCOVER", "session_id", sessID)
		var resolved *dhcp.ResolvedDHCPv4
		if v4Profile == nil || v4Profile.GetMode() == "server" {
			resolved = c.resolveDHCPv4(allocCtx)
		}
		pkt := &dhcp4.Packet{
			SessionID: sessID,
			MAC:       mac.String(),
			SVLAN:     svlan,
			CVLAN:     cvlan,
			Raw:       pendingDiscover,
			Resolved:  resolved,
			SwIfIndex: encapIfIndex,
			Interface: accessIfName,
			Profile:   v4Profile,
			LocalMAC:  localMAC,
		}
		response, err := provider.HandlePacket(c.Ctx, pkt)
		if err != nil {
			c.logger.Error("DHCP provider failed for DISCOVER", "session_id", sessID, "error", err)
			return
		}
		if response != nil && len(response.Raw) > 0 {
			if err := c.sendDHCPResponse(sessID, svlan, cvlan, encapIfIndex, mac, response.Raw, "OFFER"); err != nil {
				c.logger.Error("Failed to send DHCP OFFER", "session_id", sessID, "error", err)
				return
			}
		}
	}

	if pendingRequest != nil {
		c.logger.Debug("Forwarding pending DHCP REQUEST", "session_id", sessID)
		var resolved *dhcp.ResolvedDHCPv4
		if v4Profile == nil || v4Profile.GetMode() == "server" {
			resolved = c.resolveDHCPv4(allocCtx)
		}
		pkt := &dhcp4.Packet{
			SessionID: sessID,
			MAC:       mac.String(),
			SVLAN:     svlan,
			CVLAN:     cvlan,
			Raw:       pendingRequest,
			Resolved:  resolved,
			SwIfIndex: encapIfIndex,
			Interface: accessIfName,
			Profile:   v4Profile,
			LocalMAC:  localMAC,
		}
		response, err := provider.HandlePacket(c.Ctx, pkt)
		if err != nil {
			c.logger.Error("DHCP provider failed for REQUEST", "session_id", sessID, "error", err)
			return
		}
		if response != nil && len(response.Raw) > 0 {
			if err := c.sendDHCPResponse(sessID, svlan, cvlan, encapIfIndex, mac, response.Raw, "ACK"); err != nil {
				c.logger.Error("Failed to send DHCP ACK", "session_id", sessID, "error", err)
				return
			}
		}
	}
}

func (c *Component) forwardPendingDHCPv6(sess *SessionState, sessID string, mac net.HardwareAddr, svlan, cvlan uint16, encapIfIndex uint32, accessIfName string, v6Profile *ip.IPv6Profile, v6Provider dhcp6.DHCPProvider, localMAC net.HardwareAddr, allocCtx *allocator.Context, dhcpv6DUID []byte, pendingSolicit, pendingRequest []byte) {
	if pendingSolicit != nil {
		c.logger.WithGroup(logger.IPoEDHCP6).Debug("Forwarding pending DHCPv6 SOLICIT", "session_id", sessID)
		var resolved *dhcp.ResolvedDHCPv6
		if v6Profile == nil || v6Profile.GetMode() == "server" {
			resolved = c.resolveDHCPv6(allocCtx)
		}
		inner, relayInfo := splitPendingDHCPv6(pendingSolicit)
		pkt := &dhcp6.Packet{
			SessionID: sessID,
			MAC:       mac.String(),
			SVLAN:     svlan,
			CVLAN:     cvlan,
			DUID:      dhcpv6DUID,
			Raw:       inner,
			Resolved:  resolved,
			SwIfIndex: encapIfIndex,
			Interface: accessIfName,
			PeerAddr:  sess.ClientLinkLocal,
			Profile:   v6Profile,
			LocalMAC:  localMAC,
			RelayInfo: relayInfo,
		}
		response, err := v6Provider.HandlePacket(c.Ctx, pkt)
		if err != nil {
			c.logger.Error("DHCPv6 provider failed for SOLICIT", "session_id", sessID, "error", err)
		} else if response != nil && len(response.Raw) > 0 {
			if err := c.sendDHCPv6Response(sess, response.Raw); err != nil {
				c.logger.Error("Failed to send DHCPv6 ADVERTISE", "session_id", sessID, "error", err)
			}
		}
	}

	if pendingRequest != nil {
		c.logger.WithGroup(logger.IPoEDHCP6).Debug("Forwarding pending DHCPv6 REQUEST", "session_id", sessID)
		var resolved *dhcp.ResolvedDHCPv6
		if v6Profile == nil || v6Profile.GetMode() == "server" {
			resolved = c.resolveDHCPv6(allocCtx)
		}
		inner, relayInfo := splitPendingDHCPv6(pendingRequest)
		pkt := &dhcp6.Packet{
			SessionID: sessID,
			MAC:       mac.String(),
			SVLAN:     svlan,
			CVLAN:     cvlan,
			DUID:      dhcpv6DUID,
			Raw:       inner,
			Resolved:  resolved,
			SwIfIndex: encapIfIndex,
			Interface: accessIfName,
			PeerAddr:  sess.ClientLinkLocal,
			Profile:   v6Profile,
			LocalMAC:  localMAC,
			RelayInfo: relayInfo,
		}
		response, err := v6Provider.HandlePacket(c.Ctx, pkt)
		if err != nil {
			c.logger.Error("DHCPv6 provider failed for REQUEST", "session_id", sessID, "error", err)
		} else if response != nil && len(response.Raw) > 0 {
			if err := c.sendDHCPv6Response(sess, response.Raw); err != nil {
				c.logger.Error("Failed to send DHCPv6 REPLY", "session_id", sessID, "error", err)
			}
			respMsg := unwrapInnerReply(response.Raw)
			if respMsg != nil && respMsg.MsgType == dhcp6.MsgTypeReply {
				c.handleDHCPv6Reply(sess, respMsg)
			}
		}
	}
}

func (c *Component) forwardLatePendingPackets(sess *SessionState, sessID string, mac net.HardwareAddr, svlan, cvlan uint16, encapIfIndex uint32, srgName string, allocCtx *allocator.Context, dhcpv6DUID []byte, pendingV4Discover, pendingV4Request, pendingV6Solicit, pendingV6Request []byte) {
	if pendingV4Discover == nil && pendingV4Request == nil && pendingV6Solicit == nil && pendingV6Request == nil {
		return
	}

	v4Profile := c.resolveIPv4Profile(allocCtx)
	v6Profile := c.resolveIPv6Profile(allocCtx)
	accessIfName := c.resolveAccessInterfaceName(encapIfIndex)
	localMAC := c.getLocalMAC(srgName, encapIfIndex)

	if pendingV4Discover != nil {
		c.logger.Debug("Forwarding late-pending DHCP DISCOVER", "session_id", sessID)
		var resolved *dhcp.ResolvedDHCPv4
		if v4Profile == nil || v4Profile.GetMode() == "server" {
			resolved = c.resolveDHCPv4(allocCtx)
		}
		pkt := &dhcp4.Packet{
			SessionID: sessID,
			MAC:       mac.String(),
			SVLAN:     svlan,
			CVLAN:     cvlan,
			Raw:       pendingV4Discover,
			Resolved:  resolved,
			SwIfIndex: encapIfIndex,
			Interface: accessIfName,
			Profile:   v4Profile,
			LocalMAC:  localMAC,
		}
		provider := c.getDHCP4Provider(v4Profile)
		if provider != nil {
			response, err := provider.HandlePacket(c.Ctx, pkt)
			if err != nil {
				c.logger.Error("DHCP provider failed for late-pending DISCOVER", "session_id", sessID, "error", err)
			} else if response != nil && len(response.Raw) > 0 {
				if err := c.sendDHCPResponse(sessID, svlan, cvlan, encapIfIndex, mac, response.Raw, "OFFER"); err != nil {
					c.logger.Error("Failed to send DHCP OFFER", "session_id", sessID, "error", err)
				}
			}
		}
	}

	if pendingV4Request != nil {
		c.logger.Debug("Forwarding late-pending DHCP REQUEST", "session_id", sessID)
		var resolved *dhcp.ResolvedDHCPv4
		if v4Profile == nil || v4Profile.GetMode() == "server" {
			resolved = c.resolveDHCPv4(allocCtx)
		}
		pkt := &dhcp4.Packet{
			SessionID: sessID,
			MAC:       mac.String(),
			SVLAN:     svlan,
			CVLAN:     cvlan,
			Raw:       pendingV4Request,
			Resolved:  resolved,
			SwIfIndex: encapIfIndex,
			Interface: accessIfName,
			Profile:   v4Profile,
			LocalMAC:  localMAC,
		}
		provider := c.getDHCP4Provider(v4Profile)
		if provider != nil {
			response, err := provider.HandlePacket(c.Ctx, pkt)
			if err != nil {
				c.logger.Error("DHCP provider failed for late-pending REQUEST", "session_id", sessID, "error", err)
			} else if response != nil && len(response.Raw) > 0 {
				if err := c.sendDHCPResponse(sessID, svlan, cvlan, encapIfIndex, mac, response.Raw, "ACK"); err != nil {
					c.logger.Error("Failed to send DHCP ACK", "session_id", sessID, "error", err)
				}
			}
		}
	}

	v6Provider := c.getDHCP6Provider(v6Profile)

	if pendingV6Solicit != nil && v6Provider != nil {
		c.logger.WithGroup(logger.IPoEDHCP6).Debug("Forwarding late-pending DHCPv6 SOLICIT", "session_id", sessID)
		var resolved *dhcp.ResolvedDHCPv6
		if v6Profile == nil || v6Profile.GetMode() == "server" {
			resolved = c.resolveDHCPv6(allocCtx)
		}
		inner, relayInfo := splitPendingDHCPv6(pendingV6Solicit)
		pkt := &dhcp6.Packet{
			SessionID: sessID,
			MAC:       mac.String(),
			SVLAN:     svlan,
			CVLAN:     cvlan,
			DUID:      dhcpv6DUID,
			Raw:       inner,
			Resolved:  resolved,
			SwIfIndex: encapIfIndex,
			Interface: accessIfName,
			PeerAddr:  sess.ClientLinkLocal,
			Profile:   v6Profile,
			LocalMAC:  localMAC,
			RelayInfo: relayInfo,
		}
		response, err := v6Provider.HandlePacket(c.Ctx, pkt)
		if err != nil {
			c.logger.Error("DHCPv6 provider failed for late-pending SOLICIT", "session_id", sessID, "error", err)
		} else if response != nil && len(response.Raw) > 0 {
			if err := c.sendDHCPv6Response(sess, response.Raw); err != nil {
				c.logger.Error("Failed to send DHCPv6 ADVERTISE", "session_id", sessID, "error", err)
			}
		}
	}

	if pendingV6Request != nil && v6Provider != nil {
		c.logger.WithGroup(logger.IPoEDHCP6).Debug("Forwarding late-pending DHCPv6 REQUEST", "session_id", sessID)
		var resolved *dhcp.ResolvedDHCPv6
		if v6Profile == nil || v6Profile.GetMode() == "server" {
			resolved = c.resolveDHCPv6(allocCtx)
		}
		inner, relayInfo := splitPendingDHCPv6(pendingV6Request)
		pkt := &dhcp6.Packet{
			SessionID: sessID,
			MAC:       mac.String(),
			SVLAN:     svlan,
			CVLAN:     cvlan,
			DUID:      dhcpv6DUID,
			Raw:       inner,
			Resolved:  resolved,
			SwIfIndex: encapIfIndex,
			Interface: accessIfName,
			PeerAddr:  sess.ClientLinkLocal,
			Profile:   v6Profile,
			LocalMAC:  localMAC,
			RelayInfo: relayInfo,
		}
		response, err := v6Provider.HandlePacket(c.Ctx, pkt)
		if err != nil {
			c.logger.Error("DHCPv6 provider failed for late-pending REQUEST", "session_id", sessID, "error", err)
		} else if response != nil && len(response.Raw) > 0 {
			if err := c.sendDHCPv6Response(sess, response.Raw); err != nil {
				c.logger.Error("Failed to send DHCPv6 REPLY", "session_id", sessID, "error", err)
			}
			respMsg := unwrapInnerReply(response.Raw)
			if respMsg != nil && respMsg.MsgType == dhcp6.MsgTypeReply {
				c.handleDHCPv6Reply(sess, respMsg)
			}
		}
	}
}
