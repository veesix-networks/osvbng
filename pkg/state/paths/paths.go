package paths

type Path string

const (
	AAARadiusServers           Path = "aaa.radius.servers"
	ProtocolsBGPStatistics     Path = "protocols.bgp.statistics"
	ProtocolsBGPIPv6Statistics Path = "protocols.bgp.ipv6.statistics"
	ProtocolsBGPNeighbors      Path = "protocols.bgp.neighbors.<*:ip>"
	SubscriberSessions         Path = "subscriber.sessions"

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
