// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package southbound

// L2GWCvlanAny selects the per-S-VLAN wildcard on a circuit side: the
// circuit matches any (or no) inner VLAN and inner tags pass through
// untouched.
const L2GWCvlanAny = 0xFFFF

// L2GWCircuit describes one bidirectional wholesale circuit between an
// access port and a handoff port. VLAN values are the on-wire tags on
// each side; TPIDs are the outer TPID emitted toward that side (0 =
// dot1ad default).
type L2GWCircuit struct {
	AccessIfIndex  uint32
	AccessSVLAN    uint16
	AccessCVLAN    uint16
	AccessTPID     uint16
	HandoffIfIndex uint32
	HandoffSVLAN   uint16
	HandoffCVLAN   uint16
	HandoffTPID    uint16
	Transparent    bool
	Enabled        bool
}

type L2GW interface {
	L2GWEnableInput(ifaceName string, enable bool) error
	// L2GWTriggerSVLANRange arms (or disarms) the dataplane trigger
	// snoop for an S-VLAN range on an access port: on circuit miss,
	// frames in armed S-VLANs are punted to the control plane. With
	// anyProtocol false only DHCPv4/DHCPv6 punt; with anyProtocol true
	// the first frame of any ethertype punts, gated by the plugin's
	// per-tuple dampener.
	L2GWTriggerSVLANRange(ifaceName string, svlanLo, svlanHi uint16, anyProtocol, add bool) error
	// AddL2GWCircuit returns (circuit id == access-direction counter
	// index, handoff-direction counter index) in the /osvbng/l2gw stats
	// segment.
	AddL2GWCircuit(circuit L2GWCircuit) (uint32, uint32, error)
	DelL2GWCircuit(circuit L2GWCircuit) error
	SetL2GWCircuitState(circuitID uint32, enabled bool) error
	DumpL2GWCircuits() ([]L2GWCircuitDetails, error)
	GetL2GWStats() (map[uint32]L2GWEntryStats, error)
}

// L2GWEntryStats is one direction's cumulative counters from the
// /osvbng/l2gw stats segment, summed across workers.
type L2GWEntryStats struct {
	Packets uint64
	Bytes   uint64
}

// L2GWCircuitDetails is one circuit from the dataplane dump. The entry
// indices are the per-direction counter indices in the /osvbng/l2gw
// stats segment.
type L2GWCircuitDetails struct {
	CircuitID         uint32
	AccessEntryIndex  uint32
	HandoffEntryIndex uint32
	L2GWCircuit
}
