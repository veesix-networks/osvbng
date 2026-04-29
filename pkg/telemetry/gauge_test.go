// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"errors"
	"sync"
	"testing"
)

func TestGauge_SetGetValue(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	g, err := r.RegisterGauge(GaugeOpts{Name: "osvbng_test_gauge", Help: "h", Labels: []string{"vrf"}})
	if err != nil {
		t.Fatalf("RegisterGauge: %v", err)
	}
	h := g.WithLabelValues("default")
	h.Set(3.5)
	if got := h.Value(); got != 3.5 {
		t.Fatalf("got %v, want 3.5", got)
	}
	h.Set(7)
	if got := h.Value(); got != 7 {
		t.Fatalf("got %v, want 7", got)
	}
}

func TestGauge_AddSubIncDec(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	g, _ := r.RegisterGauge(GaugeOpts{Name: "osvbng_test_gauge_arith", Help: "h"})
	h := g.WithLabelValues()

	h.Inc()
	h.Inc()
	h.Add(0.5)
	h.Sub(0.25)
	h.Dec()

	if got := h.Value(); got != 1.25 {
		t.Fatalf("got %v, want 1.25", got)
	}
}

func TestGauge_ConcurrentAdd(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	g, _ := r.RegisterGauge(GaugeOpts{Name: "osvbng_test_gauge_concurrent", Help: "h"})
	h := g.WithLabelValues()

	const goroutines = 64
	const opsPerGoroutine = 1000

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				h.Add(1)
			}
		}()
	}
	wg.Wait()

	want := float64(goroutines * opsPerGoroutine)
	if got := h.Value(); got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestGauge_VariadicLookupOrDrop(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	g, _ := r.RegisterGauge(GaugeOpts{Name: "osvbng_test_gauge_var", Help: "h", Labels: []string{"vrf"}})
	g.WithLabelValues("known")
	g.Set(42, "known")
	g.Add(3, "known")

	g.Set(5, "unknown")
	g.Add(1, "also-unknown")

	if got := g.WithLabelValues("known").Value(); got != 45 {
		t.Fatalf("got %v, want 45", got)
	}
	if g.unknownSeriesEmits.Load() != 2 {
		t.Fatalf("unknownSeriesEmits=%d, want 2", g.unknownSeriesEmits.Load())
	}
}

func TestGauge_TombstoneOnOverflow(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	g, _ := r.RegisterGauge(GaugeOpts{
		Name:               "osvbng_test_gauge_overflow",
		Help:               "h",
		Labels:             []string{"vrf"},
		MaxSeriesPerMetric: 2,
	})
	g.WithLabelValues("a").Set(1)
	g.WithLabelValues("b").Set(2)

	tomb1 := g.WithLabelValues("c")
	tomb2 := g.WithLabelValues("d")
	if tomb1 != tomb2 {
		t.Fatalf("tombstone must be singleton")
	}
	if !tomb1.isTombstone {
		t.Fatalf("expected tombstone")
	}
	tomb1.Set(99)
	tomb1.Add(1)
	if g.cardinalityDrops.Load() != 2 {
		t.Fatalf("cardinalityDrops=%d, want 2", g.cardinalityDrops.Load())
	}
}

func TestGauge_StaleHandleEmits(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	g, _ := r.RegisterGauge(GaugeOpts{
		Name:          "osvbng_test_gauge_stale",
		Help:          "h",
		Labels:        []string{"session_id"},
		StreamingOnly: true,
	})
	h := g.WithLabelValues("s-1")
	h.Set(10)

	g.UnregisterSeries("s-1")

	h.Set(99)
	h.Add(1)

	if g.staleHandleEmits.Load() != 2 {
		t.Fatalf("staleHandleEmits=%d, want 2", g.staleHandleEmits.Load())
	}
}

func TestGauge_AppendSnapshot(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	g, _ := r.RegisterGauge(GaugeOpts{Name: "osvbng_test_gauge_snap", Help: "h", Labels: []string{"vrf"}})
	g.WithLabelValues("a").Set(1.5)
	g.WithLabelValues("b").Set(2.5)

	out := r.AppendSnapshot(nil, SnapshotOptions{PathGlob: "osvbng_test_gauge_snap"})
	values := map[string]float64{}
	for _, s := range out {
		if s.Name != "osvbng_test_gauge_snap" {
			continue
		}
		if s.Type != MetricGauge {
			t.Errorf("expected MetricGauge, got %v", s.Type)
		}
		values[s.Labels[0].Value] = s.Value
	}
	if values["a"] != 1.5 || values["b"] != 2.5 {
		t.Fatalf("snapshot values wrong: %v", values)
	}
}

func TestGauge_RejectUnboundedLabel(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_, err := r.RegisterGauge(GaugeOpts{Name: "x", Help: "h", Labels: []string{"session_id"}})
	if !errors.Is(err, ErrUnboundedLabel) {
		t.Fatalf("got err=%v, want ErrUnboundedLabel", err)
	}
}

func TestGauge_TypeMismatch(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_, err := r.RegisterCounter(CounterOpts{Name: "osvbng_test_dual"})
	if err != nil {
		t.Fatalf("counter register: %v", err)
	}
	_, err = r.RegisterGauge(GaugeOpts{Name: "osvbng_test_dual"})
	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("got err=%v, want ErrTypeMismatch", err)
	}
}
