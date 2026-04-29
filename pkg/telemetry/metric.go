// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

// MetricType discriminates between primitive metric types.
type MetricType uint8

const (
	MetricCounter MetricType = iota + 1
	MetricGauge
	MetricHistogram
)

func (t MetricType) String() string {
	switch t {
	case MetricCounter:
		return "counter"
	case MetricGauge:
		return "gauge"
	case MetricHistogram:
		return "histogram"
	default:
		return "unknown"
	}
}

// LabelPair is a snapshot view of one label name/value pair.
type LabelPair struct {
	Name  string
	Value string
}

// Sample is one snapshot record from a metric series.
type Sample struct {
	Name          string
	Help          string
	Type          MetricType
	Labels        []LabelPair
	StreamingOnly bool
	Value         float64
	Histogram     HistogramSnapshot
}

// HistogramSnapshot is the value-typed snapshot of a histogram series.
// The Buckets slice is allocated per histogram series per snapshot;
// histogram allocations are excluded from the no-allocation contract
// applied to counter and gauge snapshots.
type HistogramSnapshot struct {
	Sum     float64
	Count   uint64
	Buckets []BucketSample
}

type BucketSample struct {
	UpperBound float64
	Count      uint64
}

// SnapshotOptions controls AppendSnapshot filtering.
type SnapshotOptions struct {
	// PathGlob filters by metric name; empty or "*" matches all.
	PathGlob string

	// IncludeStreamingOnly controls whether metrics registered with
	// StreamingOnly=true appear in the snapshot. Default false makes
	// the snapshot Prometheus-safe.
	IncludeStreamingOnly bool
}

// metric is the registry-internal interface every metric type implements.
type metric interface {
	name() string
	help() string
	labelNames() []string
	metricType() MetricType
	streamingOnly() bool
	appendSamples(dst []Sample) []Sample
	swapDirty() bool

	// Internal observability accessors. Each metric type maintains
	// per-metric atomic counters that the registry surfaces as
	// osvbng_telemetry_* metrics during snapshot.
	cardinalityDropsLoad() uint64
	unknownSeriesEmitsLoad() uint64
	staleHandleEmitsLoad() uint64
	seriesCountLoad() int64
	internalLabelsRef() []LabelPair
}
