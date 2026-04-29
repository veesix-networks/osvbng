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

// RegisterCounter registers a counter on the package-level default registry.
func RegisterCounter(opts CounterOpts) (*Counter, error) {
	return defaultRegistry.RegisterCounter(opts)
}

// RegisterGauge registers a gauge on the package-level default registry.
func RegisterGauge(opts GaugeOpts) (*Gauge, error) {
	return defaultRegistry.RegisterGauge(opts)
}

// RegisterHistogram registers a histogram on the package-level default
// registry.
func RegisterHistogram(opts HistogramOpts) (*Histogram, error) {
	return defaultRegistry.RegisterHistogram(opts)
}

// AppendSnapshot reads the package-level default registry into dst.
func AppendSnapshot(dst []Sample, opts SnapshotOptions) []Sample {
	return defaultRegistry.AppendSnapshot(dst, opts)
}

// Subscribe registers a subscriber on the package-level default registry.
func Subscribe(opts SubscribeOptions) *Subscription {
	return defaultRegistry.Subscribe(opts)
}

// SetTickInterval overrides the tick cadence on the package-level default
// registry. Must be called before any Subscribe to take effect.
func SetTickInterval(d time.Duration) {
	defaultRegistry.SetTickInterval(d)
}

// Shutdown releases resources held by the package-level default registry.
func Shutdown(ctx context.Context) error {
	return defaultRegistry.Shutdown(ctx)
}
