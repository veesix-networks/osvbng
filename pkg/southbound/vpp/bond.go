// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package vpp

import (
	"fmt"
	"net"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/bond"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/lacp"
)

func bondModeString(m bond.BondMode) string {
	switch m {
	case bond.BOND_API_MODE_ROUND_ROBIN:
		return "round-robin"
	case bond.BOND_API_MODE_ACTIVE_BACKUP:
		return "active-backup"
	case bond.BOND_API_MODE_XOR:
		return "xor"
	case bond.BOND_API_MODE_BROADCAST:
		return "broadcast"
	case bond.BOND_API_MODE_LACP:
		return "lacp"
	default:
		return fmt.Sprintf("unknown(%d)", m)
	}
}

func bondLbAlgoString(lb bond.BondLbAlgo) string {
	switch lb {
	case bond.BOND_API_LB_ALGO_L2:
		return "l2"
	case bond.BOND_API_LB_ALGO_L23:
		return "l23"
	case bond.BOND_API_LB_ALGO_L34:
		return "l34"
	case bond.BOND_API_LB_ALGO_RR:
		return "round-robin"
	case bond.BOND_API_LB_ALGO_BC:
		return "broadcast"
	case bond.BOND_API_LB_ALGO_AB:
		return "active-backup"
	default:
		return fmt.Sprintf("unknown(%d)", lb)
	}
}

func (v *VPP) DumpBondInterfaces() ([]southbound.BondInterfaceInfo, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &bond.SwBondInterfaceDump{
		SwIfIndex: interface_types.InterfaceIndex(^uint32(0)),
	}

	stream := ch.SendMultiRequest(req)
	var result []southbound.BondInterfaceInfo

	for {
		reply := &bond.SwBondInterfaceDetails{}
		stop, err := stream.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("dump bond interfaces: %w", err)
		}

		name := strings.TrimRight(reply.InterfaceName, "\x00")
		if iface := v.ifMgr.Get(uint32(reply.SwIfIndex)); iface != nil {
			name = iface.Name
		}

		result = append(result, southbound.BondInterfaceInfo{
			SwIfIndex:     uint32(reply.SwIfIndex),
			Name:          name,
			Mode:          bondModeString(reply.Mode),
			LoadBalance:   bondLbAlgoString(reply.Lb),
			Members:       reply.Members,
			ActiveMembers: reply.ActiveMembers,
		})
	}

	return result, nil
}

func (v *VPP) DumpBondMembers(bondSwIfIndex uint32) ([]southbound.BondMemberInfo, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &bond.SwMemberInterfaceDump{
		SwIfIndex: interface_types.InterfaceIndex(bondSwIfIndex),
	}

	stream := ch.SendMultiRequest(req)
	var result []southbound.BondMemberInfo

	for {
		reply := &bond.SwMemberInterfaceDetails{}
		stop, err := stream.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("dump bond members: %w", err)
		}

		name := strings.TrimRight(reply.InterfaceName, "\x00")
		if iface := v.ifMgr.Get(uint32(reply.SwIfIndex)); iface != nil {
			name = iface.Name
		}

		result = append(result, southbound.BondMemberInfo{
			SwIfIndex:     uint32(reply.SwIfIndex),
			Name:          name,
			IsPassive:     reply.IsPassive,
			IsLongTimeout: reply.IsLongTimeout,
			IsLocalNuma:   reply.IsLocalNuma,
			Weight:        reply.Weight,
		})
	}

	return result, nil
}

func (v *VPP) DumpLACPInterfaces() ([]southbound.LACPInterfaceInfo, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &lacp.SwInterfaceLacpDump{}
	stream := ch.SendMultiRequest(req)
	var result []southbound.LACPInterfaceInfo

	for {
		reply := &lacp.SwInterfaceLacpDetails{}
		stop, err := stream.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("dump LACP interfaces: %w", err)
		}

		name := strings.TrimRight(reply.InterfaceName, "\x00")
		bondName := strings.TrimRight(reply.BondInterfaceName, "\x00")

		if iface := v.ifMgr.Get(uint32(reply.SwIfIndex)); iface != nil {
			name = iface.Name
		}

		result = append(result, southbound.LACPInterfaceInfo{
			SwIfIndex:             uint32(reply.SwIfIndex),
			Name:                  name,
			BondName:              bondName,
			RxState:               reply.RxState,
			TxState:               reply.TxState,
			MuxState:              reply.MuxState,
			PtxState:              reply.PtxState,
			ActorSystemPriority:   reply.ActorSystemPriority,
			ActorSystem:           net.HardwareAddr(reply.ActorSystem[:]).String(),
			ActorKey:              reply.ActorKey,
			ActorPortPriority:     reply.ActorPortPriority,
			ActorPortNumber:       reply.ActorPortNumber,
			ActorState:            reply.ActorState,
			PartnerSystemPriority: reply.PartnerSystemPriority,
			PartnerSystem:         net.HardwareAddr(reply.PartnerSystem[:]).String(),
			PartnerKey:            reply.PartnerKey,
			PartnerPortPriority:   reply.PartnerPortPriority,
			PartnerPortNumber:     reply.PartnerPortNumber,
			PartnerState:          reply.PartnerState,
		})
	}

	return result, nil
}
