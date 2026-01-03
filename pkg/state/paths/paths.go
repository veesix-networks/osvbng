package paths

type Path string

const (
	AAARadiusServers           Path = "aaa.radius.servers"
	ProtocolsBGPStatistics     Path = "protocols.bgp.statistics"
	ProtocolsBGPIPv6Statistics Path = "protocols.bgp.ipv6.statistics"
	SubscriberSessions         Path = "subscriber.sessions"
)

func (p Path) String() string {
	return string(p)
}
