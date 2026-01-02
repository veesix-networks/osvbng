package paths

type Path string

const (
	AAARadiusServers           Path = "aaa.radius.servers"
	IPTable                    Path = "ip.table"
	ProtocolsBGPStatistics     Path = "protocols.bgp.statistics"
	ProtocolsBGPIPv6Statistics Path = "protocols.bgp.ipv6.statistics"
	SubscriberSessions         Path = "subscriber.sessions"
	SubscriberSession          Path = "subscriber.session"
	SubscriberStats            Path = "subscriber.stats"
	SystemThreads              Path = "system.threads"

	VRFS Path = "vrfs"
)

func (p Path) String() string {
	return string(p)
}
