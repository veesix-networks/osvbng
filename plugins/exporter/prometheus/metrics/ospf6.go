package metrics

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/models/protocols/ospf6"
	"github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	Register("protocols.ospf6", func(logger *slog.Logger) (MetricHandler, error) {
		return NewOSPF6MetricHandler(logger), nil
	})
}

type OSPF6MetricHandler struct {
	logger        *slog.Logger
	neighborCount *prometheus.Desc
}

func NewOSPF6MetricHandler(logger *slog.Logger) *OSPF6MetricHandler {
	return &OSPF6MetricHandler{
		logger: logger,
		neighborCount: prometheus.NewDesc(
			"osvbng_ospf6_neighbor_count",
			"Number of OSPFv3 neighbors",
			[]string{"state"},
			nil,
		),
	}
}

func (h *OSPF6MetricHandler) Name() string    { return "protocols.ospf6" }
func (h *OSPF6MetricHandler) Paths() []string { return []string{paths.ProtocolsOSPF6Neighbors.String()} }

func (h *OSPF6MetricHandler) Describe(ch chan<- *prometheus.Desc) {
	ch <- h.neighborCount
}

func (h *OSPF6MetricHandler) Collect(ctx context.Context, c cache.Cache, ch chan<- prometheus.Metric) error {
	data, err := c.Get(ctx, "osvbng:state:"+paths.ProtocolsOSPF6Neighbors.String())
	if err != nil {
		return err
	}

	var neighbors []ospf6.Neighbor
	if err := json.Unmarshal(data, &neighbors); err != nil {
		return err
	}

	counts := make(map[string]int)
	for _, nbr := range neighbors {
		counts[nbr.State]++
	}

	for state, count := range counts {
		ch <- prometheus.MustNewConstMetric(h.neighborCount, prometheus.GaugeValue, float64(count), state)
	}

	return nil
}
