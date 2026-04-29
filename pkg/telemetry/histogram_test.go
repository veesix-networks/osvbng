// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"errors"
	"math"
	"sync"
	"testing"
)

func TestHistogram_BasicObserve(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	h, err := r.RegisterHistogram(HistogramOpts{
		Name:    "osvbng_test_hist",
		Help:    "h",
		Labels:  []string{"vrf"},
		Buckets: []float64{1, 2.5, 5, 10},
	})
	if err != nil {
		t.Fatalf("RegisterHistogram: %v", err)
	}
	hh := h.WithLabelValues("default")
	for _, v := range []float64{0.5, 1.5, 3, 7, 15} {
		hh.Observe(v)
	}

	out := r.AppendSnapshot(nil, SnapshotOptions{PathGlob: "osvbng_test_hist"})
	if len(out) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(out))
	}
	hs := out[0].Histogram
	if hs.Count != 5 {
		t.Fatalf("count=%d, want 5", hs.Count)
	}
	if hs.Sum != 0.5+1.5+3+7+15 {
		t.Fatalf("sum=%v, want 27", hs.Sum)
	}

	wantCounts := []uint64{1, 1, 1, 1, 1}
	if len(hs.Buckets) != 5 {
		t.Fatalf("buckets len=%d, want 5", len(hs.Buckets))
	}
	for i, b := range hs.Buckets {
		if b.Count != wantCounts[i] {
			t.Errorf("bucket[%d] (le=%v): count=%d, want %d", i, b.UpperBound, b.Count, wantCounts[i])
		}
	}
	if !math.IsInf(hs.Buckets[len(hs.Buckets)-1].UpperBound, 1) {
		t.Errorf("last bucket should be +Inf, got %v", hs.Buckets[len(hs.Buckets)-1].UpperBound)
	}
}

func TestHistogram_DefaultBuckets(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	h, err := r.RegisterHistogram(HistogramOpts{Name: "osvbng_test_hist_default", Help: "h"})
	if err != nil {
		t.Fatalf("RegisterHistogram: %v", err)
	}
	if len(h.opts.Buckets) != len(DefaultHistogramBuckets) {
		t.Fatalf("default buckets not applied: got %v", h.opts.Buckets)
	}
}

func TestHistogram_ConcurrentObserve(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	h, _ := r.RegisterHistogram(HistogramOpts{
		Name:    "osvbng_test_hist_concurrent",
		Help:    "h",
		Buckets: []float64{1, 5, 10},
	})
	hh := h.WithLabelValues()

	const goroutines = 64
	const opsPerGoroutine = 1000
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				hh.Observe(2.0)
			}
		}()
	}
	wg.Wait()

	wantCount := uint64(goroutines * opsPerGoroutine)
	if got := hh.count.Load(); got != wantCount {
		t.Fatalf("count=%d, want %d", got, wantCount)
	}
	wantSum := float64(goroutines * opsPerGoroutine * 2)
	if got := math.Float64frombits(hh.sumBits.Load()); got != wantSum {
		t.Fatalf("sum=%v, want %v", got, wantSum)
	}
}

func TestHistogram_VariadicLookupOrDrop(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	h, _ := r.RegisterHistogram(HistogramOpts{
		Name:    "osvbng_test_hist_var",
		Help:    "h",
		Labels:  []string{"vrf"},
		Buckets: []float64{1, 5},
	})
	h.WithLabelValues("known")
	h.Observe(2.0, "known")
	h.Observe(3.0, "unknown")
	if h.unknownSeriesEmits.Load() != 1 {
		t.Fatalf("unknownSeriesEmits=%d, want 1", h.unknownSeriesEmits.Load())
	}
}

func TestHistogram_RejectInvalidBuckets(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_, err := r.RegisterHistogram(HistogramOpts{
		Name:    "x",
		Help:    "h",
		Buckets: []float64{5, 1, 10},
	})
	if !errors.Is(err, ErrInvalidMetric) {
		t.Fatalf("expected ErrInvalidMetric, got %v", err)
	}
}

func TestHistogram_SchemaMismatchOnDifferentBuckets(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_, err := r.RegisterHistogram(HistogramOpts{
		Name:    "osvbng_test_hist_schema",
		Help:    "h",
		Buckets: []float64{1, 5},
	})
	if err != nil {
		t.Fatalf("first register: %v", err)
	}
	_, err = r.RegisterHistogram(HistogramOpts{
		Name:    "osvbng_test_hist_schema",
		Help:    "h",
		Buckets: []float64{1, 10},
	})
	if !errors.Is(err, ErrSchemaMismatch) {
		t.Fatalf("expected ErrSchemaMismatch, got %v", err)
	}
}

func TestHistogram_StaleHandleEmits(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	h, _ := r.RegisterHistogram(HistogramOpts{
		Name:          "osvbng_test_hist_stale",
		Help:          "h",
		Labels:        []string{"session_id"},
		StreamingOnly: true,
		Buckets:       []float64{1, 5},
	})
	hh := h.WithLabelValues("s-1")
	hh.Observe(2.0)

	h.UnregisterSeries("s-1")
	hh.Observe(3.0)
	hh.Observe(4.0)

	if h.staleHandleEmits.Load() != 2 {
		t.Fatalf("staleHandleEmits=%d, want 2", h.staleHandleEmits.Load())
	}
}

func TestHistogram_TombstoneOnOverflow(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	h, _ := r.RegisterHistogram(HistogramOpts{
		Name:               "osvbng_test_hist_overflow",
		Help:               "h",
		Labels:             []string{"vrf"},
		Buckets:            []float64{1, 5},
		MaxSeriesPerMetric: 1,
	})
	h.WithLabelValues("a")
	tomb := h.WithLabelValues("b")
	if !tomb.isTombstone {
		t.Fatalf("expected tombstone")
	}
	tomb.Observe(2.5)
	if h.cardinalityDrops.Load() != 1 {
		t.Fatalf("cardinalityDrops=%d, want 1", h.cardinalityDrops.Load())
	}
}
