// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import "testing"

func TestUnregisterSeries_Basic(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	c, _ := r.RegisterCounter(CounterOpts{
		Name:          "osvbng_test_unregister",
		Help:          "h",
		Labels:        []string{"session_id"},
		StreamingOnly: true,
	})

	c.WithLabelValues("s-1").Inc()
	c.WithLabelValues("s-2").Inc()

	if c.seriesCount.Load() != 2 {
		t.Fatalf("got %d series, want 2", c.seriesCount.Load())
	}

	if !c.UnregisterSeries("s-1") {
		t.Fatalf("UnregisterSeries returned false for known tuple")
	}
	if c.seriesCount.Load() != 1 {
		t.Fatalf("seriesCount=%d, want 1 after unregister", c.seriesCount.Load())
	}

	out := r.AppendSnapshot(nil, SnapshotOptions{IncludeStreamingOnly: true})
	for _, s := range out {
		if s.Name == "osvbng_test_unregister" && len(s.Labels) > 0 && s.Labels[0].Value == "s-1" {
			t.Errorf("snapshot still contains s-1 after unregister")
		}
	}

	if c.UnregisterSeries("s-never-existed") {
		t.Errorf("UnregisterSeries returned true for unknown tuple")
	}
}

func TestUnregisterSeries_StaleHandleEmits(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	c, _ := r.RegisterCounter(CounterOpts{
		Name:          "osvbng_test_stale",
		Help:          "h",
		Labels:        []string{"session_id"},
		StreamingOnly: true,
	})

	h := c.WithLabelValues("s-1")
	h.Inc()
	h.Add(4)

	c.UnregisterSeries("s-1")

	h.Inc()
	h.Add(7)

	if c.staleHandleEmits.Load() != 8 {
		t.Fatalf("staleHandleEmits=%d, want 8", c.staleHandleEmits.Load())
	}
	if r.SnapshotInternal().StaleHandleEmits != 8 {
		t.Fatalf("internal stale-handle counts mismatch: %d", r.SnapshotInternal().StaleHandleEmits)
	}
}

func TestUnregisterSeries_ReResolveCreatesFreshSeries(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	c, _ := r.RegisterCounter(CounterOpts{
		Name:          "osvbng_test_re_resolve",
		Help:          "h",
		Labels:        []string{"session_id"},
		StreamingOnly: true,
	})

	h1 := c.WithLabelValues("s-1")
	h1.Add(42)

	if !c.UnregisterSeries("s-1") {
		t.Fatalf("UnregisterSeries failed")
	}

	h2 := c.WithLabelValues("s-1")
	if h1 == h2 {
		t.Fatalf("re-resolve returned the stale handle")
	}
	if h2.value.Load() != 0 {
		t.Fatalf("re-resolved handle inherited value: got %d, want 0", h2.value.Load())
	}
}
