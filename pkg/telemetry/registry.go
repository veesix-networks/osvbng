// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// Registry holds registered metrics and supports snapshot and subscribe.
// In production code, use the package-level functions which delegate to a
// shared default registry. Tests should construct an isolated Registry via
// NewRegistry() to avoid cross-test state leakage under t.Parallel().
type Registry struct {
	metrics sync.Map

	subscriberCount atomic.Int64

	registrationErrors atomic.Uint64
}

// NewRegistry constructs an isolated registry. Use this in tests; production
// code should use the package-level convenience functions backed by the
// default registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// RegisterCounter registers a counter metric. Re-registering with the same
// name and matching schema returns the existing metric.
func (r *Registry) RegisterCounter(opts CounterOpts) (*Counter, error) {
	if opts.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidMetric)
	}
	if err := validateLabels(opts.Labels, opts.StreamingOnly); err != nil {
		r.registrationErrors.Add(1)
		return nil, err
	}
	if opts.MaxSeriesPerMetric == 0 {
		opts.MaxSeriesPerMetric = DefaultMaxSeriesPerMetric
	}

	if existing, ok := r.metrics.Load(opts.Name); ok {
		m := existing.(metric)
		if m.metricType() != MetricCounter {
			r.registrationErrors.Add(1)
			return nil, fmt.Errorf("%w: %q is %s", ErrTypeMismatch, opts.Name, m.metricType())
		}
		c := existing.(*Counter)
		if !labelNamesEqual(c.opts.Labels, opts.Labels) {
			r.registrationErrors.Add(1)
			return nil, fmt.Errorf("%w: %q", ErrSchemaMismatch, opts.Name)
		}
		return c, nil
	}

	c := &Counter{
		opts:           opts,
		registry:       r,
		internalLabels: []LabelPair{{Name: internalLabelMetric, Value: opts.Name}},
	}
	actual, loaded := r.metrics.LoadOrStore(opts.Name, c)
	if loaded {
		existing := actual.(*Counter)
		if !labelNamesEqual(existing.opts.Labels, opts.Labels) {
			r.registrationErrors.Add(1)
			return nil, fmt.Errorf("%w: %q", ErrSchemaMismatch, opts.Name)
		}
		return existing, nil
	}
	return c, nil
}

// RegisterGauge registers a gauge metric. Re-registering with the same
// name and matching schema returns the existing metric.
func (r *Registry) RegisterGauge(opts GaugeOpts) (*Gauge, error) {
	if opts.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidMetric)
	}
	if err := validateLabels(opts.Labels, opts.StreamingOnly); err != nil {
		r.registrationErrors.Add(1)
		return nil, err
	}
	if opts.MaxSeriesPerMetric == 0 {
		opts.MaxSeriesPerMetric = DefaultMaxSeriesPerMetric
	}

	if existing, ok := r.metrics.Load(opts.Name); ok {
		m := existing.(metric)
		if m.metricType() != MetricGauge {
			r.registrationErrors.Add(1)
			return nil, fmt.Errorf("%w: %q is %s", ErrTypeMismatch, opts.Name, m.metricType())
		}
		g := existing.(*Gauge)
		if !labelNamesEqual(g.opts.Labels, opts.Labels) {
			r.registrationErrors.Add(1)
			return nil, fmt.Errorf("%w: %q", ErrSchemaMismatch, opts.Name)
		}
		return g, nil
	}

	g := &Gauge{
		opts:           opts,
		registry:       r,
		internalLabels: []LabelPair{{Name: internalLabelMetric, Value: opts.Name}},
	}
	actual, loaded := r.metrics.LoadOrStore(opts.Name, g)
	if loaded {
		existing := actual.(*Gauge)
		if !labelNamesEqual(existing.opts.Labels, opts.Labels) {
			r.registrationErrors.Add(1)
			return nil, fmt.Errorf("%w: %q", ErrSchemaMismatch, opts.Name)
		}
		return existing, nil
	}
	return g, nil
}

// RegisterHistogram registers a histogram metric. Re-registering with the
// same name, matching schema, and matching bucket boundaries returns the
// existing metric.
func (r *Registry) RegisterHistogram(opts HistogramOpts) (*Histogram, error) {
	if opts.Name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidMetric)
	}
	if err := validateLabels(opts.Labels, opts.StreamingOnly); err != nil {
		r.registrationErrors.Add(1)
		return nil, err
	}
	if len(opts.Buckets) == 0 {
		opts.Buckets = DefaultHistogramBuckets
	}
	if err := validateBuckets(opts.Buckets); err != nil {
		r.registrationErrors.Add(1)
		return nil, err
	}
	if opts.MaxSeriesPerMetric == 0 {
		opts.MaxSeriesPerMetric = DefaultMaxSeriesPerMetric
	}

	if existing, ok := r.metrics.Load(opts.Name); ok {
		m := existing.(metric)
		if m.metricType() != MetricHistogram {
			r.registrationErrors.Add(1)
			return nil, fmt.Errorf("%w: %q is %s", ErrTypeMismatch, opts.Name, m.metricType())
		}
		h := existing.(*Histogram)
		if !labelNamesEqual(h.opts.Labels, opts.Labels) || !bucketsEqual(h.opts.Buckets, opts.Buckets) {
			r.registrationErrors.Add(1)
			return nil, fmt.Errorf("%w: %q", ErrSchemaMismatch, opts.Name)
		}
		return h, nil
	}

	h := &Histogram{
		opts:           opts,
		registry:       r,
		internalLabels: []LabelPair{{Name: internalLabelMetric, Value: opts.Name}},
	}
	actual, loaded := r.metrics.LoadOrStore(opts.Name, h)
	if loaded {
		existing := actual.(*Histogram)
		if !labelNamesEqual(existing.opts.Labels, opts.Labels) || !bucketsEqual(existing.opts.Buckets, opts.Buckets) {
			r.registrationErrors.Add(1)
			return nil, fmt.Errorf("%w: %q", ErrSchemaMismatch, opts.Name)
		}
		return existing, nil
	}
	return h, nil
}

func bucketsEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func labelNamesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// MetricCount returns the number of registered metrics. Used by the internal
// observability metric and by tests.
func (r *Registry) MetricCount() int {
	count := 0
	r.metrics.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// SeriesCount returns the total series count across all registered metrics.
func (r *Registry) SeriesCount() int64 {
	var total int64
	r.metrics.Range(func(_, v any) bool {
		if m, ok := v.(metric); ok {
			total += m.seriesCountLoad()
		}
		return true
	})
	return total
}

// Shutdown releases registry-internal resources. Safe to call multiple times.
// Subscribe support added in a later phase will use this for tick-goroutine
// teardown.
func (r *Registry) Shutdown(_ context.Context) error {
	return nil
}

var defaultRegistry = NewRegistry()

// Default returns the package-level default registry.
func Default() *Registry {
	return defaultRegistry
}
