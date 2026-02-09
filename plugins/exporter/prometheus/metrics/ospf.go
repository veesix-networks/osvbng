package metrics

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/models/protocols/ospf"
	"github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	Register("protocols.ospf", func(logger *slog.Logger) (MetricHandler, error) {
		return NewOSPFMetricHandler(logger), nil
	})
}

type OSPFMetricHandler struct {
	logger        *slog.Logger
	neighborCount *prometheus.Desc
}

func NewOSPFMetricHandler(logger *slog.Logger) *OSPFMetricHandler {
	return &OSPFMetricHandler{
		logger: logger,
		neighborCount: prometheus.NewDesc(
			"osvbng_ospf_neighbor_count",
			"Number of OSPF neighbors",
			[]string{"state"},
			nil,
		),
	}
}

func (h *OSPFMetricHandler) Name() string    { return "protocols.ospf" }
func (h *OSPFMetricHandler) Paths() []string { return []string{paths.ProtocolsOSPFNeighbors.String()} }

func (h *OSPFMetricHandler) Describe(ch chan<- *prometheus.Desc) {
	ch <- h.neighborCount
}

func (h *OSPFMetricHandler) Collect(ctx context.Context, c cache.Cache, ch chan<- prometheus.Metric) error {
	data, err := c.Get(ctx, "osvbng:state:"+paths.ProtocolsOSPFNeighbors.String())
	if err != nil {
		return err
	}

	var neighbors map[string][]ospf.Neighbor
	if err := json.Unmarshal(data, &neighbors); err != nil {
		return err
	}

	counts := make(map[string]int)
	for _, nbrList := range neighbors {
		for _, nbr := range nbrList {
			counts[nbr.Converged]++
		}
	}

	for state, count := range counts {
		ch <- prometheus.MustNewConstMetric(h.neighborCount, prometheus.GaugeValue, float64(count), state)
	}

	return nil
}
