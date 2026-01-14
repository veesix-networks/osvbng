package metrics

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type FieldMetric struct {
	FieldName  string
	MetricName string
	Help       string
	Type       prometheus.ValueType
	IsLabel    bool
}

func ParsePrometheusTag(tag string) (name, help string, metricType prometheus.ValueType, isLabel bool, err error) {
	if tag == "label" {
		return "", "", 0, true, nil
	}

	parts := strings.Split(tag, ",")
	nameFound := false
	helpFound := false
	typeFound := false

	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])

		switch key {
		case "name":
			name = value
			nameFound = true
		case "help":
			help = value
			helpFound = true
		case "type":
			typeFound = true
			switch value {
			case "counter":
				metricType = prometheus.CounterValue
			case "gauge":
				metricType = prometheus.GaugeValue
			case "histogram":
				metricType = prometheus.UntypedValue
			default:
				return "", "", 0, false, fmt.Errorf("unknown metric type: %s", value)
			}
		}
	}

	if !nameFound || !helpFound || !typeFound {
		return "", "", 0, false, fmt.Errorf("missing required prometheus tag fields (name, help, type)")
	}

	return name, help, metricType, false, nil
}

func GenerateMetrics(structType reflect.Type) ([]FieldMetric, []string, error) {
	var metrics []FieldMetric
	var labels []string

	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		tag, ok := field.Tag.Lookup("prometheus")
		if !ok {
			continue
		}

		name, help, metricType, isLabel, err := ParsePrometheusTag(tag)
		if err != nil {
			return nil, nil, fmt.Errorf("field %s: %w", field.Name, err)
		}

		if isLabel {
			labels = append(labels, field.Name)
			continue
		}

		metrics = append(metrics, FieldMetric{
			FieldName:  field.Name,
			MetricName: name,
			Help:       help,
			Type:       metricType,
			IsLabel:    false,
		})
	}

	return metrics, labels, nil
}

func GetFieldValue(v reflect.Value, fieldName string) (float64, error) {
	field := v.FieldByName(fieldName)
	if !field.IsValid() {
		return 0, fmt.Errorf("field %s not found", fieldName)
	}

	switch field.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(field.Uint()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(field.Int()), nil
	case reflect.Float32, reflect.Float64:
		return field.Float(), nil
	case reflect.Struct:
		if field.Type() == reflect.TypeOf(time.Time{}) {
			t := field.Interface().(time.Time)
			return float64(t.Unix()), nil
		}
		return 0, fmt.Errorf("unsupported struct type for field %s", fieldName)
	default:
		return 0, fmt.Errorf("unsupported type for field %s: %s", fieldName, field.Kind())
	}
}

func GetLabelValues(v reflect.Value, labelFields []string) []string {
	values := make([]string, len(labelFields))
	for i, fieldName := range labelFields {
		field := v.FieldByName(fieldName)
		if !field.IsValid() {
			continue
		}
		values[i] = fmt.Sprintf("%v", field.Interface())
	}
	return values
}
