// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package models

import "net"

type CGNATMapping struct {
	SessionID      string `json:"session_id,omitempty"`
	PoolName       string `json:"pool_name"`
	PoolID         uint32 `json:"pool_id"`
	InsideIP       net.IP `json:"inside_ip"`
	InsideVRFID    uint32 `json:"inside_vrf_id"`
	OutsideIP      net.IP `json:"outside_ip"`
	PortBlockStart uint16 `json:"port_block_start"`
	PortBlockEnd   uint16 `json:"port_block_end"`
	SwIfIndex      uint32 `json:"sw_if_index"`
}

type CGNATPoolStats struct {
	Name               string  `json:"name"               metric:"label"`
	Mode               string  `json:"mode"               metric:"label"`
	TotalAddresses     uint32  `json:"total_addresses"    metric:"name=cgnat.pool.addresses_total,type=gauge,help=Total addresses in this CGNAT pool."`
	AllocatedAddresses uint32  `json:"allocated_addresses" metric:"name=cgnat.pool.addresses_allocated,type=gauge,help=Allocated addresses in this CGNAT pool."`
	FreeBlocks         uint32  `json:"free_blocks"        metric:"name=cgnat.pool.blocks_free,type=gauge,help=Free port-blocks in this CGNAT pool."`
	TotalBlocks        uint32  `json:"total_blocks"       metric:"name=cgnat.pool.blocks_total,type=gauge,help=Total port-blocks in this CGNAT pool."`
	ExcludedAddresses  uint32  `json:"excluded_addresses" metric:"name=cgnat.pool.addresses_excluded,type=gauge,help=Excluded addresses in this CGNAT pool."`
	SubscriberCount    uint32  `json:"subscriber_count"   metric:"name=cgnat.pool.subscribers,type=gauge,help=Subscribers mapped to this CGNAT pool."`
	Utilization        float64 `json:"utilization"        metric:"name=cgnat.pool.utilization,type=gauge,help=CGNAT pool utilization (0.0 to 1.0)."`
}

type CGNATSessionInfo struct {
	OutsideIP net.IP `json:"outside_ip"`
	PortStart uint16 `json:"port_start"`
	PortEnd   uint16 `json:"port_end"`
	Pool      string `json:"pool"`
	Mode      string `json:"mode"`
}

// CGNATSession is one active NAT translation (a 5-tuple flow). Ports are
// host-order; for ICMP, inside_port/outside_port carry the ICMP identifier and
// remote_port is 0.
type CGNATSession struct {
	PoolName       string  `json:"pool_name"`
	PoolID         uint32  `json:"pool_id"`
	InsideIP       net.IP  `json:"inside_ip"`
	InsidePort     uint16  `json:"inside_port" description:"Subscriber port; for ICMP this is the ICMP identifier."`
	OutsideIP      net.IP  `json:"outside_ip"`
	OutsidePort    uint16  `json:"outside_port" description:"Translated port; for ICMP this is the translated ICMP identifier."`
	RemoteIP       net.IP  `json:"remote_ip"`
	RemotePort     uint16  `json:"remote_port" description:"Remote peer port; 0 for ICMP."`
	Proto          string  `json:"proto"`
	ALGFlags       uint8   `json:"alg_flags"`
	Packets        uint64  `json:"packets"`
	Bytes          uint64  `json:"bytes"`
	AgeSeconds     float64 `json:"age_seconds"`
	TimeoutSeconds uint32  `json:"timeout_seconds"`
}

// CGNATSessionPage is the result of a session dump. It is returned as a struct
// (not a bare slice) so the northbound passes it through without re-paginating:
// the plugin already filtered and windowed the result. Total is the global live
// session count (O(1), not filter-scoped); NextCursor/HasMore drive paging.
type CGNATSessionPage struct {
	Sessions   []CGNATSession `json:"sessions"`
	Total      uint64         `json:"total"`
	Returned   int            `json:"returned"`
	NextCursor uint32         `json:"next_cursor"`
	HasMore    bool           `json:"has_more"`
}

// CGNATSessionFilter is the validated, typed filter the show handler hands to
// the component (which translates it to a southbound filter). Zero values mean
// "no filter"; Cursor/Limit drive backend windowing.
type CGNATSessionFilter struct {
	InsideIP    net.IP
	OutsideIP   net.IP
	RemoteIP    net.IP
	InsidePort  uint16
	OutsidePort uint16
	RemotePort  uint16
	Proto       uint8
	PoolID      uint32
	Cursor      uint32
	Limit       uint32
}

type CGNATBypassEntry struct {
	Prefix      string `json:"prefix"`
	InsideVRFID uint32 `json:"inside_vrf_id"`
}
