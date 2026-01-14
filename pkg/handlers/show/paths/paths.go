package paths

import (
	"fmt"
	"strings"
)

type Path string

const (
	AAARadiusServers           Path = "aaa.radius.servers"
	IPTable                    Path = "ip.table"
	PluginsInfo                Path = "plugins.info"
	ProtocolsBGPStatistics     Path = "protocols.bgp.statistics"
	ProtocolsBGPIPv6Statistics Path = "protocols.bgp.ipv6.statistics"
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

	VRFS Path = "vrfs"
)

func (p Path) String() string {
	return string(p)
}

func (p Path) ExtractWildcards(path string, expectedCount int) ([]string, error) {
	patternParts := strings.Split(string(p), ".")
	pathParts := strings.Split(path, ".")

	if len(patternParts) != len(pathParts) {
		return nil, fmt.Errorf("path format mismatch")
	}

	wildcards := make([]string, 0, expectedCount)
	for i := range patternParts {
		if patternParts[i] == "*" {
			wildcards = append(wildcards, pathParts[i])
		}
	}

	if len(wildcards) != expectedCount {
		return nil, fmt.Errorf("expected %d wildcards, got %d", expectedCount, len(wildcards))
	}

	return wildcards, nil
}
