// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"sync"
)

// RegisterOpts configures MustRegisterStruct. Path is prepended to every
// metric name parsed from struct tags with "." as the separator. With
// Path="aaa.radius" and a tag name="auth.requests", the registered path
// is "aaa.radius.auth.requests".
type RegisterOpts struct {
	Path string
}

// StructMetrics is the per-struct registration result. WithLabelValues
// returns cached typed handles for one tuple of label values, ready for
// hot-path emit.
type StructMetrics[T any] struct {
	registry   *Registry
	labelNames []string // wire label names, in declaration order

	fields []structField // metric fields in the order they appear on T
	byName map[string]int
}

type structField struct {
	goName    string
	fullName  string
	help      string
	typ       MetricType
	fieldIdx  int      // index into reflect.StructField list
	counter   *Counter // exactly one of counter/gauge/histogram is non-nil
	gauge     *Gauge
	histogram *Histogram
}

// MustRegisterStruct registers every field of T tagged `metric:"..."`
// against the default registry. Reflection runs only here.
func MustRegisterStruct[T any](opts RegisterOpts) *StructMetrics[T] {
	return MustRegisterStructWith[T](defaultRegistry, opts)
}

// MustRegisterStructWith registers against the supplied registry, for
// tests that need isolation under t.Parallel().
func MustRegisterStructWith[T any](reg *Registry, opts RegisterOpts) *StructMetrics[T] {
	var zero T
	t := reflect.TypeOf(zero)
	if t.Kind() != reflect.Struct {
		panic(fmt.Errorf("telemetry: MustRegisterStruct: %T is not a struct", zero))
	}

	sm := &StructMetrics[T]{
		registry: reg,
		byName:   make(map[string]int),
	}

	// First pass: collect label fields (in declaration order). The wire
	// label name comes from the metric tag (label=name), then the json
	// tag, then the lowercased Go field name as a final fallback.
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("metric")
		if tag == "" {
			continue
		}
		spec := parseMetricTag(tag)
		if spec.isLabel {
			sm.labelNames = append(sm.labelNames, labelName(f, spec))
		}
	}

	// Second pass: register metric fields.
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("metric")
		if tag == "" {
			continue
		}
		spec := parseMetricTag(tag)
		if spec.isLabel {
			continue
		}
		if spec.name == "" {
			panic(fmt.Errorf("telemetry: %s.%s metric tag missing name", t.Name(), f.Name))
		}
		full := spec.name
		if opts.Path != "" {
			full = opts.Path + "." + spec.name
		}

		entry := structField{
			goName:   f.Name,
			fullName: full,
			help:     spec.help,
			fieldIdx: i,
		}

		switch spec.kind {
		case "counter":
			entry.typ = MetricCounter
			entry.counter = reg.MustRegisterCounter(CounterOpts{
				Name:          full,
				Help:          spec.help,
				Labels:        sm.labelNames,
				StreamingOnly: spec.streamingOnly,
			})
		case "gauge":
			entry.typ = MetricGauge
			entry.gauge = reg.MustRegisterGauge(GaugeOpts{
				Name:          full,
				Help:          spec.help,
				Labels:        sm.labelNames,
				StreamingOnly: spec.streamingOnly,
			})
		case "histogram":
			entry.typ = MetricHistogram
			entry.histogram = reg.MustRegisterHistogram(HistogramOpts{
				Name:          full,
				Help:          spec.help,
				Labels:        sm.labelNames,
				Buckets:       spec.buckets,
				StreamingOnly: spec.streamingOnly,
			})
		default:
			panic(fmt.Errorf("telemetry: %s.%s metric tag missing or unknown type=%q", t.Name(), f.Name, spec.kind))
		}

		sm.byName[f.Name] = len(sm.fields)
		sm.fields = append(sm.fields, entry)
	}

	return sm
}

// WithLabelValues resolves and caches per-tuple handles. Call from cold
// paths; the returned StructHandles is safe to retain for hot-path emit.
func (m *StructMetrics[T]) WithLabelValues(labelValues ...string) *StructHandles {
	if len(labelValues) != len(m.labelNames) {
		panic(fmt.Errorf("telemetry: StructMetrics.WithLabelValues: expected %d label values, got %d", len(m.labelNames), len(labelValues)))
	}
	h := &StructHandles{
		fields: make([]fieldHandle, len(m.fields)),
		byName: m.byName,
	}
	for i, e := range m.fields {
		fh := fieldHandle{name: e.goName, typ: e.typ}
		switch e.typ {
		case MetricCounter:
			fh.counter = e.counter.WithLabelValues(labelValues...)
		case MetricGauge:
			fh.gauge = e.gauge.WithLabelValues(labelValues...)
		case MetricHistogram:
			fh.histogram = e.histogram.WithLabelValues(labelValues...)
		}
		h.fields[i] = fh
	}
	return h
}

func (m *StructMetrics[T]) LabelNames() []string {
	out := make([]string, len(m.labelNames))
	copy(out, m.labelNames)
	return out
}

// EmitFrom copies counter/gauge values from src into the SDK metrics.
// Counter fields use absolute-to-delta semantics: Add(src - lastObserved),
// so callers can pass an externally-tracked monotonic value (e.g. VPP
// stats segment). On values going backwards (counter reset), the new
// absolute is treated as a fresh increment from zero. Gauge fields are
// set directly. Histograms and label fields are skipped.
func (m *StructMetrics[T]) EmitFrom(h *StructHandles, src *T) {
	v := reflect.ValueOf(src).Elem()
	for i, e := range m.fields {
		fv := v.Field(e.fieldIdx)
		fh := &h.fields[i]
		switch e.typ {
		case MetricCounter:
			var current uint64
			switch fv.Kind() {
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				current = fv.Uint()
			default:
				continue
			}
			last := fh.counter.Value()
			if current >= last {
				fh.counter.Add(current - last)
			} else {
				fh.counter.Add(current)
			}
		case MetricGauge:
			var current float64
			switch fv.Kind() {
			case reflect.Float32, reflect.Float64:
				current = fv.Float()
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				current = float64(fv.Uint())
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				current = float64(fv.Int())
			default:
				continue
			}
			fh.gauge.Set(current)
		}
	}
}

// FillSnapshot reads the current handle values into dst. Counter/gauge
// fields whose underlying Go type is uint64 receive Counter/Gauge values
// directly; float64 fields receive gauge values; mismatched types are
// skipped. Label fields are not touched (callers fill them themselves).
// Histogram fields are skipped (they don't fit a single primitive field).
func (m *StructMetrics[T]) FillSnapshot(h *StructHandles, dst *T) {
	v := reflect.ValueOf(dst).Elem()
	for i, e := range m.fields {
		fv := v.Field(e.fieldIdx)
		if !fv.CanSet() {
			continue
		}
		fh := &h.fields[i]
		switch e.typ {
		case MetricCounter:
			if fv.Kind() == reflect.Uint64 {
				fv.SetUint(fh.counter.Value())
			} else if fv.Kind() == reflect.Float64 {
				fv.SetFloat(float64(fh.counter.Value()))
			}
		case MetricGauge:
			val := fh.gauge.Value()
			switch fv.Kind() {
			case reflect.Float64:
				fv.SetFloat(val)
			case reflect.Uint64:
				if !math.IsNaN(val) && val >= 0 {
					fv.SetUint(uint64(val))
				}
			}
		}
	}
}

// StructHandles is the per-tuple handle bundle. Inc/Add/Set/Observe are
// keyed by Go field name (single map lookup). Hot paths that emit at
// sustained rates can call Counter/Gauge/Histogram once and cache the
// raw handle to skip the lookup.
type StructHandles struct {
	fields []fieldHandle
	byName map[string]int
}

type fieldHandle struct {
	name      string
	typ       MetricType
	counter   *CounterHandle
	gauge     *GaugeHandle
	histogram *HistogramHandle
}

func (h *StructHandles) Inc(goFieldName string) {
	idx, ok := h.byName[goFieldName]
	if !ok {
		return
	}
	if c := h.fields[idx].counter; c != nil {
		c.Inc()
	}
}

func (h *StructHandles) Add(goFieldName string, delta uint64) {
	idx, ok := h.byName[goFieldName]
	if !ok {
		return
	}
	if c := h.fields[idx].counter; c != nil {
		c.Add(delta)
	}
}

func (h *StructHandles) Set(goFieldName string, v float64) {
	idx, ok := h.byName[goFieldName]
	if !ok {
		return
	}
	if g := h.fields[idx].gauge; g != nil {
		g.Set(v)
	}
}

func (h *StructHandles) Observe(goFieldName string, v float64) {
	idx, ok := h.byName[goFieldName]
	if !ok {
		return
	}
	if hi := h.fields[idx].histogram; hi != nil {
		hi.Observe(v)
	}
}

func (h *StructHandles) Counter(goFieldName string) *CounterHandle {
	idx, ok := h.byName[goFieldName]
	if !ok {
		return nil
	}
	return h.fields[idx].counter
}

func (h *StructHandles) Gauge(goFieldName string) *GaugeHandle {
	idx, ok := h.byName[goFieldName]
	if !ok {
		return nil
	}
	return h.fields[idx].gauge
}

func (h *StructHandles) Histogram(goFieldName string) *HistogramHandle {
	idx, ok := h.byName[goFieldName]
	if !ok {
		return nil
	}
	return h.fields[idx].histogram
}

// metricSpec is the parsed `metric:"..."` tag.
type metricSpec struct {
	isLabel       bool
	labelName     string // explicit label wire name, optional
	name          string
	kind          string // "counter", "gauge", "histogram"
	help          string
	streamingOnly bool
	buckets       []float64 // histogram only; nil falls back to DefaultHistogramBuckets
}

// labelName returns the wire label name for a struct field, in priority:
// (1) metric:"label=foo" tag value; (2) the json tag; (3) lowercased Go
// field name.
func labelName(f reflect.StructField, spec metricSpec) string {
	if spec.labelName != "" {
		return spec.labelName
	}
	if jsonTag := f.Tag.Get("json"); jsonTag != "" {
		if comma := strings.IndexByte(jsonTag, ','); comma >= 0 {
			jsonTag = jsonTag[:comma]
		}
		if jsonTag != "" && jsonTag != "-" {
			return jsonTag
		}
	}
	return strings.ToLower(f.Name)
}

// parseMetricTag understands forms:
//
//	metric:"label=server"
//	metric:"name=auth.requests,type=counter,help=RADIUS Access-Request packets sent."
//	metric:"name=session.setup_seconds,type=histogram,help=...,buckets=0.01;0.05;0.1;0.5;1"
//	metric:"name=session.bytes,type=counter,help=...,streaming_only"
//
// Comma separates fields. Within a value, ',' is not allowed (use the literal
// help text without commas, or escape via the unused ';' if buckets are needed).
var parseMutex sync.Mutex // serialize panics in dev only

func parseMetricTag(tag string) metricSpec {
	parseMutex.Lock()
	defer parseMutex.Unlock()

	var spec metricSpec
	for _, part := range strings.Split(tag, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, hasEq := strings.Cut(part, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch {
		case !hasEq && key == "label":
			spec.isLabel = true
		case !hasEq && (key == "counter" || key == "gauge" || key == "histogram"):
			spec.kind = key
		case !hasEq && key == "streaming_only":
			spec.streamingOnly = true
		case key == "label":
			spec.isLabel = true
			spec.labelName = value
		case key == "name":
			spec.name = value
		case key == "type":
			spec.kind = value
		case key == "help":
			spec.help = value
		case key == "streaming_only":
			spec.streamingOnly = value == "true"
		case key == "buckets":
			spec.buckets = parseBuckets(value)
		}
	}
	return spec
}

func parseBuckets(s string) []float64 {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ";")
	out := make([]float64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var f float64
		_, err := fmt.Sscan(p, &f)
		if err != nil {
			panic(fmt.Errorf("telemetry: invalid bucket %q: %w", p, err))
		}
		out = append(out, f)
	}
	return out
}
