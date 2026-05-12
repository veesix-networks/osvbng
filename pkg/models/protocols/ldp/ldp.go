// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

// Package ldp holds typed Go structures matching FRRouting's vtysh JSON
// output for MPLS LDP show commands. Used by internal/routing to parse
// FRR responses, and tagged with `metric:"..."` so the show handlers
// register telemetry via pkg/telemetry.RegisterMetric[T].
//
// Modelling principle (osvbng-context #59 D2): only fields used for
// metrics or rendered by CLI/API consumers. Unmodeled FRR JSON keys are
// silently dropped on json.Unmarshal. See osvbng-context shapes/ldp.md
// for the captured FRR command shapes and field inventory rationale.
//
// FRR version: shapes authored against FRRouting 10.5.3.
package ldp

// Neighbor matches one entry in the `neighbors` array from
// `show mpls ldp neighbor json`. UpTimeSecs is a synthetic numeric
// representation of FRR's HH:MM:SS UpTime string, parsed by the routing
// layer for telemetry emission.
type Neighbor struct {
	AddressFamily    string `json:"addressFamily" metric:"label"`
	NeighborId       string `json:"neighborId" metric:"label"`
	State            string `json:"state" metric:"label"`
	TransportAddress string `json:"transportAddress,omitempty"` // show-output only
	UpTime           string `json:"upTime,omitempty"`           // show-output (human-readable)
	UpTimeSecs       uint64 `json:"-" metric:"name=protocols.ldp.neighbor.uptime_seconds,type=gauge,help=LDP neighbor session uptime in seconds."`
}

// Binding matches one entry in the `bindings` array from
// `show mpls ldp binding json`. LocalLabel/RemoteLabel are strings
// because FRR uses sentinels like "imp-null" alongside numeric labels.
type Binding struct {
	AddressFamily string `json:"addressFamily" metric:"label"`
	Prefix        string `json:"prefix" metric:"label"`
	NeighborId    string `json:"neighborId" metric:"label"`
	LocalLabel    string `json:"localLabel,omitempty"`  // show-output only
	RemoteLabel   string `json:"remoteLabel,omitempty"` // show-output only
	InUse         uint64 `json:"inUse" metric:"name=protocols.ldp.binding.in_use,type=gauge,help=1 if this LDP label binding is installed in the dataplane, 0 otherwise."`
}

// Discovery matches one entry in the `adjacencies` array from
// `show mpls ldp discovery json`.
type Discovery struct {
	AddressFamily string `json:"addressFamily" metric:"label"`
	NeighborId    string `json:"neighborId" metric:"label"`
	Type          string `json:"type" metric:"label"`
	Interface     string `json:"interface" metric:"label"`
	HelloHoldtime uint32 `json:"helloHoldtime" metric:"name=protocols.ldp.discovery.hello_holdtime_seconds,type=gauge,help=LDP adjacency hello holdtime in seconds."`
}
