package paths

type Path string

const (
	AAARadiusServers           Path = "aaa.radius.servers"
	ProtocolsBGPStatistics     Path = "protocols.bgp.statistics"
	ProtocolsBGPIPv6Statistics Path = "protocols.bgp.ipv6.statistics"
	ProtocolsBGPNeighbors      Path = "protocols.bgp.neighbors.<*:ip>"
	ProtocolsOSPFNeighbors  Path = "protocols.ospf.neighbors"
	ProtocolsOSPF6Neighbors Path = "protocols.ospf6.neighbors"
	ProtocolsISISNeighbors  Path = "protocols.isis.neighbors"
	ProtocolsMPLSTable      Path = "protocols.mpls.table"
	ProtocolsMPLSInterfaces Path = "protocols.mpls.interfaces"
	ProtocolsLDPNeighbors   Path = "protocols.ldp.neighbors"
	ProtocolsLDPBindings    Path = "protocols.ldp.bindings"
	ProtocolsLDPDiscovery   Path = "protocols.ldp.discovery"
	SubscriberSessions         Path = "subscriber.sessions"
	SubscriberStats            Path = "subscriber.stats"

	SystemDataplaneSystem     Path = "system.dataplane.system"
	SystemDataplaneMemory     Path = "system.dataplane.memory"
	SystemDataplaneInterfaces Path = "system.dataplane.interfaces"
	SystemDataplaneNodes      Path = "system.dataplane.nodes"
	SystemDataplaneErrors     Path = "system.dataplane.errors"
	SystemDataplaneBuffers    Path = "system.dataplane.buffers"
)

func (p Path) String() string {
	return string(p)
}
