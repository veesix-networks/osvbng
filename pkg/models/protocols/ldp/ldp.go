// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

// Package ldp holds typed Go structures matching FRRouting's vtysh JSON
// output for MPLS LDP show commands.
package ldp

// Neighbor's UpTimeSecs is a synthetic field; the routing layer parses
// FRR's HH:MM:SS UpTime string into seconds for telemetry emission.
type Neighbor struct {
	AddressFamily    string `json:"addressFamily" metric:"label=afi"`
	NeighborId       string `json:"neighborId" metric:"label=neighbor_id"`
	State            string `json:"state" metric:"label"`
	TransportAddress string `json:"transportAddress,omitempty"`
	UpTime           string `json:"upTime,omitempty"`
	UpTimeSecs       uint64 `json:"-" metric:"name=protocols.ldp.neighbor.uptime_seconds,type=gauge,help=LDP neighbor session uptime in seconds."`
}

// Binding's LocalLabel/RemoteLabel are strings because FRR uses
// sentinels like "imp-null" alongside numeric labels.
type Binding struct {
	AddressFamily string `json:"addressFamily" metric:"label=afi"`
	Prefix        string `json:"prefix" metric:"label"`
	NeighborId    string `json:"neighborId" metric:"label=neighbor_id"`
	LocalLabel    string `json:"localLabel,omitempty"`
	RemoteLabel   string `json:"remoteLabel,omitempty"`
	InUse         uint64 `json:"inUse" metric:"name=protocols.ldp.binding.in_use,type=gauge,help=1 if this LDP label binding is installed in the dataplane, 0 otherwise."`
}

type Discovery struct {
	AddressFamily string `json:"addressFamily" metric:"label=afi"`
	NeighborId    string `json:"neighborId" metric:"label=neighbor_id"`
	Type          string `json:"type" metric:"label"`
	Interface     string `json:"interface" metric:"label"`
	HelloHoldtime uint32 `json:"helloHoldtime" metric:"name=protocols.ldp.discovery.hello_holdtime_seconds,type=gauge,help=LDP adjacency hello holdtime in seconds."`
}
