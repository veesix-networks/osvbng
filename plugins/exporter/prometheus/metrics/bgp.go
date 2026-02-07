package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/veesix-networks/osvbng/pkg/models/protocols/bgp"
	"github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	RegisterMetricSingleWithLabels[bgp.Statistics](paths.ProtocolsBGPStatistics, prometheus.Labels{"address_family": "ipv4"})
	RegisterMetricSingleWithLabels[bgp.Statistics](paths.ProtocolsBGPIPv6Statistics, prometheus.Labels{"address_family": "ipv6"})
}
