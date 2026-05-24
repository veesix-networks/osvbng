// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package vpp

import (
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/urpf"
)

// EnableSourceVerify turns on uRPF on swIfIndex for both IPv4 and IPv6 in
// the inbound direction. mode is strict by default; pass strict=false for
// loose-mode (any FIB entry matches, not necessarily the ingress
// interface). VPP's urpf_update API is set-or-replace, so re-applying with
// the same mode is a free no-op.
func (v *VPP) EnableSourceVerify(swIfIndex uint32, strict bool) error {
	mode := urpf.URPF_API_MODE_STRICT
	if !strict {
		mode = urpf.URPF_API_MODE_LOOSE
	}
	if err := v.setURPF(swIfIndex, ip_types.ADDRESS_IP4, mode); err != nil {
		return fmt.Errorf("urpf ip4 enable: %w", err)
	}
	if err := v.setURPF(swIfIndex, ip_types.ADDRESS_IP6, mode); err != nil {
		return fmt.Errorf("urpf ip6 enable: %w", err)
	}
	return nil
}

// DisableSourceVerify turns off uRPF on swIfIndex for both AFs in the
// inbound direction.
func (v *VPP) DisableSourceVerify(swIfIndex uint32) error {
	if err := v.setURPF(swIfIndex, ip_types.ADDRESS_IP4, urpf.URPF_API_MODE_OFF); err != nil {
		return fmt.Errorf("urpf ip4 disable: %w", err)
	}
	if err := v.setURPF(swIfIndex, ip_types.ADDRESS_IP6, urpf.URPF_API_MODE_OFF); err != nil {
		return fmt.Errorf("urpf ip6 disable: %w", err)
	}
	return nil
}

func (v *VPP) setURPF(swIfIndex uint32, af ip_types.AddressFamily, mode urpf.UrpfMode) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &urpf.UrpfUpdate{
		IsInput:   true,
		Mode:      mode,
		Af:        af,
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
	}
	reply := &urpf.UrpfUpdateReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("urpf_update: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("urpf_update retval=%d", reply.Retval)
	}
	return nil
}
