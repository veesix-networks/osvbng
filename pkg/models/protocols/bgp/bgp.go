// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

// Package bgp holds typed Go structures matching FRRouting's vtysh JSON
// output for BGP show commands.
package bgp

type Statistics struct {
	// AddressFamily is populated by the routing layer ("ipv4"/"ipv6")
	// so IPv4 and IPv6 statistics emit as distinct metric series.
	AddressFamily string `json:"-" metric:"label=afi_safi"`

	// VRF is populated by the routing layer from the FRR "instance" string
	// ("VRF default" → "default"). Replaces the older `instance` label for
	// parity with the rest of routing.
	VRF string `json:"-" metric:"label=vrf"`

	Instance string `json:"instance"`

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
