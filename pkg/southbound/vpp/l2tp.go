// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package vpp

import (
	"fmt"
	"net"

	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
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

// AddL2TPSessionIP installs a DECAP_IP session. The plugin allocates a
// per-session vnet interface and the returned sw_if_index is what the
// caller binds for FIB / QoS / counters.
func (v *VPP) AddL2TPSessionIP(local, peer net.IP, localTunnelID, localSessionID, peerSessionID uint16, decapVrfID uint32, encapIfIndex uint32) (uint32, error) {
	return v.addL2TPSession(local, peer, localTunnelID, localSessionID, peerSessionID, l2tpDecapModeIP, "", 0, decapVrfID, encapIfIndex)
}

// AddL2TPSessionRaw installs a DECAP_RAW session. PPP frames decapsulated
// from this session are forwarded to `rawNextNode` (resolved at session-
// add time by the plugin) with `rawOpaque` stashed in the buffer for the
// downstream node to interpret.
func (v *VPP) AddL2TPSessionRaw(local, peer net.IP, localTunnelID, localSessionID, peerSessionID uint16, rawNextNode string, rawOpaque uint32, encapIfIndex uint32) error {
	_, err := v.addL2TPSession(local, peer, localTunnelID, localSessionID, peerSessionID, l2tpDecapModeRaw, rawNextNode, rawOpaque, 0, encapIfIndex)
	return err
}

func (v *VPP) addL2TPSession(local, peer net.IP, localTunnelID, localSessionID, peerSessionID uint16, mode uint8, rawNextNode string, rawOpaque, decapVrfID, encapIfIndex uint32) (uint32, error) {
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
