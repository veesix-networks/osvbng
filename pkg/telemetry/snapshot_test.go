// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"testing"
)

func TestAppendSnapshot_BasicCounters(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	c, _ := r.RegisterCounter(CounterOpts{Name: "osvbng_test_snap_a", Help: "h", Labels: []string{"vrf"}})
	c.WithLabelValues("a").Add(3)
	c.WithLabelValues("b").Add(7)

	d, _ := r.RegisterCounter(CounterOpts{Name: "osvbng_test_snap_b", Help: "h", Labels: []string{"vrf"}})
	d.WithLabelValues("c").Add(11)

	got := r.AppendSnapshot(nil, SnapshotOptions{})
	appCounters := filterByPrefix(got, "osvbng_test_snap_")
	if len(appCounters) != 3 {
		t.Fatalf("expected 3 application samples, got %d", len(appCounters))
	}

	values := map[string]uint64{}
	for _, s := range appCounters {
		key := s.Name + ":" + s.Labels[0].Value
		values[key] = uint64(s.Value)
	}
	if values["osvbng_test_snap_a:a"] != 3 || values["osvbng_test_snap_a:b"] != 7 || values["osvbng_test_snap_b:c"] != 11 {
		t.Fatalf("snapshot values wrong: %v", values)
	}
}

func TestAppendSnapshot_GlobFilter(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	c, _ := r.RegisterCounter(CounterOpts{Name: "osvbng_aaa_test", Help: "h"})
	c.WithLabelValues().Inc()
	d, _ := r.RegisterCounter(CounterOpts{Name: "osvbng_dhcp_test", Help: "h"})
	d.WithLabelValues().Inc()

	out := r.AppendSnapshot(nil, SnapshotOptions{PathGlob: "osvbng_aaa_*"})
	app := filterByPrefix(out, "osvbng_aaa_")
	if len(app) != 1 {
		t.Fatalf("expected 1 aaa sample, got %d", len(app))
	}
	for _, s := range app {
		if s.Name != "osvbng_aaa_test" {
			t.Errorf("unexpected sample %q", s.Name)
		}
	}
}

func TestAppendSnapshot_StreamingOnlyExcludedByDefault(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	regular, _ := r.RegisterCounter(CounterOpts{Name: "osvbng_regular_total", Help: "h", Labels: []string{"vrf"}})
	regular.WithLabelValues("a").Inc()
	streaming, _ := r.RegisterCounter(CounterOpts{
		Name:          "osvbng_streaming_total",
		Help:          "h",
		Labels:        []string{"session_id"},
		StreamingOnly: true,
	})
	streaming.WithLabelValues("session-1").Inc()

	defaultSnap := r.AppendSnapshot(nil, SnapshotOptions{})
	for _, s := range defaultSnap {
		if s.Name == "osvbng_streaming_total" {
			t.Fatalf("default snapshot leaked streaming_only metric")
		}
	}

	allSnap := r.AppendSnapshot(nil, SnapshotOptions{IncludeStreamingOnly: true})
	found := false
	for _, s := range allSnap {
		if s.Name == "osvbng_streaming_total" {
			found = true
			if !s.StreamingOnly {
				t.Errorf("Sample.StreamingOnly not set")
			}
		}
	}
	if !found {
		t.Fatalf("streaming_only metric missing when IncludeStreamingOnly=true")
	}
}

func TestAppendSnapshot_ZeroAllocSteadyState(t *testing.T) {
	r := NewRegistry()
	c, _ := r.RegisterCounter(CounterOpts{Name: "osvbng_zero_alloc", Help: "h", Labels: []string{"vrf"}})
	for _, v := range []string{"a", "b", "c", "d", "e"} {
		c.WithLabelValues(v).Inc()
	}

	dst := make([]Sample, 0, 32)
	dst = r.AppendSnapshot(dst, SnapshotOptions{})
	dst = dst[:0]

	allocs := testing.AllocsPerRun(100, func() {
		dst = r.AppendSnapshot(dst, SnapshotOptions{})
		dst = dst[:0]
	})
	if allocs > 0 {
		t.Fatalf("AppendSnapshot allocated %.2f times per run, want 0", allocs)
	}
}

func TestAppendSnapshot_InternalMetricsPresent(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	c, _ := r.RegisterCounter(CounterOpts{
		Name:               "osvbng_test_internal_present",
		Help:               "h",
		Labels:             []string{"vrf"},
		MaxSeriesPerMetric: 1,
	})
	c.WithLabelValues("ok")
	c.WithLabelValues("over").Inc()

	out := r.AppendSnapshot(nil, SnapshotOptions{})
	var sawDrop, sawTotal bool
	for _, s := range out {
		switch s.Name {
		case internalMetricCardinalityDrops:
			sawDrop = true
		case internalMetricMetricsTotal:
			sawTotal = true
		}
	}
	if !sawDrop {
		t.Errorf("expected %q in snapshot", internalMetricCardinalityDrops)
	}
	if !sawTotal {
		t.Errorf("expected %q in snapshot", internalMetricMetricsTotal)
	}
}

func filterByPrefix(in []Sample, prefix string) []Sample {
	out := in[:0:0]
	for _, s := range in {
		if len(s.Name) >= len(prefix) && s.Name[:len(prefix)] == prefix {
			out = append(out, s)
		}
	}
	return out
}
