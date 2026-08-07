// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package vpp

import (
	"fmt"
	"net"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	vppinterfaces "github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/osvbng_tunnel"
)

type pwBinding struct {
	transport        string
	tunnelSwIfIndex  uint32
	headendSwIfIndex uint32
}

func (v *VPP) pwDecapNext(isIPv6 bool) (uint32, error) {
	v.tunnelDecapMu.Lock()
	defer v.tunnelDecapMu.Unlock()

	if !v.pwDecapLoaded {
		ch, err := v.conn.NewAPIChannel()
		if err != nil {
			return 0, fmt.Errorf("create API channel: %w", err)
		}
		defer ch.Close()

		req := &osvbng_tunnel.OsvbngPwDecapNextGet{}
		reply := &osvbng_tunnel.OsvbngPwDecapNextGetReply{}
		if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
			return 0, fmt.Errorf("pw decap next get: %w", err)
		}
		if reply.Retval != 0 {
			return 0, fmt.Errorf("pw decap next get failed: retval=%d", reply.Retval)
		}

		v.pwDecap4 = reply.Vxlan4Next
		v.pwDecap6 = reply.Vxlan6Next
		v.pwDecapLoaded = true
	}

	next := v.pwDecap4
	if isIPv6 {
		next = v.pwDecap6
	}
	if next == ^uint32(0) {
		return 0, fmt.Errorf("vxlan plugin not loaded in VPP")
	}
	return next, nil
}

func (v *VPP) createPseudowire(cfg *interfaces.InterfaceConfig) error {
	if cfg.Pseudowire == nil {
		return fmt.Errorf("interface %s has type pseudowire but no pseudowire config", cfg.Name)
	}
	if err := cfg.Pseudowire.Validate(); err != nil {
		return fmt.Errorf("invalid pseudowire config for %s: %w", cfg.Name, err)
	}

	if existing := v.ifMgr.GetByName(cfg.Name); existing != nil {
		if !strings.EqualFold(existing.DevType, "loopback") {
			return fmt.Errorf("pseudowire %q conflicts with existing %s interface", cfg.Name, existing.DevType)
		}
		v.logger.Info("Pseudowire headend already exists in VPP, skipping creation", "interface", cfg.Name, "sw_if_index", existing.SwIfIndex)
		if cfg.Enabled {
			if err := v.setInterfaceState(cfg.Name, true); err != nil {
				v.logger.Warn("Failed to set interface up", "interface", cfg.Name, "error", err)
			}
		}
		return nil
	}

	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &vppinterfaces.CreateLoopbackInstance{
		IsSpecified: false,
	}
	if cfg.Pseudowire.MACAddress != "" {
		mac, err := net.ParseMAC(cfg.Pseudowire.MACAddress)
		if err != nil {
			return fmt.Errorf("parse pseudowire mac-address: %w", err)
		}
		copy(req.MacAddress[:], mac)
	}
	reply := &vppinterfaces.CreateLoopbackInstanceReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("create pseudowire headend: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("create pseudowire headend failed: retval=%d", reply.Retval)
	}
	swIfIndex := uint32(reply.SwIfIndex)

	rollback := func() {
		delReq := &vppinterfaces.DeleteLoopback{
			SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		}
		delReply := &vppinterfaces.DeleteLoopbackReply{}
		if err := ch.SendRequest(delReq).ReceiveReply(delReply); err != nil {
			v.logger.Warn("Failed to roll back pseudowire headend", "interface", cfg.Name, "error", err)
		}
	}

	if err := v.LoadInterfaces(); err != nil {
		rollback()
		return fmt.Errorf("reload interfaces after pseudowire create: %w", err)
	}

	iface := v.ifMgr.Get(swIfIndex)
	if iface == nil {
		rollback()
		return fmt.Errorf("pseudowire headend %s not found after creation", cfg.Name)
	}

	if iface.Name != cfg.Name {
		if err := v.renameVPPInterface(iface.Name, cfg.Name); err != nil {
			rollback()
			return fmt.Errorf("rename pseudowire headend: %w", err)
		}
	}

	if cfg.Description != "" {
		v.SetInterfaceDescription(cfg.Name, cfg.Description)
	}

	if cfg.Enabled {
		if err := v.setInterfaceState(cfg.Name, true); err != nil {
			v.logger.Warn("Failed to set interface up", "interface", cfg.Name, "error", err)
		}
	}

	v.logger.Info("Created pseudowire headend", "interface", cfg.Name, "sw_if_index", swIfIndex, "transport", cfg.Pseudowire.Transport)

	return nil
}

func (v *VPP) BindPseudowire(name, transport string) error {
	headendIdx, err := v.GetInterfaceIndex(name)
	if err != nil {
		return fmt.Errorf("pseudowire %s not found: %w", name, err)
	}
	tunnelIdx, err := v.GetInterfaceIndex(transport)
	if err != nil {
		return fmt.Errorf("pseudowire transport %s not found: %w", transport, err)
	}

	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &osvbng_tunnel.OsvbngPwBind{
		TunnelSwIfIndex:  uint32(tunnelIdx),
		HeadendSwIfIndex: uint32(headendIdx),
		IsBind:           true,
	}
	reply := &osvbng_tunnel.OsvbngPwBindReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("pw bind: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("pw bind failed: retval=%d", reply.Retval)
	}

	v.pwMu.Lock()
	v.pwBindings[name] = pwBinding{
		transport:        transport,
		tunnelSwIfIndex:  uint32(tunnelIdx),
		headendSwIfIndex: uint32(headendIdx),
	}
	v.pwMu.Unlock()

	v.logger.Info("Bound pseudowire to transport", "interface", name, "transport", transport)
	return nil
}

func (v *VPP) deletePseudowire(iface *ifmgr.Interface) error {
	v.pwMu.Lock()
	binding, bound := v.pwBindings[iface.Name]
	delete(v.pwBindings, iface.Name)
	v.pwMu.Unlock()

	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	if bound {
		req := &osvbng_tunnel.OsvbngPwBind{
			TunnelSwIfIndex:  binding.tunnelSwIfIndex,
			HeadendSwIfIndex: binding.headendSwIfIndex,
			IsBind:           false,
		}
		reply := &osvbng_tunnel.OsvbngPwBindReply{}
		if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
			return fmt.Errorf("pw unbind: %w", err)
		}
		if reply.Retval != 0 {
			v.logger.Warn("PW unbind failed, deleting headend anyway", "interface", iface.Name, "retval", reply.Retval)
		}
	}

	req := &vppinterfaces.DeleteLoopback{
		SwIfIndex: interface_types.InterfaceIndex(iface.SwIfIndex),
	}
	reply := &vppinterfaces.DeleteLoopbackReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("delete pseudowire headend: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("delete pseudowire headend failed: retval=%d", reply.Retval)
	}

	v.ifMgr.Remove(iface.SwIfIndex)
	v.logger.Debug("Deleted pseudowire headend", "interface", iface.Name)
	return nil
}
