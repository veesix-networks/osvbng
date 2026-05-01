// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// structMetrics is the show-driven recursive walker's per-type bound state.
// A separate tree from StructMetrics[T] (which keeps the flat field model
// AAA depends on); each show registration owns its own tree, with one
// *structMetrics per nested struct reached by `metric:"flatten"`.
type structMetrics struct {
	typ reflect.Type

	labelIdxs  []int    // typ.Field(i) indexes for label fields, declaration order
	labelNames []string // wire names parallel to labelIdxs

	mapKeyLabelPos int // index into labelNames where a map key projects, -1 if none

	fields []walkerField
}

type walkerField struct {
	fieldIdx int
	kind     walkerKind

	// kind == walkerValue
	mtype       MetricType
	counter     *Counter
	gauge       *Gauge
	histogram   *Histogram
	retainStale bool

	// kind == walkerFlatten
	inner *structMetrics
}

type walkerKind uint8

const (
	walkerValue walkerKind = iota
	walkerFlatten
)

type bindContext struct {
	reg             *Registry
	seenMetricNames map[string]struct{}
	visiting        map[reflect.Type]struct{}
	inherited       []string
	insideFlatten   bool // true once we recurse into a flatten field — used to reject nested maps
}

// bindShowType walks t and registers every value-metric field against reg,
// recursing into `metric:"flatten"` fields. All registration-time
// validation panics here so misconfigured plugins fail at process start.
func bindShowType(reg *Registry, t reflect.Type) *structMetrics {
	bc := &bindContext{
		reg:             reg,
		seenMetricNames: make(map[string]struct{}),
		visiting:        make(map[reflect.Type]struct{}),
	}
	return bc.bindStruct(t)
}

func (bc *bindContext) bindStruct(t reflect.Type) *structMetrics {
	if t.Kind() != reflect.Struct {
		panic(fmt.Errorf("telemetry: bindShowType: expected struct kind, got %s", t.Kind()))
	}
	if _, cycle := bc.visiting[t]; cycle {
		panic(fmt.Errorf("telemetry: cyclic flatten path through %s", t))
	}
	bc.visiting[t] = struct{}{}
	defer delete(bc.visiting, t)

	sm := &structMetrics{typ: t, mapKeyLabelPos: -1}

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("metric")
		if tag == "" {
			continue
		}
		spec := parseMetricTag(tag)
		if !spec.isLabel {
			continue
		}
		wire := labelName(f, spec)
		sm.labelIdxs = append(sm.labelIdxs, i)
		sm.labelNames = append(sm.labelNames, wire)
		if spec.mapKey {
			if sm.mapKeyLabelPos != -1 {
				panic(fmt.Errorf("telemetry: %s has more than one map_key field", t))
			}
			sm.mapKeyLabelPos = len(sm.labelNames) - 1
			switch f.Type.Kind() {
			case reflect.String,
				reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
				reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
				reflect.Bool:
			default:
				panic(fmt.Errorf("telemetry: %s.%s: map_key on unsupported kind %s", t, f.Name, f.Type.Kind()))
			}
		}
	}

	combined := make([]string, 0, len(bc.inherited)+len(sm.labelNames))
	combined = append(combined, bc.inherited...)
	seen := make(map[string]struct{}, len(combined)+len(sm.labelNames))
	for _, n := range combined {
		seen[n] = struct{}{}
	}
	for _, n := range sm.labelNames {
		if _, dup := seen[n]; dup {
			panic(fmt.Errorf("telemetry: %s: label name %q collides with an inherited label", t, n))
		}
		seen[n] = struct{}{}
		combined = append(combined, n)
	}

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

		if spec.flatten {
			if spec.kind != "" || spec.name != "" {
				panic(fmt.Errorf("telemetry: %s.%s: flatten cannot combine with a value-metric tag", t, f.Name))
			}
			if isMapShape(f.Type) && bc.insideFlatten {
				panic(fmt.Errorf("telemetry: %s.%s: nested map flatten is not supported", t, f.Name))
			}
			innerType := unwrapFlatten(t, f)
			saved := bc.inherited
			savedInside := bc.insideFlatten
			bc.inherited = combined
			bc.insideFlatten = true
			inner := bc.bindStruct(innerType)
			bc.inherited = saved
			bc.insideFlatten = savedInside
			sm.fields = append(sm.fields, walkerField{
				fieldIdx: i,
				kind:     walkerFlatten,
				inner:    inner,
			})
			continue
		}

		if spec.name == "" {
			panic(fmt.Errorf("telemetry: %s.%s metric tag missing name", t, f.Name))
		}
		if spec.kind == "" {
			panic(fmt.Errorf("telemetry: %s.%s metric tag missing or unknown type=%q", t, f.Name, spec.kind))
		}
		if _, dup := bc.seenMetricNames[spec.name]; dup {
			panic(fmt.Errorf("telemetry: duplicate metric name %q across flatten paths", spec.name))
		}
		bc.seenMetricNames[spec.name] = struct{}{}

		wf := walkerField{fieldIdx: i, kind: walkerValue, retainStale: spec.retainStale}
		switch spec.kind {
		case "counter":
			wf.mtype = MetricCounter
			wf.counter = bc.reg.MustRegisterCounter(CounterOpts{
				Name:          spec.name,
				Help:          spec.help,
				Labels:        combined,
				StreamingOnly: spec.streamingOnly,
			})
		case "gauge":
			wf.mtype = MetricGauge
			wf.gauge = bc.reg.MustRegisterGauge(GaugeOpts{
				Name:          spec.name,
				Help:          spec.help,
				Labels:        combined,
				StreamingOnly: spec.streamingOnly,
			})
		case "histogram":
			wf.mtype = MetricHistogram
			wf.histogram = bc.reg.MustRegisterHistogram(HistogramOpts{
				Name:          spec.name,
				Help:          spec.help,
				Labels:        combined,
				Buckets:       spec.buckets,
				StreamingOnly: spec.streamingOnly,
			})
		default:
			panic(fmt.Errorf("telemetry: %s.%s metric tag missing or unknown type=%q", t, f.Name, spec.kind))
		}
		sm.fields = append(sm.fields, wf)
	}

	return sm
}

// unwrapFlatten resolves the inner struct type of a `metric:"flatten"`
// field. Pointer-to-supported-kind is unwrapped; map handling lives in
// Phase 3 and is rejected here for non-map shapes.
func unwrapFlatten(parent reflect.Type, f reflect.StructField) reflect.Type {
	t := f.Type
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Struct:
		return t
	case reflect.Slice, reflect.Array:
		elem := t.Elem()
		for elem.Kind() == reflect.Pointer {
			elem = elem.Elem()
		}
		if elem.Kind() != reflect.Struct {
			panic(fmt.Errorf("telemetry: %s.%s: flatten slice/array element must be a struct, got %s", parent, f.Name, elem.Kind()))
		}
		return elem
	case reflect.Map:
		// Phase 3 adds map iteration. For now, accept maps so the walker
		// builds; Phase 3 will introduce nested-map rejection and the
		// emit-time iteration logic.
		elem := t.Elem()
		for elem.Kind() == reflect.Pointer {
			elem = elem.Elem()
		}
		if elem.Kind() == reflect.Slice {
			elem = elem.Elem()
			for elem.Kind() == reflect.Pointer {
				elem = elem.Elem()
			}
		}
		if elem.Kind() != reflect.Struct {
			panic(fmt.Errorf("telemetry: %s.%s: flatten map value must be a struct, got %s", parent, f.Name, elem.Kind()))
		}
		return elem
	default:
		panic(fmt.Errorf("telemetry: %s.%s: flatten on unsupported kind %s", parent, f.Name, t.Kind()))
	}
}

// pollState records every (value-metric field, label-tuple) pair the
// walker emits within one poll. The poll loop diffs the new pollState
// against the previous successful poll's pollState and unregisters any
// tuple absent from the new set (D11 default clear_on_absent). Fields
// tagged metric:"retain_stale" are not tracked.
type pollState struct {
	seen map[*walkerField]map[string][]string
}

func newPollState() *pollState {
	return &pollState{seen: make(map[*walkerField]map[string][]string)}
}

func (ps *pollState) record(wf *walkerField, labels []string) {
	if ps == nil || wf.retainStale {
		return
	}
	inner := ps.seen[wf]
	if inner == nil {
		inner = make(map[string][]string)
		ps.seen[wf] = inner
	}
	key := strings.Join(labels, "\x00")
	if _, ok := inner[key]; ok {
		return
	}
	cp := append([]string(nil), labels...)
	inner[key] = cp
}

// reconcile compares prev to current. Tuples present in prev but absent
// in current have their corresponding metric series UnregisterSeries'd.
// Returns the new "previous" state for the next poll (== current).
func reconcile(prev, current *pollState) *pollState {
	if prev == nil {
		return current
	}
	for wf, prevTuples := range prev.seen {
		currentTuples := current.seen[wf]
		for prevKey, prevLabels := range prevTuples {
			if _, ok := currentTuples[prevKey]; ok {
				continue
			}
			switch wf.mtype {
			case MetricCounter:
				wf.counter.UnregisterSeries(prevLabels...)
			case MetricGauge:
				wf.gauge.UnregisterSeries(prevLabels...)
			case MetricHistogram:
				wf.histogram.UnregisterSeries(prevLabels...)
			}
		}
	}
	return current
}

// walk dispatches on the top-level reflect.Kind of the show handler's
// snapshot value. Pointers are unwrapped; struct/slice/array iterate the
// flat case via emit; maps iterate via emitMap. Phase 4's RegisterMetric
// poll loop calls this once per poll with the result of Snapshot. ps may
// be nil for tests that bypass the lifecycle path.
func (sm *structMetrics) walk(rv reflect.Value, ps *pollState) {
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Struct:
		sm.emit(rv, nil, ps)
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			el := rv.Index(i)
			for el.Kind() == reflect.Pointer {
				if el.IsNil() {
					goto nextSlice
				}
				el = el.Elem()
			}
			if el.Kind() == reflect.Struct {
				sm.emit(el, nil, ps)
			}
		nextSlice:
		}
	case reflect.Map:
		emitMap(sm, rv, nil, ps)
	}
}

// emit walks rv and writes every leaf value field into the registry.
// inheritedLabels is the resolved label tuple from outer scopes; this
// struct's local labels append to it before each leaf emission.
//
// Phase 2 supports struct, slice, array, and pointer-to-those for flatten.
// Phase 3 adds map iteration with map_key projection.
func (sm *structMetrics) emit(rv reflect.Value, inheritedLabels []string, ps *pollState) {
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return
	}

	labels := append([]string(nil), inheritedLabels...)
	for _, idx := range sm.labelIdxs {
		labels = append(labels, formatLabelValue(rv.Field(idx)))
	}

	for i := range sm.fields {
		wf := &sm.fields[i]
		switch wf.kind {
		case walkerValue:
			emitValueField(wf, rv.Field(wf.fieldIdx), labels, ps)
		case walkerFlatten:
			emitFlattenField(wf.inner, rv.Field(wf.fieldIdx), labels, ps)
		}
	}
}

func emitValueField(wf *walkerField, fv reflect.Value, labels []string, ps *pollState) {
	switch wf.mtype {
	case MetricCounter:
		cur, ok := readUintField(fv)
		if !ok {
			return
		}
		h := wf.counter.WithLabelValues(labels...)
		last := h.Value()
		if cur >= last {
			h.Add(cur - last)
		} else {
			h.Add(cur)
		}
	case MetricGauge:
		cur, ok := readFloatField(fv)
		if !ok {
			return
		}
		wf.gauge.WithLabelValues(labels...).Set(cur)
	default:
		return
	}
	ps.record(wf, labels)
}

func emitFlattenField(inner *structMetrics, fv reflect.Value, inherited []string, ps *pollState) {
	for fv.Kind() == reflect.Pointer {
		if fv.IsNil() {
			return
		}
		fv = fv.Elem()
	}
	switch fv.Kind() {
	case reflect.Struct:
		inner.emit(fv, inherited, ps)
	case reflect.Slice, reflect.Array:
		for i := 0; i < fv.Len(); i++ {
			inner.emit(fv.Index(i), inherited, ps)
		}
	case reflect.Map:
		emitMap(inner, fv, inherited, ps)
	}
}

// emitMap iterates a map source. If the inner type carries a `metric:"map_key"`
// field, the map key is formatted via strconv (no fmt.Sprintf allocations
// for supported numeric and bool kinds) and substituted into the inner
// emission's label tuple. Otherwise the map key is silently dropped.
func emitMap(inner *structMetrics, mv reflect.Value, inherited []string, ps *pollState) {
	if !mv.IsValid() || mv.IsNil() {
		return
	}
	iter := mv.MapRange()
	for iter.Next() {
		key := formatMapKey(iter.Key())
		val := iter.Value()
		emitMapValue(inner, val, inherited, key, ps)
	}
}

func emitMapValue(inner *structMetrics, val reflect.Value, inherited []string, key string, ps *pollState) {
	for val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return
		}
		val = val.Elem()
	}
	switch val.Kind() {
	case reflect.Struct:
		inner.emitWithMapKey(val, inherited, key, ps)
	case reflect.Slice, reflect.Array:
		for i := 0; i < val.Len(); i++ {
			el := val.Index(i)
			for el.Kind() == reflect.Pointer {
				if el.IsNil() {
					goto next
				}
				el = el.Elem()
			}
			if el.Kind() == reflect.Struct {
				inner.emitWithMapKey(el, inherited, key, ps)
			}
		next:
		}
	}
}

// emitWithMapKey emits a struct instance whose label tuple has the map
// key substituted at sm.mapKeyLabelPos (if set). The mapKey arg is the
// already-formatted string.
func (sm *structMetrics) emitWithMapKey(rv reflect.Value, inheritedLabels []string, mapKey string, ps *pollState) {
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return
	}

	labels := append([]string(nil), inheritedLabels...)
	for pos, idx := range sm.labelIdxs {
		if pos == sm.mapKeyLabelPos {
			labels = append(labels, mapKey)
			continue
		}
		labels = append(labels, formatLabelValue(rv.Field(idx)))
	}

	for i := range sm.fields {
		wf := &sm.fields[i]
		switch wf.kind {
		case walkerValue:
			emitValueField(wf, rv.Field(wf.fieldIdx), labels, ps)
		case walkerFlatten:
			emitFlattenField(wf.inner, rv.Field(wf.fieldIdx), labels, ps)
		}
	}
}

func formatMapKey(k reflect.Value) string {
	switch k.Kind() {
	case reflect.String:
		return k.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(k.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(k.Uint(), 10)
	case reflect.Bool:
		return strconv.FormatBool(k.Bool())
	}
	return ""
}

// isMapShape returns true if t (after pointer unwrap) is a map kind.
func isMapShape(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Kind() == reflect.Map
}

func readUintField(fv reflect.Value) (uint64, bool) {
	switch fv.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fv.Uint(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v := fv.Int()
		if v < 0 {
			return 0, false
		}
		return uint64(v), true
	}
	return 0, false
}

func readFloatField(fv reflect.Value) (float64, bool) {
	switch fv.Kind() {
	case reflect.Float32, reflect.Float64:
		return fv.Float(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(fv.Uint()), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(fv.Int()), true
	case reflect.Bool:
		if fv.Bool() {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}
