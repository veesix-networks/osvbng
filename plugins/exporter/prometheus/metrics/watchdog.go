package metrics

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	Register(paths.SystemWatchdog.String(), func(logger *slog.Logger) (MetricHandler, error) {
		return newWatchdogMetricHandler(logger), nil
	})
}

type watchdogTargetMetric struct {
	Name            string  `json:"name"`
	State           string  `json:"state"`
	Critical        bool    `json:"critical"`
	LastCheckOK     bool    `json:"last-check-ok"`
	LastCheckMs     float64 `json:"last-check-ms"`
	ConsecFailures  int64   `json:"consecutive-failures"`
	TotalFailures   int64   `json:"total-failures"`
	TotalRecoveries int64   `json:"total-recoveries"`
	TotalRestarts   int64   `json:"total-restarts"`
}

type watchdogMetricHandler struct {
	logger *slog.Logger
	descs  map[string]*prometheus.Desc
}

func newWatchdogMetricHandler(logger *slog.Logger) *watchdogMetricHandler {
	labels := []string{"target"}
	return &watchdogMetricHandler{
		logger: logger,
		descs: map[string]*prometheus.Desc{
			"up":       prometheus.NewDesc("osvbng_watchdog_target_up", "Whether the watchdog target is up (1) or not (0)", labels, nil),
			"duration": prometheus.NewDesc("osvbng_watchdog_health_check_duration_seconds", "Duration of the last health check in seconds", labels, nil),
			"failures": prometheus.NewDesc("osvbng_watchdog_failures_total", "Total number of health check failures", labels, nil),
			"recoveries": prometheus.NewDesc("osvbng_watchdog_recoveries_total", "Total number of successful recoveries", labels, nil),
			"restarts":   prometheus.NewDesc("osvbng_watchdog_restarts_total", "Total number of target restarts", labels, nil),
		},
	}
}

func (h *watchdogMetricHandler) Name() string {
	return paths.SystemWatchdog.String()
}

func (h *watchdogMetricHandler) Paths() []string {
	return []string{paths.SystemWatchdog.String()}
}

func (h *watchdogMetricHandler) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range h.descs {
		ch <- desc
	}
}

func (h *watchdogMetricHandler) Collect(ctx context.Context, store cache.Cache, ch chan<- prometheus.Metric) error {
	fullPath := "osvbng:state:" + paths.SystemWatchdog.String()

	data, err := store.Get(ctx, fullPath)
	if err != nil {
		return err
	}

	var targets []watchdogTargetMetric
	if err := json.Unmarshal(data, &targets); err != nil {
		return err
	}

	for _, t := range targets {
		var up float64
		if t.LastCheckOK {
			up = 1
		}

		ch <- prometheus.MustNewConstMetric(h.descs["up"], prometheus.GaugeValue, up, t.Name)
		ch <- prometheus.MustNewConstMetric(h.descs["duration"], prometheus.GaugeValue, t.LastCheckMs/1000.0, t.Name)
		ch <- prometheus.MustNewConstMetric(h.descs["failures"], prometheus.CounterValue, float64(t.TotalFailures), t.Name)
		ch <- prometheus.MustNewConstMetric(h.descs["recoveries"], prometheus.CounterValue, float64(t.TotalRecoveries), t.Name)
		ch <- prometheus.MustNewConstMetric(h.descs["restarts"], prometheus.CounterValue, float64(t.TotalRestarts), t.Name)
	}

	return nil
}
