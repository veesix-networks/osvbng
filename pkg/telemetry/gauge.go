// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"math"
	"sync"
	"sync/atomic"
)

// GaugeOpts describes a gauge metric registration.
type GaugeOpts struct {
	Name               string
	Help               string
	Labels             []string
	StreamingOnly      bool
	MaxSeriesPerMetric int
}

// Gauge is a registered float64 gauge metric. Set is single-instruction
// atomic store; Add and Sub use a math.Float64bits CAS loop because
// sync/atomic has no AddFloat64.
type Gauge struct {
	opts     GaugeOpts
	registry *Registry

	series      sync.Map
	seriesCount atomic.Int64

	tombstoneOnce sync.Once
	tombstone     *GaugeHandle

	cardinalityDrops   atomic.Uint64
	unknownSeriesEmits atomic.Uint64
	staleHandleEmits   atomic.Uint64

	internalLabels []LabelPair

	dirty atomic.Bool
}

func (g *Gauge) name() string                   { return g.opts.Name }
func (g *Gauge) help() string                   { return g.opts.Help }
func (g *Gauge) labelNames() []string           { return g.opts.Labels }
func (g *Gauge) metricType() MetricType         { return MetricGauge }
func (g *Gauge) streamingOnly() bool            { return g.opts.StreamingOnly }
func (g *Gauge) swapDirty() bool                { return g.dirty.Swap(false) }
func (g *Gauge) cardinalityDropsLoad() uint64   { return g.cardinalityDrops.Load() }
func (g *Gauge) unknownSeriesEmitsLoad() uint64 { return g.unknownSeriesEmits.Load() }
func (g *Gauge) staleHandleEmitsLoad() uint64   { return g.staleHandleEmits.Load() }
func (g *Gauge) seriesCountLoad() int64         { return g.seriesCount.Load() }
func (g *Gauge) internalLabelsRef() []LabelPair { return g.internalLabels }

// GaugeHandle is the per-series emit handle.
type GaugeHandle struct {
	gauge       *Gauge
	bits        atomic.Uint64
	labelValues []string
	labels      []LabelPair

	isTombstone bool
	stale       atomic.Bool
}

// Set replaces the handle's value.
func (h *GaugeHandle) Set(value float64) {
	if h.isTombstone {
		h.gauge.cardinalityDrops.Add(1)
		return
	}
	if h.stale.Load() {
		h.gauge.staleHandleEmits.Add(1)
		return
	}
	h.bits.Store(math.Float64bits(value))
	h.gauge.markDirty()
}

// Add increments the handle's value by delta. Implemented as a CAS loop
// over math.Float64bits because sync/atomic has no AddFloat64.
func (h *GaugeHandle) Add(delta float64) {
	if h.isTombstone {
		h.gauge.cardinalityDrops.Add(1)
		return
	}
	if h.stale.Load() {
		h.gauge.staleHandleEmits.Add(1)
		return
	}
	for {
		old := h.bits.Load()
		newVal := math.Float64bits(math.Float64frombits(old) + delta)
		if h.bits.CompareAndSwap(old, newVal) {
			h.gauge.markDirty()
			return
		}
	}
}

// Sub decrements the handle's value by delta.
func (h *GaugeHandle) Sub(delta float64) { h.Add(-delta) }

// Inc adds 1 to the handle's value.
func (h *GaugeHandle) Inc() { h.Add(1) }

// Dec subtracts 1 from the handle's value.
func (h *GaugeHandle) Dec() { h.Add(-1) }

// Value returns the current handle value. Used by snapshot and tests.
func (h *GaugeHandle) Value() float64 {
	return math.Float64frombits(h.bits.Load())
}

func (g *Gauge) markDirty() {
	if g.registry == nil {
		return
	}
	if g.registry.subscriberCount.Load() == 0 {
		return
	}
	if g.dirty.Load() {
		return
	}
	g.dirty.CompareAndSwap(false, true)
}

// WithLabelValues resolves and returns the handle for the supplied tuple.
// See Counter.WithLabelValues for cold-create semantics.
func (g *Gauge) WithLabelValues(labelValues ...string) *GaugeHandle {
	if len(labelValues) != len(g.opts.Labels) {
		panic(ErrLabelCount)
	}

	h := hashLabelValues(labelValues)

	if v, ok := g.series.Load(h); ok {
		entry := v.(*GaugeHandle)
		if labelValuesEqual(entry.labelValues, labelValues) {
			return entry
		}
	}

	if g.opts.MaxSeriesPerMetric > 0 && g.seriesCount.Load() >= int64(g.opts.MaxSeriesPerMetric) {
		return g.tombstoneHandle()
	}

	candidate := &GaugeHandle{
		gauge:       g,
		labelValues: copyStrings(labelValues),
	}
	candidate.labels = makeLabelPairs(g.opts.Labels, candidate.labelValues)

	actual, loaded := g.series.LoadOrStore(h, candidate)
	if loaded {
		existing := actual.(*GaugeHandle)
		if labelValuesEqual(existing.labelValues, labelValues) {
			return existing
		}
		return g.tombstoneHandle()
	}

	g.seriesCount.Add(1)

	if g.opts.MaxSeriesPerMetric > 0 && g.seriesCount.Load() > int64(g.opts.MaxSeriesPerMetric) {
		g.series.Delete(h)
		g.seriesCount.Add(-1)
		return g.tombstoneHandle()
	}

	return candidate
}

// Set resolves the supplied tuple and stores value. Lookup-or-drop: an
// unknown tuple bumps the metric's unknown-series-emits counter.
func (g *Gauge) Set(value float64, labelValues ...string) {
	if len(labelValues) != len(g.opts.Labels) {
		panic(ErrLabelCount)
	}
	h := hashLabelValues(labelValues)
	if v, ok := g.series.Load(h); ok {
		entry := v.(*GaugeHandle)
		if labelValuesEqual(entry.labelValues, labelValues) {
			entry.Set(value)
			return
		}
	}
	g.unknownSeriesEmits.Add(1)
}

// Add adds delta to the resolved tuple. Lookup-or-drop semantics.
func (g *Gauge) Add(delta float64, labelValues ...string) {
	if len(labelValues) != len(g.opts.Labels) {
		panic(ErrLabelCount)
	}
	h := hashLabelValues(labelValues)
	if v, ok := g.series.Load(h); ok {
		entry := v.(*GaugeHandle)
		if labelValuesEqual(entry.labelValues, labelValues) {
			entry.Add(delta)
			return
		}
	}
	g.unknownSeriesEmits.Add(1)
}

// UnregisterSeries removes the series for the supplied label tuple. See
// Counter.UnregisterSeries for race semantics.
func (g *Gauge) UnregisterSeries(labelValues ...string) bool {
	if len(labelValues) != len(g.opts.Labels) {
		panic(ErrLabelCount)
	}
	h := hashLabelValues(labelValues)
	v, ok := g.series.Load(h)
	if !ok {
		return false
	}
	entry := v.(*GaugeHandle)
	if !labelValuesEqual(entry.labelValues, labelValues) {
		return false
	}
	g.series.Delete(h)
	g.seriesCount.Add(-1)
	entry.stale.Store(true)
	return true
}

func (g *Gauge) tombstoneHandle() *GaugeHandle {
	g.tombstoneOnce.Do(func() {
		g.tombstone = &GaugeHandle{
			gauge:       g,
			isTombstone: true,
		}
	})
	return g.tombstone
}

func (g *Gauge) appendSamples(dst []Sample) []Sample {
	g.series.Range(func(_, v any) bool {
		entry := v.(*GaugeHandle)
		dst = append(dst, Sample{
			Name:          g.opts.Name,
			Help:          g.opts.Help,
			Type:          MetricGauge,
			Labels:        entry.labels,
			StreamingOnly: g.opts.StreamingOnly,
			Value:         math.Float64frombits(entry.bits.Load()),
		})
		return true
	})
	return dst
}
