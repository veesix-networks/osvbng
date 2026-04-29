// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"sync/atomic"
)

// HistogramOpts describes a histogram metric registration.
type HistogramOpts struct {
	Name               string
	Help               string
	Labels             []string
	Buckets            []float64
	StreamingOnly      bool
	MaxSeriesPerMetric int
}

// DefaultHistogramBuckets matches the Prometheus client_golang default
// bucket boundaries: a reasonable starting point for general latency or
// size distributions.
var DefaultHistogramBuckets = []float64{
	0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
}

// Histogram is a registered histogram metric. Per-series storage is a
// fixed-size []atomic.Uint64 of bucket counters, plus an atomic count and
// an atomic-bits-CAS sum.
type Histogram struct {
	opts     HistogramOpts
	registry *Registry

	series      sync.Map
	seriesCount atomic.Int64

	tombstoneOnce sync.Once
	tombstone     *HistogramHandle

	cardinalityDrops   atomic.Uint64
	unknownSeriesEmits atomic.Uint64
	staleHandleEmits   atomic.Uint64

	internalLabels []LabelPair

	dirty atomic.Bool
}

func (h *Histogram) name() string                   { return h.opts.Name }
func (h *Histogram) help() string                   { return h.opts.Help }
func (h *Histogram) labelNames() []string           { return h.opts.Labels }
func (h *Histogram) metricType() MetricType         { return MetricHistogram }
func (h *Histogram) streamingOnly() bool            { return h.opts.StreamingOnly }
func (h *Histogram) swapDirty() bool                { return h.dirty.Swap(false) }
func (h *Histogram) cardinalityDropsLoad() uint64   { return h.cardinalityDrops.Load() }
func (h *Histogram) unknownSeriesEmitsLoad() uint64 { return h.unknownSeriesEmits.Load() }
func (h *Histogram) staleHandleEmitsLoad() uint64   { return h.staleHandleEmits.Load() }
func (h *Histogram) seriesCountLoad() int64         { return h.seriesCount.Load() }
func (h *Histogram) internalLabelsRef() []LabelPair { return h.internalLabels }

// HistogramHandle is the per-series emit handle. Buckets is one larger
// than opts.Buckets — the final slot accumulates observations >= the
// largest configured bucket ceiling.
type HistogramHandle struct {
	histogram   *Histogram
	bucketCount []atomic.Uint64
	count       atomic.Uint64
	sumBits     atomic.Uint64
	labelValues []string
	labels      []LabelPair

	isTombstone bool
	stale       atomic.Bool
}

// Observe records value into the appropriate bucket and updates count + sum.
func (h *HistogramHandle) Observe(value float64) {
	if h.isTombstone {
		h.histogram.cardinalityDrops.Add(1)
		return
	}
	if h.stale.Load() {
		h.histogram.staleHandleEmits.Add(1)
		return
	}

	idx := bucketIndex(h.histogram.opts.Buckets, value)
	h.bucketCount[idx].Add(1)
	h.count.Add(1)

	for {
		old := h.sumBits.Load()
		newVal := math.Float64bits(math.Float64frombits(old) + value)
		if h.sumBits.CompareAndSwap(old, newVal) {
			break
		}
	}

	h.histogram.markDirty()
}

// bucketIndex returns the index in a Buckets+1 slice for value. Bucket
// boundaries are upper-inclusive; the index past the last boundary is the
// "+Inf" overflow.
func bucketIndex(buckets []float64, value float64) int {
	for i, b := range buckets {
		if value <= b {
			return i
		}
	}
	return len(buckets)
}

func (h *Histogram) markDirty() {
	if h.registry == nil {
		return
	}
	if h.registry.subscriberCount.Load() == 0 {
		return
	}
	if h.dirty.Load() {
		return
	}
	h.dirty.CompareAndSwap(false, true)
}

// WithLabelValues resolves and returns the handle for the supplied tuple.
func (h *Histogram) WithLabelValues(labelValues ...string) *HistogramHandle {
	if len(labelValues) != len(h.opts.Labels) {
		panic(ErrLabelCount)
	}

	hash := hashLabelValues(labelValues)

	if v, ok := h.series.Load(hash); ok {
		entry := v.(*HistogramHandle)
		if labelValuesEqual(entry.labelValues, labelValues) {
			return entry
		}
	}

	if h.opts.MaxSeriesPerMetric > 0 && h.seriesCount.Load() >= int64(h.opts.MaxSeriesPerMetric) {
		return h.tombstoneHandle()
	}

	candidate := &HistogramHandle{
		histogram:   h,
		bucketCount: make([]atomic.Uint64, len(h.opts.Buckets)+1),
		labelValues: copyStrings(labelValues),
	}
	candidate.labels = makeLabelPairs(h.opts.Labels, candidate.labelValues)

	actual, loaded := h.series.LoadOrStore(hash, candidate)
	if loaded {
		existing := actual.(*HistogramHandle)
		if labelValuesEqual(existing.labelValues, labelValues) {
			return existing
		}
		return h.tombstoneHandle()
	}

	h.seriesCount.Add(1)

	if h.opts.MaxSeriesPerMetric > 0 && h.seriesCount.Load() > int64(h.opts.MaxSeriesPerMetric) {
		h.series.Delete(hash)
		h.seriesCount.Add(-1)
		return h.tombstoneHandle()
	}

	return candidate
}

// Observe resolves the supplied tuple and records value. Lookup-or-drop
// semantics; an unknown tuple bumps the metric's unknown-series-emits.
func (h *Histogram) Observe(value float64, labelValues ...string) {
	if len(labelValues) != len(h.opts.Labels) {
		panic(ErrLabelCount)
	}
	hash := hashLabelValues(labelValues)
	if v, ok := h.series.Load(hash); ok {
		entry := v.(*HistogramHandle)
		if labelValuesEqual(entry.labelValues, labelValues) {
			entry.Observe(value)
			return
		}
	}
	h.unknownSeriesEmits.Add(1)
}

// UnregisterSeries removes the series for the supplied label tuple.
func (h *Histogram) UnregisterSeries(labelValues ...string) bool {
	if len(labelValues) != len(h.opts.Labels) {
		panic(ErrLabelCount)
	}
	hash := hashLabelValues(labelValues)
	v, ok := h.series.Load(hash)
	if !ok {
		return false
	}
	entry := v.(*HistogramHandle)
	if !labelValuesEqual(entry.labelValues, labelValues) {
		return false
	}
	h.series.Delete(hash)
	h.seriesCount.Add(-1)
	entry.stale.Store(true)
	return true
}

func (h *Histogram) tombstoneHandle() *HistogramHandle {
	h.tombstoneOnce.Do(func() {
		h.tombstone = &HistogramHandle{
			histogram:   h,
			isTombstone: true,
		}
	})
	return h.tombstone
}

func (h *Histogram) appendSamples(dst []Sample) []Sample {
	h.series.Range(func(_, v any) bool {
		entry := v.(*HistogramHandle)
		buckets := make([]BucketSample, len(h.opts.Buckets)+1)
		for i, b := range h.opts.Buckets {
			buckets[i] = BucketSample{
				UpperBound: b,
				Count:      entry.bucketCount[i].Load(),
			}
		}
		buckets[len(h.opts.Buckets)] = BucketSample{
			UpperBound: math.Inf(1),
			Count:      entry.bucketCount[len(h.opts.Buckets)].Load(),
		}
		dst = append(dst, Sample{
			Name:          h.opts.Name,
			Type:          MetricHistogram,
			Labels:        entry.labels,
			StreamingOnly: h.opts.StreamingOnly,
			Histogram: HistogramSnapshot{
				Sum:     math.Float64frombits(entry.sumBits.Load()),
				Count:   entry.count.Load(),
				Buckets: buckets,
			},
		})
		return true
	})
	return dst
}

func validateBuckets(buckets []float64) error {
	if len(buckets) == 0 {
		return fmt.Errorf("%w: histogram requires at least one bucket boundary", ErrInvalidMetric)
	}
	if !sort.Float64sAreSorted(buckets) {
		return fmt.Errorf("%w: histogram buckets must be ascending", ErrInvalidMetric)
	}
	return nil
}
