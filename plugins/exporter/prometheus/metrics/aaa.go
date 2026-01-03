package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/veesix-networks/osvbng/internal/aaa"
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	Register("aaa.radius", func(logger *slog.Logger) (MetricHandler, error) {
		return NewAAAMetricHandler(logger)
	})
}

type AAAMetricHandler struct {
	logger      *slog.Logger
	metrics     []FieldMetric
	labelFields []string
	descs       map[string]*prometheus.Desc
}

func NewAAAMetricHandler(logger *slog.Logger) (*AAAMetricHandler, error) {
	statsType := reflect.TypeOf(aaa.ServerStats{})
	metrics, labelFields, err := GenerateMetrics(statsType)
	if err != nil {
		return nil, fmt.Errorf("failed to generate metrics: %w", err)
	}

	descs := make(map[string]*prometheus.Desc)
	for _, m := range metrics {
		descs[m.MetricName] = prometheus.NewDesc(
			m.MetricName,
			m.Help,
			labelFields,
			nil,
		)
	}

	return &AAAMetricHandler{
		logger:      logger,
		metrics:     metrics,
		labelFields: labelFields,
		descs:       descs,
	}, nil
}

func (h *AAAMetricHandler) Name() string {
	return "aaa.radius"
}

func (h *AAAMetricHandler) Paths() []string {
	return []string{paths.AAARadiusServers.String()}
}

func (h *AAAMetricHandler) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range h.descs {
		ch <- desc
	}
}

func (h *AAAMetricHandler) Collect(ctx context.Context, c cache.Cache, ch chan<- prometheus.Metric) error {
	data, err := c.GetAll(ctx, "osvbng:state:"+paths.AAARadiusServers.String())
	if err != nil {
		return err
	}

	for _, bytes := range data {
		var stats []*aaa.ServerStats
		if err := json.Unmarshal(bytes, &stats); err != nil {
			if h.logger != nil {
				h.logger.Warn("Failed to unmarshal metric", "error", err)
			}
			continue
		}

		for _, server := range stats {
			serverValue := reflect.ValueOf(*server)
			labelValues := GetLabelValues(serverValue, h.labelFields)

			for _, metric := range h.metrics {
				value, err := GetFieldValue(serverValue, metric.FieldName)
				if err != nil {
					if h.logger != nil {
						h.logger.Warn("Failed to get field value", "field", metric.FieldName, "error", err)
					}
					continue
				}

				desc := h.descs[metric.MetricName]
				ch <- prometheus.MustNewConstMetric(desc, metric.Type, value, labelValues...)
			}
		}
	}

	return nil
}
