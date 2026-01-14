package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/models/protocols/bgp"
	"github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	Register("protocols.bgp.ipv4", func(logger *slog.Logger) (MetricHandler, error) {
		return NewBGPMetricHandler(logger, "ipv4")
	})
	Register("protocols.bgp.ipv6", func(logger *slog.Logger) (MetricHandler, error) {
		return NewBGPMetricHandler(logger, "ipv6")
	})
}

type BGPMetricHandler struct {
	logger        *slog.Logger
	addressFamily string // "ipv4" or "ipv6"
	metrics       []FieldMetric
	labelFields   []string
	descs         map[string]*prometheus.Desc
}

func NewBGPMetricHandler(logger *slog.Logger, addressFamily string) (*BGPMetricHandler, error) {
	statsType := reflect.TypeOf(bgp.Statistics{})
	metrics, labelFields, err := GenerateMetrics(statsType)
	if err != nil {
		return nil, fmt.Errorf("failed to generate metrics: %w", err)
	}

	descs := make(map[string]*prometheus.Desc)
	constLabels := prometheus.Labels{"address_family": addressFamily}
	for _, m := range metrics {
		descs[m.MetricName] = prometheus.NewDesc(
			m.MetricName,
			m.Help,
			labelFields,
			constLabels,
		)
	}

	return &BGPMetricHandler{
		logger:        logger,
		addressFamily: addressFamily,
		metrics:       metrics,
		labelFields:   labelFields,
		descs:         descs,
	}, nil
}

func (h *BGPMetricHandler) Name() string {
	return fmt.Sprintf("protocols.bgp.%s", h.addressFamily)
}

func (h *BGPMetricHandler) Paths() []string {
	if h.addressFamily == "ipv4" {
		return []string{paths.ProtocolsBGPStatistics.String()}
	}
	return []string{paths.ProtocolsBGPIPv6Statistics.String()}
}

func (h *BGPMetricHandler) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range h.descs {
		ch <- desc
	}
}

func (h *BGPMetricHandler) Collect(ctx context.Context, store cache.Cache, ch chan<- prometheus.Metric) error {
	var path string
	if h.addressFamily == "ipv4" {
		path = "osvbng:state:" + paths.ProtocolsBGPStatistics.String()
	} else {
		path = "osvbng:state:" + paths.ProtocolsBGPIPv6Statistics.String()
	}

	data, err := store.GetAll(ctx, path)
	if err != nil {
		return err
	}

	for _, bytes := range data {
		var stats bgp.Statistics
		if err := json.Unmarshal(bytes, &stats); err != nil {
			if h.logger != nil {
				h.logger.Warn("Failed to unmarshal BGP metric", "error", err)
			}
			continue
		}

		statsValue := reflect.ValueOf(stats)
		labelValues := GetLabelValues(statsValue, h.labelFields)

		for _, metric := range h.metrics {
			value, err := GetFieldValue(statsValue, metric.FieldName)
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

	return nil
}
