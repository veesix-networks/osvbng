// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package interfaces

type InterfaceSummary struct {
	Name      string `json:"name"`
	AdminUp   bool   `json:"admin_up"`
	LinkUp    bool   `json:"link_up"`
	MTU       uint32 `json:"mtu"`
	Type      string `json:"type"`
	RxPackets uint64 `json:"rx_packets"`
	RxBytes   uint64 `json:"rx_bytes"`
	RxErrors  uint64 `json:"rx_errors"`
	TxPackets uint64 `json:"tx_packets"`
	TxBytes   uint64 `json:"tx_bytes"`
	TxErrors  uint64 `json:"tx_errors"`
	Drops     uint64 `json:"drops"`
}

type InterfaceDetail struct {
	Name          string   `json:"name"`
	SwIfIndex     uint32   `json:"sw_if_index"`
	AdminUp       bool     `json:"admin_up"`
	LinkUp        bool     `json:"link_up"`
	MTU           uint32   `json:"mtu"`
	MAC           string   `json:"mac"`
	LinkSpeed     uint32   `json:"link_speed"`
	Description   string   `json:"description,omitempty"`
	FIBTableID    uint32   `json:"fib_table_id"`
	IPv4Addresses []string `json:"ipv4_addresses,omitempty"`
	IPv6Addresses []string `json:"ipv6_addresses,omitempty"`

	Stats *InterfaceDetailStats `json:"stats,omitempty"`

	Bond         *BondSection         `json:"bond,omitempty"`
	Subinterface *SubinterfaceSection `json:"subinterface,omitempty"`
}

type InterfaceDetailStats struct {
	RxPackets uint64 `json:"rx_packets"`
	RxBytes   uint64 `json:"rx_bytes"`
	RxErrors  uint64 `json:"rx_errors"`
	TxPackets uint64 `json:"tx_packets"`
	TxBytes   uint64 `json:"tx_bytes"`
	TxErrors  uint64 `json:"tx_errors"`
	Drops     uint64 `json:"drops"`
}

type BondSection struct {
	Mode          string           `json:"mode"`
	LoadBalance   string           `json:"load_balance"`
	Members       uint32           `json:"members"`
	ActiveMembers uint32           `json:"active_members"`
	MemberDetails []BondMemberInfo `json:"member_details,omitempty"`
	LACP          []LACPMemberInfo `json:"lacp,omitempty"`
}

type BondMemberInfo struct {
	Name          string `json:"name"`
	SwIfIndex     uint32 `json:"sw_if_index"`
	IsPassive     bool   `json:"is_passive"`
	IsLongTimeout bool   `json:"is_long_timeout"`
	IsLocalNuma   bool   `json:"is_local_numa"`
	Weight        uint32 `json:"weight"`
}

type LACPMemberInfo struct {
	Name                  string `json:"name"`
	SwIfIndex             uint32 `json:"sw_if_index"`
	RxState               uint32 `json:"rx_state"`
	TxState               uint32 `json:"tx_state"`
	MuxState              uint32 `json:"mux_state"`
	PtxState              uint32 `json:"ptx_state"`
	ActorSystemPriority   uint16 `json:"actor_system_priority"`
	ActorSystem           string `json:"actor_system"`
	ActorKey              uint16 `json:"actor_key"`
	ActorPortPriority     uint16 `json:"actor_port_priority"`
	ActorPortNumber       uint16 `json:"actor_port_number"`
	ActorState            uint8  `json:"actor_state"`
	PartnerSystemPriority uint16 `json:"partner_system_priority"`
	PartnerSystem         string `json:"partner_system"`
	PartnerKey            uint16 `json:"partner_key"`
	PartnerPortPriority   uint16 `json:"partner_port_priority"`
	PartnerPortNumber     uint16 `json:"partner_port_number"`
	PartnerState          uint8  `json:"partner_state"`
}

type SubinterfaceSection struct {
	Parent          string `json:"parent"`
	SubID           uint32 `json:"sub_id"`
	SubNumberOfTags uint8  `json:"sub_number_of_tags"`
	OuterVlanID     uint16 `json:"outer_vlan_id,omitempty"`
	InnerVlanID     uint16 `json:"inner_vlan_id,omitempty"`
}
