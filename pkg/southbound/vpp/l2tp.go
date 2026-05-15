// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package vpp

import (
	"fmt"
	"net"

	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip_types"
	binapi "github.com/veesix-networks/osvbng/pkg/vpp/binapi/l2tpv2"
)

const (
	l2tpDecapModeIP  uint8 = 0
	l2tpDecapModeRaw uint8 = 1
)

// AddL2TPTunnel installs a tunnel-level lookup entry in the VPP plugin.
// Returns the plugin-assigned tunnel_index.
func (v *VPP) AddL2TPTunnel(local, peer net.IP, localID, peerID, localPort, peerPort uint16, dfBit bool) (uint32, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return 0, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	localAddr, err := v.toAddress(local)
	if err != nil {
		return 0, fmt.Errorf("convert local IP: %w", err)
	}
	peerAddr, err := v.toAddress(peer)
	if err != nil {
		return 0, fmt.Errorf("convert peer IP: %w", err)
	}

	req := &binapi.L2tpv2AddDelTunnel{
		IsAdd:         true,
		LocalIP:       localAddr,
		PeerIP:        peerAddr,
		LocalTunnelID: localID,
		PeerTunnelID:  peerID,
		LocalUDPPort:  localPort,
		PeerUDPPort:   peerPort,
		DfBit:         dfBit,
	}
	reply := &binapi.L2tpv2AddDelTunnelReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return 0, fmt.Errorf("l2tpv2_add_del_tunnel: %w", err)
	}
	if reply.Retval != 0 {
		return 0, fmt.Errorf("l2tpv2_add_del_tunnel rv=%d", reply.Retval)
	}
	return reply.TunnelIndex, nil
}

// DeleteL2TPTunnel removes a tunnel-level lookup entry. The tunnel must
// have no remaining sessions; the plugin returns INSTANCE_IN_USE otherwise.
func (v *VPP) DeleteL2TPTunnel(local, peer net.IP, localID uint16) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	localAddr, err := v.toAddress(local)
	if err != nil {
		return err
	}
	peerAddr, err := v.toAddress(peer)
	if err != nil {
		return err
	}

	req := &binapi.L2tpv2AddDelTunnel{
		IsAdd:         false,
		LocalIP:       localAddr,
		PeerIP:        peerAddr,
		LocalTunnelID: localID,
	}
	reply := &binapi.L2tpv2AddDelTunnelReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("l2tpv2_add_del_tunnel: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("l2tpv2_add_del_tunnel rv=%d", reply.Retval)
	}
	return nil
}

// AddPPPoL2TPSession installs a DECAP_IP session for a PPP subscriber
// terminating on the LNS. The plugin allocates a per-session vnet
// interface; the returned sw_if_index is what the caller binds for
// FIB / QoS / counters.
func (v *VPP) AddPPPoL2TPSession(local, peer net.IP, localTunnelID, localSessionID, peerSessionID uint16, decapVrfID uint32, encapIfIndex uint32, pppHdrSkip uint8) (uint32, error) {
	return v.addL2TPSession(local, peer, localTunnelID, localSessionID, peerSessionID, l2tpDecapModeIP, "", 0, decapVrfID, encapIfIndex, pppHdrSkip)
}

// AddL2TPSessionRaw installs a DECAP_RAW session. PPP frames decapsulated
// from this session are forwarded to `rawNextNode` (resolved at session-
// add time by the plugin) with `rawOpaque` stashed in the buffer for the
// downstream node to interpret. The returned uint32 is the L2TPv2 plugin's
// session pool index — the consumer (LAC bridge) stashes it on the partner
// PPPoE session struct so the subscriber→LNS path can stash it back into
// `vnet_buffer_l2tpv2_opaque` for l2tpv2-encap-raw to look up the session.
func (v *VPP) AddL2TPSessionRaw(local, peer net.IP, localTunnelID, localSessionID, peerSessionID uint16, rawNextNode string, rawOpaque uint32, encapIfIndex uint32, pppHdrSkip uint8) (uint32, error) {
	return v.addL2TPSession(local, peer, localTunnelID, localSessionID, peerSessionID, l2tpDecapModeRaw, rawNextNode, rawOpaque, 0, encapIfIndex, pppHdrSkip)
}

func (v *VPP) addL2TPSession(local, peer net.IP, localTunnelID, localSessionID, peerSessionID uint16, mode uint8, rawNextNode string, rawOpaque, decapVrfID, encapIfIndex uint32, pppHdrSkip uint8) (uint32, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return 0, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	localAddr, err := v.toAddress(local)
	if err != nil {
		return 0, err
	}
	peerAddr, err := v.toAddress(peer)
	if err != nil {
		return 0, err
	}

	req := &binapi.L2tpv2AddDelSession{
		IsAdd:           true,
		LocalIP:         localAddr,
		PeerIP:          peerAddr,
		LocalTunnelID:   localTunnelID,
		LocalSessionID:  localSessionID,
		PeerSessionID:   peerSessionID,
		DecapMode:       mode,
		RawNextNodeName: rawNextNode,
		RawOpaque:       rawOpaque,
		DecapVrfID:      decapVrfID,
		EncapIfIndex:    interface_types.InterfaceIndex(encapIfIndex),
		PppHdrSkip:      pppHdrSkip,
	}
	reply := &binapi.L2tpv2AddDelSessionReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return 0, fmt.Errorf("l2tpv2_add_del_session: %w", err)
	}
	if reply.Retval != 0 {
		return 0, fmt.Errorf("l2tpv2_add_del_session rv=%d", reply.Retval)
	}
	return uint32(reply.SwIfIndex), nil
}

// DeleteL2TPSession removes a session. For DECAP_IP sessions this also
// disables and reclaims the per-session vnet interface.
func (v *VPP) DeleteL2TPSession(local, peer net.IP, localTunnelID, localSessionID uint16) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	localAddr, err := v.toAddress(local)
	if err != nil {
		return err
	}
	peerAddr, err := v.toAddress(peer)
	if err != nil {
		return err
	}

	req := &binapi.L2tpv2AddDelSession{
		IsAdd:          false,
		LocalIP:        localAddr,
		PeerIP:         peerAddr,
		LocalTunnelID:  localTunnelID,
		LocalSessionID: localSessionID,
	}
	reply := &binapi.L2tpv2AddDelSessionReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("l2tpv2_add_del_session: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("l2tpv2_add_del_session rv=%d", reply.Retval)
	}
	return nil
}

// PPPoL2TPSetSubscriberIPv4 installs (or removes) the subscriber's
// IPv4 /32 route on the per-session DECAP_IP vnet interface. The
// plugin tracks the binding and auto-cleans it if the session is
// deleted before the caller unbinds.
func (v *VPP) PPPoL2TPSetSubscriberIPv4(swIfIndex uint32, clientIP net.IP, isAdd bool) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	ip4 := clientIP.To4()
	if ip4 == nil {
		return fmt.Errorf("invalid IPv4 address: %s", clientIP)
	}
	var addr ip_types.IP4Address
	copy(addr[:], ip4)

	req := &binapi.L2tpv2SetSessionIPv4{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		ClientIP:  addr,
		IsAdd:     isAdd,
	}
	reply := &binapi.L2tpv2SetSessionIPv4Reply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("l2tpv2_set_session_ipv4: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("l2tpv2_set_session_ipv4 rv=%d", reply.Retval)
	}
	if isAdd {
		v.ifMgr.AddIPv4Address(swIfIndex, clientIP)
	} else {
		v.ifMgr.RemoveIPv4Address(swIfIndex, clientIP)
	}
	return nil
}

// PPPoL2TPSetSubscriberIPv6 installs (or removes) the subscriber's
// IPv6 /128 (IA_NA) route on the per-session DECAP_IP vnet interface.
func (v *VPP) PPPoL2TPSetSubscriberIPv6(swIfIndex uint32, clientIP net.IP, isAdd bool) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	ip6 := clientIP.To16()
	if ip6 == nil || clientIP.To4() != nil {
		return fmt.Errorf("invalid IPv6 address: %s", clientIP)
	}
	var addr ip_types.IP6Address
	copy(addr[:], ip6)

	req := &binapi.L2tpv2SetSessionIPv6{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		ClientIP:  addr,
		IsAdd:     isAdd,
	}
	reply := &binapi.L2tpv2SetSessionIPv6Reply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("l2tpv2_set_session_ipv6: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("l2tpv2_set_session_ipv6 rv=%d", reply.Retval)
	}
	if isAdd {
		v.ifMgr.AddIPv6Address(swIfIndex, clientIP)
	} else {
		v.ifMgr.RemoveIPv6Address(swIfIndex, clientIP)
	}
	return nil
}

// PPPoL2TPSetDelegatedPrefix installs (or removes) a delegated IPv6
// prefix routed via the per-session DECAP_IP vnet interface, with
// next-hop used as the FIB path nexthop.
func (v *VPP) PPPoL2TPSetDelegatedPrefix(swIfIndex uint32, prefix net.IPNet, nextHop net.IP, isAdd bool) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	ip6 := prefix.IP.To16()
	if ip6 == nil || prefix.IP.To4() != nil {
		return fmt.Errorf("invalid IPv6 prefix: %s", prefix.String())
	}
	prefixLen, _ := prefix.Mask.Size()

	var prefixAddr ip_types.IP6Address
	copy(prefixAddr[:], ip6)
	var nhAddr ip_types.IP6Address
	if nextHop != nil {
		nh := nextHop.To16()
		if nh == nil || nextHop.To4() != nil {
			return fmt.Errorf("invalid IPv6 next hop: %s", nextHop)
		}
		copy(nhAddr[:], nh)
	}

	req := &binapi.L2tpv2SetDelegatedPrefix{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		Prefix: ip_types.Prefix{
			Address: ip_types.Address{
				Af: ip_types.ADDRESS_IP6,
				Un: ip_types.AddressUnionIP6(prefixAddr),
			},
			Len: uint8(prefixLen),
		},
		NextHop: nhAddr,
		IsAdd:   isAdd,
	}
	reply := &binapi.L2tpv2SetDelegatedPrefixReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("l2tpv2_set_delegated_prefix: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("l2tpv2_set_delegated_prefix rv=%d", reply.Retval)
	}
	return nil
}
