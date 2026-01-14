package metrics

import (
	"context"
	"encoding/json"
	"log/slog"
	"reflect"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/models/subscribers"
	"github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	Register("subscriber.sessions", func(logger *slog.Logger) (MetricHandler, error) {
		return NewSubscriberMetricHandler(logger)
	})
}

type SubscriberMetricHandler struct {
	logger      *slog.Logger
	metrics     []FieldMetric
	labelFields []string
	descs       map[string]*prometheus.Desc
}

func NewSubscriberMetricHandler(logger *slog.Logger) (*SubscriberMetricHandler, error) {
	statsType := reflect.TypeOf(subscribers.Statistics{})
	metrics, labelFields, err := GenerateMetrics(statsType)
	if err != nil {
		return nil, err
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

	return &SubscriberMetricHandler{
		logger:      logger,
		metrics:     metrics,
		labelFields: labelFields,
		descs:       descs,
	}, nil
}

func (h *SubscriberMetricHandler) Name() string {
	return "subscriber.sessions"
}

func (h *SubscriberMetricHandler) Paths() []string {
	return []string{paths.SubscriberSessions.String()}
}

func (h *SubscriberMetricHandler) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range h.descs {
		ch <- desc
	}
}

func (h *SubscriberMetricHandler) Collect(ctx context.Context, store cache.Cache, ch chan<- prometheus.Metric) error {
	path := "osvbng:state:" + paths.SubscriberSessions.String()

	h.logger.Debug("Collecting subscriber metrics", "path", path)
	data, err := store.GetAll(ctx, path)
	if err != nil {
		h.logger.Error("Failed to get data from cache", "path", path, "error", err)
		return err
	}
	h.logger.Debug("Got data from cache", "path", path, "count", len(data))

	for _, bytes := range data {
		h.logger.Debug("Unmarshaling data", "bytes", string(bytes))
		var stats subscribers.Statistics
		if err := json.Unmarshal(bytes, &stats); err != nil {
			if h.logger != nil {
				h.logger.Warn("Failed to unmarshal subscriber metric", "error", err)
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
