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

	VRFS Path = "vrfs"
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
