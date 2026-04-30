// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

// Package telemetry is the typed in-memory metric registry that backs
// osvbng's streaming telemetry exporters. Counter, gauge, and histogram
// primitives expose hot-path emit methods backed by sync/atomic. Plugin
// authors register typed metrics, resolve per-series handles, and emit
// without JSON or reflection.
package telemetry

import (
	"context"
	"time"
)

func RegisterCounter(opts CounterOpts) (*Counter, error) {
	return defaultRegistry.RegisterCounter(opts)
}

func RegisterGauge(opts GaugeOpts) (*Gauge, error) {
	return defaultRegistry.RegisterGauge(opts)
}

func RegisterHistogram(opts HistogramOpts) (*Histogram, error) {
	return defaultRegistry.RegisterHistogram(opts)
}

// MustRegisterCounter panics on registration error. Intended for component
// init paths where a failure means a programming bug (duplicate name with
// mismatched schema, unbounded label without StreamingOnly), not runtime
// input.
func MustRegisterCounter(opts CounterOpts) *Counter {
	return defaultRegistry.MustRegisterCounter(opts)
}

func MustRegisterGauge(opts GaugeOpts) *Gauge {
	return defaultRegistry.MustRegisterGauge(opts)
}

func MustRegisterHistogram(opts HistogramOpts) *Histogram {
	return defaultRegistry.MustRegisterHistogram(opts)
}

func AppendSnapshot(dst []Sample, opts SnapshotOptions) []Sample {
	return defaultRegistry.AppendSnapshot(dst, opts)
}

func Subscribe(opts SubscribeOptions) *Subscription {
	return defaultRegistry.Subscribe(opts)
}

func SetTickInterval(d time.Duration) {
	defaultRegistry.SetTickInterval(d)
}

func Shutdown(ctx context.Context) error {
	return defaultRegistry.Shutdown(ctx)
}
