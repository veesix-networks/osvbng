// Copyright 2025 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package paths

import (
	"github.com/veesix-networks/osvbng/pkg/paths"
)

type Path string

const (
	AAARadiusServers           Path = "aaa.radius.servers"
	IPTable                    Path = "ip.table"
	PluginsInfo                Path = "plugins.info"
	ProtocolsBGPStatistics            Path = "protocols.bgp.statistics"
	ProtocolsBGPIPv6Statistics        Path = "protocols.bgp.ipv6.statistics"
	ProtocolsBGPStatisticsAll         Path = "protocols.bgp.statistics.all"
	ProtocolsBGPIPv6StatisticsAll     Path = "protocols.bgp.ipv6.statistics.all"
	ProtocolsBGPNeighbors             Path = "protocols.bgp.neighbors.<*:ip>"
	ProtocolsBGPNeighborsAggregate    Path = "protocols.bgp.neighbors"
	ProtocolsBGPNeighborsAggregateAll Path = "protocols.bgp.neighbors.all"
	ProtocolsBGPSummary               Path = "protocols.bgp.summary"
	ProtocolsBGPSummaryAll            Path = "protocols.bgp.summary.all"
	ProtocolsBGPIPv4UnicastSummary    Path = "protocols.bgp.ipv4.unicast.summary"
	ProtocolsBGPIPv4UnicastSummaryAll Path = "protocols.bgp.ipv4.unicast.summary.all"
	ProtocolsBGPIPv6UnicastSummary    Path = "protocols.bgp.ipv6.unicast.summary"
	ProtocolsBGPIPv6UnicastSummaryAll Path = "protocols.bgp.ipv6.unicast.summary.all"
	ProtocolsBGPVPNIPv4        Path = "protocols.bgp.vpn.ipv4"
	ProtocolsBGPVPNIPv6        Path = "protocols.bgp.vpn.ipv6"
	ProtocolsBGPVPNIPv4Summary Path = "protocols.bgp.vpn.ipv4.summary"
	ProtocolsBGPVPNIPv6Summary Path = "protocols.bgp.vpn.ipv6.summary"
	SubscriberSessions         Path = "subscriber.sessions"
	SubscriberSession          Path = "subscriber.session"
	SubscriberStats            Path = "subscriber.stats"
	SystemThreads              Path = "system.threads"
	SystemCacheStatistics      Path = "system.cache.statistics"
	SystemCacheKeys            Path = "system.cache.keys"
	SystemCacheKey             Path = "system.cache.key"
	SystemConfHandlers         Path = "system.conf_handlers"
	SystemShowHandlers         Path = "system.show_handlers"
	SystemOperHandlers         Path = "system.oper_handlers"
	SystemVersion              Path = "system.version"
	SystemOpDBSessions         Path = "system.opdb.sessions"
	SystemOpDBStatistics       Path = "system.opdb.statistics"
	SystemLogging              Path = "system.logging"

	ProtocolsOSPF                   Path = "protocols.ospf"
	ProtocolsOSPFAll                Path = "protocols.ospf.all"
	ProtocolsOSPFInterfaces         Path = "protocols.ospf.interfaces"
	ProtocolsOSPFInterfacesAll      Path = "protocols.ospf.interfaces.all"
	ProtocolsOSPFNeighbors          Path = "protocols.ospf.neighbors"
	ProtocolsOSPFNeighbor           Path = "protocols.ospf.neighbors.<*:ipv4>"
	ProtocolsOSPFNeighborsDetail    Path = "protocols.ospf.neighbors.detail"
	ProtocolsOSPFNeighborsDetailAll Path = "protocols.ospf.neighbors.detail.all"
	ProtocolsOSPFGRHelper           Path = "protocols.ospf.graceful-restart-helper"
	ProtocolsOSPFGRHelperAll        Path = "protocols.ospf.graceful-restart-helper.all"
	ProtocolsOSPFRoute              Path = "protocols.ospf.route"
	ProtocolsOSPFBorderRouters      Path = "protocols.ospf.border-routers"
	ProtocolsOSPFReachable          Path = "protocols.ospf.reachable-routers"
	ProtocolsOSPFSummary            Path = "protocols.ospf.summary-address"
	ProtocolsOSPFDatabase           Path = "protocols.ospf.database"
	ProtocolsOSPFDatabaseLSA        Path = "protocols.ospf.database.<*>"
	ProtocolsOSPFMPLSTEInterface    Path = "protocols.ospf.mpls-te.interface"
	ProtocolsOSPFMPLSTERouter       Path = "protocols.ospf.mpls-te.router"
	ProtocolsOSPFMPLSTEDatabase     Path = "protocols.ospf.mpls-te.database"
	ProtocolsOSPFRouterInfo         Path = "protocols.ospf.router-info"
	ProtocolsOSPFSegmentRouting     Path = "protocols.ospf.segment-routing"
	ProtocolsOSPF6                  Path = "protocols.ospf6"
	ProtocolsOSPF6Interfaces        Path = "protocols.ospf6.interfaces"
	ProtocolsOSPF6InterfaceTraffic  Path = "protocols.ospf6.interfaces.traffic"
	ProtocolsOSPF6InterfacePrefix   Path = "protocols.ospf6.interfaces.prefix"
	ProtocolsOSPF6GRHelper          Path = "protocols.ospf6.graceful-restart-helper"
	ProtocolsOSPF6Neighbors         Path = "protocols.ospf6.neighbors"
	ProtocolsOSPF6Neighbor          Path = "protocols.ospf6.neighbors.<*:ipv4>"
	ProtocolsOSPF6NeighborsDetail   Path = "protocols.ospf6.neighbors.detail"
	ProtocolsOSPF6NeighborsDRChoice Path = "protocols.ospf6.neighbors.drchoice"
	ProtocolsOSPF6Route             Path = "protocols.ospf6.route"
	ProtocolsOSPF6SpfTree           Path = "protocols.ospf6.spf-tree"
	ProtocolsOSPF6Redistribute      Path = "protocols.ospf6.redistribute"
	ProtocolsOSPF6Zebra             Path = "protocols.ospf6.zebra"
	ProtocolsOSPF6Summary           Path = "protocols.ospf6.summary-address"
	ProtocolsOSPF6Database          Path = "protocols.ospf6.database"
	ProtocolsOSPF6DatabaseLSA       Path = "protocols.ospf6.database.<*>"
	ProtocolsISISNeighbors          Path = "protocols.isis.neighbors"

	ProtocolsMPLSTable      Path = "protocols.mpls.table"
	ProtocolsMPLSInterfaces Path = "protocols.mpls.interfaces"
	ProtocolsLDPNeighbors   Path = "protocols.ldp.neighbors"
	ProtocolsLDPBindings    Path = "protocols.ldp.bindings"
	ProtocolsLDPDiscovery   Path = "protocols.ldp.discovery"

	ServiceGroups Path = "service-groups"
	VRFS          Path = "vrfs"

	SystemCPPMDataplane    Path = "system.cppm.dataplane"
	SystemCPPMControlplane Path = "system.cppm.controlplane"

	SystemDataplaneStats     Path = "system.dataplane.stats"
	SystemDataplaneSystem    Path = "system.dataplane.system"
	SystemDataplaneMemory    Path = "system.dataplane.memory"
	SystemDataplaneInterfaces Path = "system.dataplane.interfaces"
	SystemDataplaneNodes     Path = "system.dataplane.nodes"
	SystemDataplaneErrors    Path = "system.dataplane.errors"
	SystemDataplaneBuffers   Path = "system.dataplane.buffers"
	Interfaces       Path = "interfaces"
	InterfacesDetail Path = "interfaces.<*>"

	SystemEvents   Path = "system.events"
	SystemWatchdog Path = "system.watchdog"

	HAStatus      Path = "ha.status"
	HASRGs        Path = "ha.srg"
	HASRGState    Path = "ha.srg.state"
	HAPeer        Path = "ha.peer"
	HASRGCounters Path = "ha.srg.counters"
	HASync        Path = "ha.sync"

	DHCPRelay Path = "dhcp.relay"
	DHCPProxy Path = "dhcp.proxy"

	L2TPTunnels Path = "l2tp.tunnels"
	L2TPTunnel  Path = "l2tp.tunnels.<*>"

	CGNATSessions   Path = "cgnat.sessions"
	CGNATPools      Path = "cgnat.pools"
	CGNATStatistics Path = "cgnat.statistics"
	CGNATLookup     Path = "cgnat.lookup"

	QoSScheduler Path = "qos.scheduler"

	RoutingPolicyPrefixSets         Path = "routing-policies.prefix-sets"
	RoutingPolicyPrefixSet          Path = "routing-policies.prefix-sets.<*>"
	RoutingPolicyPrefixSetsV6       Path = "routing-policies.prefix-sets-v6"
	RoutingPolicyPrefixSetV6        Path = "routing-policies.prefix-sets-v6.<*>"
	RoutingPolicyCommunitySets      Path = "routing-policies.community-sets"
	RoutingPolicyCommunitySet       Path = "routing-policies.community-sets.<*>"
	RoutingPolicyExtCommunitySets   Path = "routing-policies.ext-community-sets"
	RoutingPolicyExtCommunitySet    Path = "routing-policies.ext-community-sets.<*>"
	RoutingPolicyLargeCommunitySets Path = "routing-policies.large-community-sets"
	RoutingPolicyLargeCommunitySet  Path = "routing-policies.large-community-sets.<*>"
	RoutingPolicyASPathSets         Path = "routing-policies.as-path-sets"
	RoutingPolicyASPathSet          Path = "routing-policies.as-path-sets.<*>"
	RoutingPolicyRoutePolicies      Path = "routing-policies.route-policies"
	RoutingPolicyRoutePolicy        Path = "routing-policies.route-policies.<*>"
)

func (p Path) String() string {
	return string(p)
}

func (p Path) ExtractWildcards(path string, expectedCount int) ([]string, error) {
	return paths.Extract(path, string(p))
}

func Build(pattern Path, values ...string) (string, error) {
	return paths.Build(string(pattern), values...)
}

func Extract(path string, pattern Path) ([]string, error) {
	return paths.Extract(path, string(pattern))
}
