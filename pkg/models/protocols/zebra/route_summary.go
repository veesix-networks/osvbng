// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package zebra

type RouteSummary struct {
	VRF            string             `json:"vrf,omitempty"     metric:"label=vrf,map_key"`
	Routes         []RouteSummaryItem `json:"routes,omitempty"  metric:"flatten"`
	RoutesTotal    uint32             `json:"routesTotal"       metric:"name=protocols.zebra.route.summary.routes_total,type=gauge,help=Total RIB entries for this VRF."`
	RoutesTotalFib uint32             `json:"routesTotalFib"    metric:"name=protocols.zebra.route.summary.routes_total_fib,type=gauge,help=Total FIB-installed entries for this VRF."`
}

type RouteSummaryItem struct {
	Type         string `json:"type"           metric:"label=protocol"`
	FIB          uint32 `json:"fib"            metric:"name=protocols.zebra.route.summary.routes_by_protocol_fib,type=gauge,help=Routes of a given protocol installed in the FIB."`
	RIB          uint32 `json:"rib"            metric:"name=protocols.zebra.route.summary.routes_by_protocol_rib,type=gauge,help=Routes of a given protocol present in the RIB."`
	FIBOffloaded uint32 `json:"fibOffLoaded"`
	FIBTrapped   uint32 `json:"fibTrapped"`
}

type RouteSummaryAll struct {
	VRFs map[string]RouteSummary `json:"-" metric:"flatten"`
}
