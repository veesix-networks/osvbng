// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

// Package bgp holds typed Go structures matching FRRouting's vtysh JSON
// output for BGP show commands. Used by internal/routing to parse FRR
// responses into typed values, and tagged with `metric:"..."` so
// pkg/handlers/show/protocols/bgp handlers register telemetry via
// pkg/telemetry.RegisterMetric[T].
//
// Modelling principle (osvbng-context #59 D2): only fields used for
// metrics or rendered by CLI/API consumers. Unmodeled FRR JSON keys are
// silently dropped on json.Unmarshal. See osvbng-context shapes/bgp.md
// for the captured FRR command shapes and field inventory rationale.
//
// FRR version: shapes authored against FRRouting 10.5.3.
package bgp

// Statistics matches `show ip bgp statistics json` / `show bgp statistics
// json` after the routing layer unwraps the AFI key (ipv4Unicast or
// ipv6Unicast) and the single-element array.
type Statistics struct {
	// AddressFamily is set by GetBGPStatistics(ipv4 bool) to "ipv4" or
	// "ipv6" so IPv4 and IPv6 statistics emit as distinct metric series.
	AddressFamily string `json:"-" metric:"label"`

	Instance string `json:"instance" metric:"label"`

	TotalAdvertisements          uint64  `json:"totalAdvertisements" metric:"name=protocols.bgp.statistics.advertisements,type=gauge,help=Total BGP advertisements."`
	TotalPrefixes                uint64  `json:"totalPrefixes" metric:"name=protocols.bgp.statistics.prefixes,type=gauge,help=Total BGP prefixes."`
	AveragePrefixLength          float64 `json:"averagePrefixLength" metric:"name=protocols.bgp.statistics.average_prefix_length,type=gauge,help=Average BGP prefix length."`
	UnaggregateablePrefixes      uint64  `json:"unaggregateablePrefixes" metric:"name=protocols.bgp.statistics.unaggregateable_prefixes,type=gauge,help=Unaggregateable prefixes."`
	MaximumAggregateablePrefixes uint64  `json:"maximumAggregateablePrefixes" metric:"name=protocols.bgp.statistics.maximum_aggregateable_prefixes,type=gauge,help=Maximum aggregateable prefixes."`
	BGPAggregateAdvertisements   uint64  `json:"bgpAggregateAdvertisements" metric:"name=protocols.bgp.statistics.aggregate_advertisements,type=gauge,help=BGP aggregate advertisements."`
	AddressSpaceAdvertised       float64 `json:"addressSpaceAdvertised" metric:"name=protocols.bgp.statistics.address_space_advertised,type=gauge,help=Total address space advertised."`
	AdvertisementsWithPaths      uint64  `json:"advertisementsWithPaths" metric:"name=protocols.bgp.statistics.advertisements_with_paths,type=gauge,help=Advertisements with paths."`
	LongestAsPath                uint64  `json:"longestAsPath" metric:"name=protocols.bgp.statistics.longest_as_path,type=gauge,help=Longest AS path length."`
	AverageAsPathLengthHops      float64 `json:"averageAsPathLengthHops" metric:"name=protocols.bgp.statistics.average_as_path_hops,type=gauge,help=Average AS path length in hops."`
	LargestAsPath                uint64  `json:"largestAsPath" metric:"name=protocols.bgp.statistics.largest_as_path,type=gauge,help=Largest AS path length."`
}
