// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

type VPNStatistics struct {
	AddressFamily string `json:"-" metric:"label=afi_safi"`
	VRF           string `json:"-" metric:"label=vrf,map_key"`

	Instance string `json:"instance,omitempty"`

	BGPBestPathCalls             uint64  `json:"bgpBestPathCalls" metric:"name=protocols.bgp.vpn.statistics.bestpath_calls,type=counter,help=BGP VPN bestpath calls."`
	BGPNodeOnQueue               uint64  `json:"bgpNodeOnQueue" metric:"name=protocols.bgp.vpn.statistics.node_on_queue,type=gauge,help=BGP VPN nodes on the work queue."`
	BGPNodeDeferredOnQueue       uint64  `json:"bgpNodeDeferredOnQueue" metric:"name=protocols.bgp.vpn.statistics.node_deferred_on_queue,type=gauge,help=BGP VPN nodes deferred on the work queue."`
	TotalAdvertisements          uint64  `json:"totalAdvertisements" metric:"name=protocols.bgp.vpn.statistics.advertisements,type=gauge,help=Total BGP VPN advertisements."`
	TotalPrefixes                uint64  `json:"totalPrefixes" metric:"name=protocols.bgp.vpn.statistics.prefixes,type=gauge,help=Total BGP VPN prefixes."`
	AveragePrefixLength          float64 `json:"averagePrefixLength" metric:"name=protocols.bgp.vpn.statistics.average_prefix_length,type=gauge,help=Average BGP VPN prefix length."`
	UnaggregateablePrefixes      uint64  `json:"unaggregateablePrefixes" metric:"name=protocols.bgp.vpn.statistics.unaggregateable_prefixes,type=gauge,help=Unaggregateable BGP VPN prefixes."`
	MaximumAggregateablePrefixes uint64  `json:"maximumAggregateablePrefixes" metric:"name=protocols.bgp.vpn.statistics.maximum_aggregateable_prefixes,type=gauge,help=Maximum aggregateable BGP VPN prefixes."`
	BGPAggregateAdvertisements   uint64  `json:"bgpAggregateAdvertisements" metric:"name=protocols.bgp.vpn.statistics.aggregate_advertisements,type=gauge,help=BGP VPN aggregate advertisements."`
	AddressSpaceAdvertised       float64 `json:"addressSpaceAdvertised" metric:"name=protocols.bgp.vpn.statistics.address_space_advertised,type=gauge,help=BGP VPN total address space advertised."`
	AdvertisementsWithPaths      uint64  `json:"advertisementsWithPaths" metric:"name=protocols.bgp.vpn.statistics.advertisements_with_paths,type=gauge,help=BGP VPN advertisements with paths."`
	LongestAsPath                uint64  `json:"longestAsPath" metric:"name=protocols.bgp.vpn.statistics.longest_as_path,type=gauge,help=Longest BGP VPN AS path length."`
	AverageAsPathLengthHops      float64 `json:"averageAsPathLengthHops" metric:"name=protocols.bgp.vpn.statistics.average_as_path_hops,type=gauge,help=Average BGP VPN AS path length in hops."`
	LargestAsPath                uint64  `json:"largestAsPath" metric:"name=protocols.bgp.vpn.statistics.largest_as_path,type=gauge,help=Largest BGP VPN AS path length."`
	HighestPublicAsn             uint64  `json:"highestPublicAsn" metric:"name=protocols.bgp.vpn.statistics.highest_public_asn,type=gauge,help=Highest public ASN seen in BGP VPN paths."`
	TotalRedistributed           uint64  `json:"totalRedistributed" metric:"name=protocols.bgp.vpn.statistics.redistributed,type=gauge,help=Total redistributed BGP VPN prefixes."`
	TotalLocalAggregates         uint64  `json:"totalLocalAggregates" metric:"name=protocols.bgp.vpn.statistics.local_aggregates,type=gauge,help=Total local BGP VPN aggregates."`
}
