// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package vpp

import (
	"fmt"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/osvbng_tunnel"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/vxlan"
	"go.fd.io/govpp/api"
)

const vxlanDefaultPort = 4789

func (v *VPP) tunnelDecapNext(isIPv6 bool) (uint32, error) {
	v.tunnelDecapMu.Lock()
	defer v.tunnelDecapMu.Unlock()

	if !v.tunnelDecapLoaded {
		ch, err := v.conn.NewAPIChannel()
		if err != nil {
			return 0, fmt.Errorf("create API channel: %w", err)
		}
		defer ch.Close()

		req := &osvbng_tunnel.OsvbngTunnelDecapNextGet{}
		reply := &osvbng_tunnel.OsvbngTunnelDecapNextGetReply{}
		if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
			return 0, fmt.Errorf("tunnel decap next get: %w", err)
		}
		if reply.Retval != 0 {
			return 0, fmt.Errorf("tunnel decap next get failed: retval=%d", reply.Retval)
		}

		v.tunnelDecap4 = reply.Vxlan4Next
		v.tunnelDecap6 = reply.Vxlan6Next
		v.tunnelDecapLoaded = true
	}

	next := v.tunnelDecap4
	if isIPv6 {
		next = v.tunnelDecap6
	}
	if next == ^uint32(0) {
		return 0, fmt.Errorf("vxlan plugin not loaded in VPP")
	}
	return next, nil
}

func (v *VPP) createVxlanTunnel(cfg *interfaces.InterfaceConfig) error {
	if cfg.Vxlan == nil {
		return fmt.Errorf("interface %s has type vxlan but no vxlan config", cfg.Name)
	}
	if err := cfg.Vxlan.Validate(); err != nil {
		return fmt.Errorf("invalid vxlan config for %s: %w", cfg.Name, err)
	}
	if cfg.Vxlan.Src == "" {
		return fmt.Errorf("vxlan src for %s not resolved from src-interface %q", cfg.Name, cfg.Vxlan.SrcInterface)
	}
	if cfg.Vxlan.Dst == "" {
		return fmt.Errorf("vxlan tunnel %s has no dst (evpn-signaled tunnels are programmed on discovery)", cfg.Name)
	}

	if existing := v.ifMgr.GetByName(cfg.Name); existing != nil {
		if !strings.EqualFold(existing.DevType, "vxlan") {
			return fmt.Errorf("vxlan tunnel %q conflicts with existing %s interface", cfg.Name, existing.DevType)
		}
		v.logger.Info("VXLAN tunnel already exists in VPP, skipping creation", "interface", cfg.Name, "sw_if_index", existing.SwIfIndex)
		if cfg.Enabled {
			if err := v.setInterfaceState(cfg.Name, true); err != nil {
				v.logger.Warn("Failed to set interface up", "interface", cfg.Name, "error", err)
			}
		}
		return nil
	}

	src, err := ip_types.ParseAddress(cfg.Vxlan.Src)
	if err != nil {
		return fmt.Errorf("parse vxlan src %q: %w", cfg.Vxlan.Src, err)
	}
	dst, err := ip_types.ParseAddress(cfg.Vxlan.Dst)
	if err != nil {
		return fmt.Errorf("parse vxlan dst %q: %w", cfg.Vxlan.Dst, err)
	}

	var decapNext uint32
	if cfg.Vxlan.PWTransport {
		decapNext, err = v.pwDecapNext(dst.Af == ip_types.ADDRESS_IP6)
	} else {
		decapNext, err = v.tunnelDecapNext(dst.Af == ip_types.ADDRESS_IP6)
	}
	if err != nil {
		return fmt.Errorf("resolve tunnel decap next: %w", err)
	}

	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &vxlan.VxlanAddDelTunnelV3{
		IsAdd:          true,
		Instance:       ^uint32(0),
		SrcAddress:     src,
		DstAddress:     dst,
		SrcPort:        vxlanDefaultPort,
		DstPort:        vxlanDefaultPort,
		McastSwIfIndex: interface_types.InterfaceIndex(^uint32(0)),
		EncapVrfID:     0,
		DecapNextIndex: decapNext,
		Vni:            cfg.Vxlan.VNI,
		IsL3:           false,
	}
	reply := &vxlan.VxlanAddDelTunnelV3Reply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("create vxlan tunnel: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("create vxlan tunnel failed: retval=%d", reply.Retval)
	}
	swIfIndex := uint32(reply.SwIfIndex)

	rollback := func() {
		delReq := *req
		delReq.IsAdd = false
		delReply := &vxlan.VxlanAddDelTunnelV3Reply{}
		if err := ch.SendRequest(&delReq).ReceiveReply(delReply); err != nil {
			v.logger.Warn("Failed to roll back vxlan tunnel", "interface", cfg.Name, "error", err)
		}
	}

	if err := v.LoadInterfaces(); err != nil {
		rollback()
		return fmt.Errorf("reload interfaces after vxlan create: %w", err)
	}

	iface := v.ifMgr.Get(swIfIndex)
	if iface == nil {
		rollback()
		return fmt.Errorf("vxlan tunnel %s not found after creation", cfg.Name)
	}

	if iface.Name != cfg.Name {
		if err := v.renameVPPInterface(iface.Name, cfg.Name); err != nil {
			rollback()
			return fmt.Errorf("rename vxlan tunnel: %w", err)
		}
	}

	if cfg.Description != "" {
		v.SetInterfaceDescription(cfg.Name, cfg.Description)
	}

	if cfg.VRF != "" {
		v.logger.Warn("VRF binding is not supported on vxlan tunnel interfaces, ignoring", "interface", cfg.Name, "vrf", cfg.VRF)
	}

	if cfg.Enabled {
		if err := v.setInterfaceState(cfg.Name, true); err != nil {
			v.logger.Warn("Failed to set interface up", "interface", cfg.Name, "error", err)
		}
	}

	v.logger.Info("Created VXLAN tunnel", "interface", cfg.Name, "sw_if_index", swIfIndex,
		"src", cfg.Vxlan.Src, "dst", cfg.Vxlan.Dst, "vni", cfg.Vxlan.VNI, "decap_next", decapNext)

	return nil
}

func (v *VPP) deleteVxlanTunnel(ch api.Channel, iface *ifmgr.Interface) error {
	dumpReq := &vxlan.VxlanTunnelV2Dump{
		SwIfIndex: interface_types.InterfaceIndex(iface.SwIfIndex),
	}

	var details *vxlan.VxlanTunnelV2Details
	stream := ch.SendMultiRequest(dumpReq)
	for {
		reply := &vxlan.VxlanTunnelV2Details{}
		stop, err := stream.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return fmt.Errorf("dump vxlan tunnel: %w", err)
		}
		if uint32(reply.SwIfIndex) == iface.SwIfIndex {
			details = reply
		}
	}

	if details == nil {
		return fmt.Errorf("vxlan tunnel %s not found in VPP", iface.Name)
	}

	req := &vxlan.VxlanAddDelTunnelV3{
		IsAdd:          false,
		Instance:       details.Instance,
		SrcAddress:     details.SrcAddress,
		DstAddress:     details.DstAddress,
		SrcPort:        details.SrcPort,
		DstPort:        details.DstPort,
		McastSwIfIndex: details.McastSwIfIndex,
		EncapVrfID:     details.EncapVrfID,
		DecapNextIndex: details.DecapNextIndex,
		Vni:            details.Vni,
	}
	reply := &vxlan.VxlanAddDelTunnelV3Reply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("delete vxlan tunnel: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("delete vxlan tunnel failed: retval=%d", reply.Retval)
	}

	v.ifMgr.Remove(iface.SwIfIndex)
	v.logger.Debug("Deleted VXLAN tunnel", "interface", iface.Name)
	return nil
}
