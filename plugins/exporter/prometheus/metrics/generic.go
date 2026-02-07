package metrics

import (
	"context"
	"encoding/json"
	"log/slog"
	"reflect"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/veesix-networks/osvbng/pkg/cache"
)

type StructHandler[T any] struct {
	name        string
	path        string
	logger      *slog.Logger
	metrics     []fieldAccessor
	labelFields []labelAccessor
	descs       map[string]*prometheus.Desc
	isSlice     bool
}

type fieldAccessor struct {
	index      int
	metricName string
	metricType prometheus.ValueType
}

type labelAccessor struct {
	index int
}

func NewStructHandler[T any](name, path string, isSlice bool, constLabels prometheus.Labels, logger *slog.Logger) (*StructHandler[T], error) {
	var zero T
	structType := reflect.TypeOf(zero)

	var metrics []fieldAccessor
	var labelFields []labelAccessor
	var labelNames []string
	descs := make(map[string]*prometheus.Desc)

	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		tag, ok := field.Tag.Lookup("prometheus")
		if !ok {
			continue
		}

		metricName, help, metricType, isLabel, err := ParsePrometheusTag(tag)
		if err != nil {
			continue
		}

		if isLabel {
			labelFields = append(labelFields, labelAccessor{index: i})
			labelNames = append(labelNames, field.Name)
		} else {
			metrics = append(metrics, fieldAccessor{
				index:      i,
				metricName: metricName,
				metricType: metricType,
			})
			descs[metricName] = prometheus.NewDesc(metricName, help, labelNames, constLabels)
		}
	}

	return &StructHandler[T]{
		name:        name,
		path:        path,
		logger:      logger,
		metrics:     metrics,
		labelFields: labelFields,
		descs:       descs,
		isSlice:     isSlice,
	}, nil
}

func (h *StructHandler[T]) Name() string        { return h.name }
func (h *StructHandler[T]) Paths() []string     { return []string{h.path} }

func (h *StructHandler[T]) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range h.descs {
		ch <- desc
	}
}

func (h *StructHandler[T]) Collect(ctx context.Context, store cache.Cache, ch chan<- prometheus.Metric) error {
	fullPath := "osvbng:state:" + h.path

	data, err := store.Get(ctx, fullPath)
	if err != nil {
		return err
	}

	if h.isSlice {
		var items []T
		if err := json.Unmarshal(data, &items); err != nil {
			return err
		}
		for i := range items {
			h.emit(reflect.ValueOf(items[i]), ch)
		}
	} else {
		var item T
		if err := json.Unmarshal(data, &item); err != nil {
			return err
		}
		h.emit(reflect.ValueOf(item), ch)
	}

	return nil
}

func (h *StructHandler[T]) emit(v reflect.Value, ch chan<- prometheus.Metric) {
	labelValues := make([]string, len(h.labelFields))
	for i, la := range h.labelFields {
		field := v.Field(la.index)
		switch field.Kind() {
		case reflect.String:
			labelValues[i] = field.String()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			labelValues[i] = strconv.FormatUint(field.Uint(), 10)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			labelValues[i] = strconv.FormatInt(field.Int(), 10)
		}
	}

	for _, m := range h.metrics {
		field := v.Field(m.index)
		var value float64

		switch field.Kind() {
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			value = float64(field.Uint())
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			value = float64(field.Int())
		case reflect.Float32, reflect.Float64:
			value = field.Float()
		default:
			continue
		}

		if desc := h.descs[m.metricName]; desc != nil {
			ch <- prometheus.MustNewConstMetric(desc, m.metricType, value, labelValues...)
		}
	}
}

type pathLike interface {
	String() string
}

func RegisterMetricSingle[T any](path pathLike) {
	pathStr := path.String()
	Register(pathStr, func(logger *slog.Logger) (MetricHandler, error) {
		return NewStructHandler[T](pathStr, pathStr, false, nil, logger)
	})
}

func RegisterMetricMulti[T any](path pathLike) {
	pathStr := path.String()
	Register(pathStr, func(logger *slog.Logger) (MetricHandler, error) {
		return NewStructHandler[T](pathStr, pathStr, true, nil, logger)
	})
}

func RegisterMetricSingleWithLabels[T any](path pathLike, constLabels prometheus.Labels) {
	pathStr := path.String()
	Register(pathStr, func(logger *slog.Logger) (MetricHandler, error) {
		return NewStructHandler[T](pathStr, pathStr, false, constLabels, logger)
	})
}

func RegisterMetricMultiWithLabels[T any](path pathLike, constLabels prometheus.Labels) {
	pathStr := path.String()
	Register(pathStr, func(logger *slog.Logger) (MetricHandler, error) {
		return NewStructHandler[T](pathStr, pathStr, true, constLabels, logger)
	})
}
