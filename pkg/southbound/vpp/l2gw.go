// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package vpp

import (
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/osvbng_l2gw"
)

func (v *VPP) L2GWEnableInput(ifaceName string, enable bool) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	req := &osvbng_l2gw.OsvbngL2gwEnableDisable{
		SwIfIndex: interface_types.InterfaceIndex(idx),
		Enable:    enable,
	}

	reply := &osvbng_l2gw.OsvbngL2gwEnableDisableReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("l2gw enable input: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("l2gw enable input failed: retval=%d", reply.Retval)
	}

	return nil
}

func (v *VPP) L2GWTriggerSVLANRange(ifaceName string, svlanLo, svlanHi uint16, anyProtocol, add bool) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	req := &osvbng_l2gw.OsvbngL2gwTriggerSvlanRange{
		SwIfIndex:   interface_types.InterfaceIndex(idx),
		SvlanLo:     svlanLo,
		SvlanHi:     svlanHi,
		AnyProtocol: anyProtocol,
		IsAdd:       add,
	}

	reply := &osvbng_l2gw.OsvbngL2gwTriggerSvlanRangeReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("l2gw trigger svlan range: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("l2gw trigger svlan range failed: retval=%d", reply.Retval)
	}

	return nil
}

func buildL2GWCircuitReq(circuit southbound.L2GWCircuit, isAdd bool) *osvbng_l2gw.OsvbngL2gwAddDelCircuit {
	req := &osvbng_l2gw.OsvbngL2gwAddDelCircuit{
		IsAdd:            isAdd,
		AccessSwIfIndex:  interface_types.InterfaceIndex(circuit.AccessIfIndex),
		AccessSvlan:      circuit.AccessSVLAN,
		AccessTpid:       circuit.AccessTPID,
		HandoffSwIfIndex: interface_types.InterfaceIndex(circuit.HandoffIfIndex),
		HandoffSvlan:     circuit.HandoffSVLAN,
		HandoffTpid:      circuit.HandoffTPID,
		Transparent:      circuit.Transparent,
		Enabled:          circuit.Enabled,
	}
	if circuit.AccessCVLAN == southbound.L2GWCvlanAny {
		req.AccessCvlanAny = true
	} else {
		req.AccessCvlan = circuit.AccessCVLAN
	}
	if circuit.HandoffCVLAN == southbound.L2GWCvlanAny {
		req.HandoffCvlanAny = true
	} else {
		req.HandoffCvlan = circuit.HandoffCVLAN
	}
	return req
}

func (v *VPP) AddL2GWCircuit(circuit southbound.L2GWCircuit) (uint32, uint32, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return 0, 0, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := buildL2GWCircuitReq(circuit, true)
	reply := &osvbng_l2gw.OsvbngL2gwAddDelCircuitReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return 0, 0, fmt.Errorf("add l2gw circuit: %w", err)
	}

	if reply.Retval == retvalEntryNeedsRefresh {
		v.logger.Info("L2GW circuit exists with drifted parameters; refreshing",
			"access_if_index", circuit.AccessIfIndex,
			"access_svlan", circuit.AccessSVLAN,
			"access_cvlan", circuit.AccessCVLAN,
			"stale_circuit_id", reply.CircuitID)

		delReq := buildL2GWCircuitReq(circuit, false)
		delReply := &osvbng_l2gw.OsvbngL2gwAddDelCircuitReply{}
		if err := ch.SendRequest(delReq).ReceiveReply(delReply); err != nil {
			return 0, 0, fmt.Errorf("refresh l2gw circuit: del: %w", err)
		}
		if delReply.Retval != 0 {
			return 0, 0, fmt.Errorf("refresh l2gw circuit: del retval=%d", delReply.Retval)
		}

		reply = &osvbng_l2gw.OsvbngL2gwAddDelCircuitReply{}
		if err := ch.SendRequest(buildL2GWCircuitReq(circuit, true)).ReceiveReply(reply); err != nil {
			return 0, 0, fmt.Errorf("refresh l2gw circuit: re-add: %w", err)
		}
	}

	if reply.Retval != 0 {
		return 0, 0, fmt.Errorf("add l2gw circuit failed: retval=%d", reply.Retval)
	}

	return reply.CircuitID, reply.HandoffEntryIndex, nil
}

func (v *VPP) DelL2GWCircuit(circuit southbound.L2GWCircuit) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := buildL2GWCircuitReq(circuit, false)
	reply := &osvbng_l2gw.OsvbngL2gwAddDelCircuitReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("del l2gw circuit: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("del l2gw circuit failed: retval=%d", reply.Retval)
	}

	return nil
}

func (v *VPP) SetL2GWCircuitState(circuitID uint32, enabled bool) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &osvbng_l2gw.OsvbngL2gwCircuitSetState{
		CircuitID: circuitID,
		Enabled:   enabled,
	}

	reply := &osvbng_l2gw.OsvbngL2gwCircuitSetStateReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("set l2gw circuit state: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("set l2gw circuit state failed: retval=%d", reply.Retval)
	}

	return nil
}

func (v *VPP) DumpL2GWCircuits() ([]southbound.L2GWCircuitDetails, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	reqCtx := ch.SendMultiRequest(&osvbng_l2gw.OsvbngL2gwCircuitDump{
		CircuitID: ^uint32(0),
	})

	var circuits []southbound.L2GWCircuitDetails
	for {
		details := &osvbng_l2gw.OsvbngL2gwCircuitDetails{}
		stop, err := reqCtx.ReceiveReply(details)
		if err != nil {
			return nil, fmt.Errorf("dump l2gw circuits: %w", err)
		}
		if stop {
			break
		}

		c := southbound.L2GWCircuitDetails{
			CircuitID:         details.CircuitID,
			AccessEntryIndex:  details.AccessEntryIndex,
			HandoffEntryIndex: details.HandoffEntryIndex,
			L2GWCircuit: southbound.L2GWCircuit{
				AccessIfIndex:  uint32(details.AccessSwIfIndex),
				AccessSVLAN:    details.AccessSvlan,
				AccessCVLAN:    details.AccessCvlan,
				AccessTPID:     details.AccessTpid,
				HandoffIfIndex: uint32(details.HandoffSwIfIndex),
				HandoffSVLAN:   details.HandoffSvlan,
				HandoffCVLAN:   details.HandoffCvlan,
				HandoffTPID:    details.HandoffTpid,
				Transparent:    details.Transparent,
				Enabled:        details.Enabled,
			},
		}
		if details.AccessCvlanAny {
			c.AccessCVLAN = southbound.L2GWCvlanAny
		}
		if details.HandoffCvlanAny {
			c.HandoffCVLAN = southbound.L2GWCvlanAny
		}
		circuits = append(circuits, c)
	}

	return circuits, nil
}
