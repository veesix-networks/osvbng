// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ldp

// Interface mirrors one entry in `show mpls ldp [<ipv4|ipv6>] interface json`.
// FRR keys the outer map by composite "<iface>: <afi>" (e.g. "eth2: ipv4");
// the routing wrapper splits that into separate Interface + AddressFamily
// fields before returning, so labels emit as `interface="eth2",afi="ipv4"`.
type Interface struct {
	Interface      string `json:"-" metric:"label,map_key"`
	AddressFamily  string `json:"addressFamily" metric:"label=afi"`
	State          string `json:"state" metric:"label"`
	UpTime         string `json:"upTime,omitempty"`
	HelloInterval  uint32 `json:"helloInterval" metric:"name=protocols.ldp.interface.hello_interval_seconds,type=gauge,help=LDP Hello interval in seconds on this interface."`
	HelloHoldtime  uint32 `json:"helloHoldtime" metric:"name=protocols.ldp.interface.hello_holdtime_seconds,type=gauge,help=LDP Hello holdtime in seconds on this interface."`
	AdjacencyCount uint32 `json:"adjacencyCount" metric:"name=protocols.ldp.interface.adjacencies,type=gauge,help=LDP adjacency count on this interface."`
}
