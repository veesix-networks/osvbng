// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"errors"
	"sync"
	"testing"
)

func TestRegisterCounter_Basic(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	c, err := r.RegisterCounter(CounterOpts{
		Name:   "osvbng_test_counter",
		Help:   "test",
		Labels: []string{"vrf"},
	})
	if err != nil {
		t.Fatalf("RegisterCounter: %v", err)
	}
	h := c.WithLabelValues("default")
	h.Inc()
	h.Add(4)
	if got := h.value.Load(); got != 5 {
		t.Fatalf("counter value = %d, want 5", got)
	}
}

func TestRegisterCounter_NoLabels(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	c, err := r.RegisterCounter(CounterOpts{Name: "osvbng_test_nolabels", Help: "h"})
	if err != nil {
		t.Fatalf("RegisterCounter: %v", err)
	}
	h := c.WithLabelValues()
	h.Inc()
	h.Inc()
	if got := h.value.Load(); got != 2 {
		t.Fatalf("got %d, want 2", got)
	}
}

func TestRegisterCounter_RejectUnboundedLabel(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	for _, label := range defaultUnboundedLabels {
		_, err := r.RegisterCounter(CounterOpts{
			Name:   "osvbng_test_rej_" + label,
			Help:   "h",
			Labels: []string{label},
		})
		if !errors.Is(err, ErrUnboundedLabel) {
			t.Errorf("label %q: got err=%v, want ErrUnboundedLabel", label, err)
		}
	}
}

func TestRegisterCounter_StreamingOnlyAllowsUnboundedLabel(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	c, err := r.RegisterCounter(CounterOpts{
		Name:          "osvbng_test_streaming",
		Help:          "h",
		Labels:        []string{"session_id"},
		StreamingOnly: true,
	})
	if err != nil {
		t.Fatalf("StreamingOnly registration failed: %v", err)
	}
	if !c.streamingOnly() {
		t.Fatalf("streamingOnly() = false")
	}
}

func TestRegisterCounter_TypeMismatch(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_, err := r.RegisterCounter(CounterOpts{Name: "osvbng_test_type", Help: "h"})
	if err != nil {
		t.Fatalf("first register: %v", err)
	}
	r.metrics.Store("osvbng_test_type", &fakeMetric{n: "osvbng_test_type", t: MetricGauge})
	_, err = r.RegisterCounter(CounterOpts{Name: "osvbng_test_type", Help: "h"})
	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("got err=%v, want ErrTypeMismatch", err)
	}
}

func TestRegisterCounter_SchemaMismatch(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_, err := r.RegisterCounter(CounterOpts{
		Name:   "osvbng_test_schema",
		Help:   "h",
		Labels: []string{"vrf"},
	})
	if err != nil {
		t.Fatalf("first register: %v", err)
	}
	_, err = r.RegisterCounter(CounterOpts{
		Name:   "osvbng_test_schema",
		Help:   "h",
		Labels: []string{"srg"},
	})
	if !errors.Is(err, ErrSchemaMismatch) {
		t.Fatalf("got err=%v, want ErrSchemaMismatch", err)
	}
}

func TestRegisterCounter_ReturnsExistingOnSameSchema(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	c1, err := r.RegisterCounter(CounterOpts{Name: "osvbng_test_dup", Help: "h", Labels: []string{"vrf"}})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	c2, err := r.RegisterCounter(CounterOpts{Name: "osvbng_test_dup", Help: "h", Labels: []string{"vrf"}})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if c1 != c2 {
		t.Fatalf("expected same counter pointer, got %p vs %p", c1, c2)
	}
}

func TestCounter_ConcurrentInc(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	c, _ := r.RegisterCounter(CounterOpts{Name: "osvbng_test_concurrent", Help: "h", Labels: []string{"vrf"}})
	h := c.WithLabelValues("default")

	const goroutines = 100
	const incsPerGoroutine = 10000

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < incsPerGoroutine; j++ {
				h.Inc()
			}
		}()
	}
	wg.Wait()

	want := uint64(goroutines * incsPerGoroutine)
	if got := h.value.Load(); got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
}

func TestCounter_VariadicEmit_KnownTuple(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	c, _ := r.RegisterCounter(CounterOpts{Name: "osvbng_test_var_known", Help: "h", Labels: []string{"vrf"}})
	c.WithLabelValues("vrf-A")
	c.Inc("vrf-A")
	c.Add(5, "vrf-A")

	got := c.WithLabelValues("vrf-A").value.Load()
	if got != 6 {
		t.Fatalf("got %d, want 6", got)
	}
	if r.SnapshotInternal().UnknownSeriesEmits != 0 {
		t.Fatalf("unknown emits should be 0; got %d", r.SnapshotInternal().UnknownSeriesEmits)
	}
}

func TestCounter_VariadicEmit_UnknownTuple_Drops(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	c, _ := r.RegisterCounter(CounterOpts{Name: "osvbng_test_var_unknown", Help: "h", Labels: []string{"vrf"}})

	c.Inc("never_resolved")
	c.Add(7, "also_never_resolved")

	if c.seriesCount.Load() != 0 {
		t.Fatalf("variadic emit must not create series, got count=%d", c.seriesCount.Load())
	}
	if r.SnapshotInternal().UnknownSeriesEmits != 8 {
		t.Fatalf("unknown emits = %d, want 8", r.SnapshotInternal().UnknownSeriesEmits)
	}
}

func TestCounter_LabelCountMismatch_Panics(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	c, _ := r.RegisterCounter(CounterOpts{Name: "osvbng_test_panic", Help: "h", Labels: []string{"a", "b"}})

	defer func() {
		r := recover()
		if !errors.Is(r.(error), ErrLabelCount) {
			t.Fatalf("expected ErrLabelCount panic, got %v", r)
		}
	}()
	c.WithLabelValues("only-one")
}

// fakeMetric is a stand-in for the Type-mismatch test; it implements the
// internal metric interface but reports a different MetricType.
type fakeMetric struct {
	n string
	t MetricType
}

func (f *fakeMetric) name() string                        { return f.n }
func (f *fakeMetric) help() string                        { return "" }
func (f *fakeMetric) labelNames() []string                { return nil }
func (f *fakeMetric) metricType() MetricType              { return f.t }
func (f *fakeMetric) streamingOnly() bool                 { return false }
func (f *fakeMetric) appendSamples(dst []Sample) []Sample { return dst }
func (f *fakeMetric) swapDirty() bool                     { return false }
