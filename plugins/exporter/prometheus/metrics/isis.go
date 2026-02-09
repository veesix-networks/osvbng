package metrics

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/models/protocols/isis"
	"github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	Register("protocols.isis", func(logger *slog.Logger) (MetricHandler, error) {
		return NewISISMetricHandler(logger), nil
	})
}

type ISISMetricHandler struct {
	logger         *slog.Logger
	adjacencyCount *prometheus.Desc
}

func NewISISMetricHandler(logger *slog.Logger) *ISISMetricHandler {
	return &ISISMetricHandler{
		logger: logger,
		adjacencyCount: prometheus.NewDesc(
			"osvbng_isis_adjacency_count",
			"Number of ISIS adjacencies",
			[]string{"area", "state"},
			nil,
		),
	}
}

func (h *ISISMetricHandler) Name() string    { return "protocols.isis" }
func (h *ISISMetricHandler) Paths() []string { return []string{paths.ProtocolsISISNeighbors.String()} }

func (h *ISISMetricHandler) Describe(ch chan<- *prometheus.Desc) {
	ch <- h.adjacencyCount
}

func (h *ISISMetricHandler) Collect(ctx context.Context, c cache.Cache, ch chan<- prometheus.Metric) error {
	data, err := c.Get(ctx, "osvbng:state:"+paths.ProtocolsISISNeighbors.String())
	if err != nil {
		return err
	}

	var areas []isis.Area
	if err := json.Unmarshal(data, &areas); err != nil {
		return err
	}

	type key struct {
		area  string
		state string
	}
	counts := make(map[key]int)

	for _, area := range areas {
		for _, circuit := range area.Circuits {
			if circuit.Adj == "" {
				continue
			}
			counts[key{area: area.Area, state: circuit.State}]++
		}
	}

	for k, count := range counts {
		ch <- prometheus.MustNewConstMetric(h.adjacencyCount, prometheus.GaugeValue, float64(count), k.area, k.state)
	}

	return nil
}
