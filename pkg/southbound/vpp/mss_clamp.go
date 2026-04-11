// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package vpp

import (
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/mss_clamp"
)

// VPP's mss_clamp plugin treats the direction enum as a bitmask: a single
// MssClampEnableDisable call with direction = MSS_CLAMP_DIR_RX|MSS_CLAMP_DIR_TX
// programs both directions. Verified against vpp/src/plugins/mss_clamp/mss_clamp.c
// (lines 23-33 use bitwise AND on the direction; line 111 sets the CLI default to
// RX|TX). One API call per family is sufficient.
const mssClampDirBoth = mss_clamp.MssClampDir(uint8(mss_clamp.MSS_CLAMP_DIR_RX) | uint8(mss_clamp.MSS_CLAMP_DIR_TX))

func (v *VPP) EnableMSSClamp(swIfIndex uint32, policy southbound.MSSClampPolicy) error {
	if !policy.Enabled {
		return v.DisableMSSClamp(swIfIndex)
	}
	if policy.IPv4MSS == 0 && policy.IPv6MSS == 0 {
		return fmt.Errorf("mss clamp enabled with both IPv4MSS and IPv6MSS = 0")
	}

	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &mss_clamp.MssClampEnableDisable{
		SwIfIndex:     interface_types.InterfaceIndex(swIfIndex),
		IPv4Mss:       policy.IPv4MSS,
		IPv6Mss:       policy.IPv6MSS,
		IPv4Direction: directionFor(policy.IPv4MSS),
		IPv6Direction: directionFor(policy.IPv6MSS),
	}

	reply := &mss_clamp.MssClampEnableDisableReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("mss_clamp enable sw_if_index=%d: %w", swIfIndex, err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("mss_clamp enable sw_if_index=%d: retval=%d", swIfIndex, reply.Retval)
	}

	v.logger.Debug("Programmed MSS clamp",
		"sw_if_index", swIfIndex,
		"ipv4_mss", policy.IPv4MSS,
		"ipv6_mss", policy.IPv6MSS)
	return nil
}

func (v *VPP) DisableMSSClamp(swIfIndex uint32) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &mss_clamp.MssClampEnableDisable{
		SwIfIndex:     interface_types.InterfaceIndex(swIfIndex),
		IPv4Mss:       0,
		IPv6Mss:       0,
		IPv4Direction: mss_clamp.MSS_CLAMP_DIR_NONE,
		IPv6Direction: mss_clamp.MSS_CLAMP_DIR_NONE,
	}

	reply := &mss_clamp.MssClampEnableDisableReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("mss_clamp disable sw_if_index=%d: %w", swIfIndex, err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("mss_clamp disable sw_if_index=%d: retval=%d", swIfIndex, reply.Retval)
	}

	v.logger.Debug("Disabled MSS clamp", "sw_if_index", swIfIndex)
	return nil
}

func directionFor(mss uint16) mss_clamp.MssClampDir {
	if mss == 0 {
		return mss_clamp.MSS_CLAMP_DIR_NONE
	}
	return mssClampDirBoth
}
