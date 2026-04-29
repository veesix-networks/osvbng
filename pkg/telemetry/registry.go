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
		switch m := v.(type) {
		case *Counter:
			total += m.seriesCount.Load()
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
