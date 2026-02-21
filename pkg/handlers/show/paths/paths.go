package paths

import (
	"github.com/veesix-networks/osvbng/pkg/paths"
)

type Path string

const (
	AAARadiusServers           Path = "aaa.radius.servers"
	IPTable                    Path = "ip.table"
	PluginsInfo                Path = "plugins.info"
	ProtocolsBGPStatistics     Path = "protocols.bgp.statistics"
	ProtocolsBGPIPv6Statistics Path = "protocols.bgp.ipv6.statistics"
	ProtocolsBGPNeighbors      Path = "protocols.bgp.neighbors.<*:ip>"
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

	ProtocolsOSPFNeighbors  Path = "protocols.ospf.neighbors"
	ProtocolsOSPF6Neighbors Path = "protocols.ospf6.neighbors"
	ProtocolsISISNeighbors  Path = "protocols.isis.neighbors"

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

	SystemWatchdog Path = "system.watchdog"
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
