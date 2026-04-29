// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"sync"
	"sync/atomic"
)

// CounterOpts describes a counter metric registration.
type CounterOpts struct {
	Name               string
	Help               string
	Labels             []string
	StreamingOnly      bool
	MaxSeriesPerMetric int
}

// Counter is a registered monotonic counter metric. Use WithLabelValues to
// resolve a per-tuple handle; cache the handle for the hot emit path.
type Counter struct {
	opts     CounterOpts
	registry *Registry

	series      sync.Map
	seriesCount atomic.Int64

	tombstoneOnce sync.Once
	tombstone     *CounterHandle

	cardinalityDrops   atomic.Uint64
	unknownSeriesEmits atomic.Uint64
	staleHandleEmits   atomic.Uint64

	internalLabels []LabelPair

	dirty atomic.Bool
}

func (c *Counter) name() string                 { return c.opts.Name }
func (c *Counter) help() string                 { return c.opts.Help }
func (c *Counter) labelNames() []string         { return c.opts.Labels }
func (c *Counter) metricType() MetricType       { return MetricCounter }
func (c *Counter) streamingOnly() bool          { return c.opts.StreamingOnly }
func (c *Counter) swapDirty() bool              { return c.dirty.Swap(false) }
func (c *Counter) cardinalityDropsLoad() uint64 { return c.cardinalityDrops.Load() }
func (c *Counter) unknownSeriesEmitsLoad() uint64 {
	return c.unknownSeriesEmits.Load()
}
func (c *Counter) staleHandleEmitsLoad() uint64   { return c.staleHandleEmits.Load() }
func (c *Counter) seriesCountLoad() int64         { return c.seriesCount.Load() }
func (c *Counter) internalLabelsRef() []LabelPair { return c.internalLabels }

// CounterHandle is the per-series emit handle. Hot path: Inc/Add are a
// single atomic add on the embedded value.
type CounterHandle struct {
	counter     *Counter
	value       atomic.Uint64
	labelValues []string
	labels      []LabelPair

	// tombstone-only: when true, Inc/Add bump the parent's cardinalityDrops
	// counter instead of the per-series value.
	isTombstone bool

	// stale: set true when UnregisterSeries removes this handle. Subsequent
	// Inc/Add bump staleHandleEmits.
	stale atomic.Bool
}

// Inc adds 1 to the counter handle's value.
func (h *CounterHandle) Inc() {
	h.Add(1)
}

// Add adds delta to the counter handle's value. If the handle is the
// metric's tombstone, the value is dropped and the metric's cardinality
// drop counter is incremented instead. If the handle is stale (the series
// was unregistered), the metric's stale-handle counter is incremented.
func (h *CounterHandle) Add(delta uint64) {
	if h.isTombstone {
		h.counter.cardinalityDrops.Add(delta)
		return
	}
	if h.stale.Load() {
		h.counter.staleHandleEmits.Add(delta)
		return
	}
	h.value.Add(delta)
	h.counter.markDirty()
}

func (c *Counter) markDirty() {
	if c.registry == nil {
		return
	}
	if c.registry.subscriberCount.Load() == 0 {
		return
	}
	if c.dirty.Load() {
		return
	}
	c.dirty.CompareAndSwap(false, true)
}

// WithLabelValues resolves (and creates if necessary) the handle for the
// supplied label tuple. Safe to call from the cold path; cache the result
// for hot emit.
func (c *Counter) WithLabelValues(labelValues ...string) *CounterHandle {
	if len(labelValues) != len(c.opts.Labels) {
		panic(ErrLabelCount)
	}

	h := hashLabelValues(labelValues)

	if v, ok := c.series.Load(h); ok {
		entry := v.(*CounterHandle)
		if labelValuesEqual(entry.labelValues, labelValues) {
			return entry
		}
	}

	if c.opts.MaxSeriesPerMetric > 0 && c.seriesCount.Load() >= int64(c.opts.MaxSeriesPerMetric) {
		return c.tombstoneHandle()
	}

	candidate := &CounterHandle{
		counter:     c,
		labelValues: copyStrings(labelValues),
	}
	candidate.labels = makeLabelPairs(c.opts.Labels, candidate.labelValues)

	actual, loaded := c.series.LoadOrStore(h, candidate)
	if loaded {
		existing := actual.(*CounterHandle)
		if labelValuesEqual(existing.labelValues, labelValues) {
			return existing
		}
		return c.tombstoneHandle()
	}

	c.seriesCount.Add(1)

	if c.opts.MaxSeriesPerMetric > 0 && c.seriesCount.Load() > int64(c.opts.MaxSeriesPerMetric) {
		c.series.Delete(h)
		c.seriesCount.Add(-1)
		return c.tombstoneHandle()
	}

	return candidate
}

// Inc resolves the supplied label tuple and increments the corresponding
// series. UNLIKE WithLabelValues, this method NEVER creates a new series:
// if the tuple has not been resolved before, the emit is dropped and the
// metric's unknown-series-emits counter is incremented. Hot-path callers
// MUST cache a CounterHandle from WithLabelValues; this method exists for
// callers that already know the tuple is resolved.
func (c *Counter) Inc(labelValues ...string) {
	c.Add(1, labelValues...)
}

func (c *Counter) Add(delta uint64, labelValues ...string) {
	if len(labelValues) != len(c.opts.Labels) {
		panic(ErrLabelCount)
	}
	h := hashLabelValues(labelValues)
	if v, ok := c.series.Load(h); ok {
		entry := v.(*CounterHandle)
		if labelValuesEqual(entry.labelValues, labelValues) {
			entry.Add(delta)
			return
		}
	}
	c.unknownSeriesEmits.Add(delta)
}

// UnregisterSeries removes the series for the supplied label tuple.
// Returns true if a series existed and was removed. Retained handles for
// the removed tuple become stale: subsequent emits via the handle bump
// the metric's stale-handle counter instead of the per-series value.
func (c *Counter) UnregisterSeries(labelValues ...string) bool {
	if len(labelValues) != len(c.opts.Labels) {
		panic(ErrLabelCount)
	}
	h := hashLabelValues(labelValues)
	v, ok := c.series.Load(h)
	if !ok {
		return false
	}
	entry := v.(*CounterHandle)
	if !labelValuesEqual(entry.labelValues, labelValues) {
		return false
	}
	c.series.Delete(h)
	c.seriesCount.Add(-1)
	entry.stale.Store(true)
	return true
}

func (c *Counter) tombstoneHandle() *CounterHandle {
	c.tombstoneOnce.Do(func() {
		c.tombstone = &CounterHandle{
			counter:     c,
			isTombstone: true,
		}
	})
	return c.tombstone
}

func (c *Counter) appendSamples(dst []Sample) []Sample {
	c.series.Range(func(_, v any) bool {
		entry := v.(*CounterHandle)
		dst = append(dst, Sample{
			Name:          c.opts.Name,
			Type:          MetricCounter,
			Labels:        entry.labels,
			StreamingOnly: c.opts.StreamingOnly,
			Value:         float64(entry.value.Load()),
		})
		return true
	})
	return dst
}

func copyStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func makeLabelPairs(names, values []string) []LabelPair {
	if len(names) == 0 {
		return nil
	}
	out := make([]LabelPair, len(names))
	for i := range names {
		out[i] = LabelPair{Name: names[i], Value: values[i]}
	}
	return out
}
