// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"errors"
	"strconv"
	"testing"
)

func TestCardinality_TombstoneSingleton(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	c, err := r.RegisterCounter(CounterOpts{
		Name:               "osvbng_test_cap",
		Help:               "h",
		Labels:             []string{"vrf"},
		MaxSeriesPerMetric: 10,
	})
	if err != nil {
		t.Fatalf("RegisterCounter: %v", err)
	}

	for i := 0; i < 10; i++ {
		c.WithLabelValues("vrf-" + strconv.Itoa(i))
	}
	if got := c.seriesCount.Load(); got != 10 {
		t.Fatalf("seriesCount=%d, want 10", got)
	}

	first := c.WithLabelValues("vrf-overflow-1")
	second := c.WithLabelValues("vrf-overflow-2")
	if first != second {
		t.Fatalf("tombstone must be singleton, got two different handles %p vs %p", first, second)
	}
	if !first.isTombstone {
		t.Fatalf("over-budget handle is not tombstone")
	}

	first.Inc()
	first.Add(9)
	second.Inc()

	if got := c.cardinalityDrops.Load(); got != 11 {
		t.Fatalf("cardinalityDrops=%d, want 11", got)
	}
	if r.SnapshotInternal().CardinalityDrops != 11 {
		t.Fatalf("internal counts mismatch: %d", r.SnapshotInternal().CardinalityDrops)
	}

	if got := c.seriesCount.Load(); got != 10 {
		t.Fatalf("seriesCount drifted to %d after tombstone emits", got)
	}
}

func TestCardinality_DefaultUnboundedListContainsBNGIdentifiers(t *testing.T) {
	t.Parallel()
	expectedSubset := []string{
		"session_id", "subscriber_id",
		"ip", "ipv4", "ipv6", "mac",
		"username", "hostname",
		"calling_station_id",
		"circuit_id", "remote_id", "agent_circuit_id", "agent_remote_id",
		"nas_port_id",
	}
	in := newLabelSet(defaultUnboundedLabels)
	for _, l := range expectedSubset {
		if _, ok := in[l]; !ok {
			t.Errorf("defaultUnboundedLabels missing %q", l)
		}
	}
}

func TestCardinality_SetUnboundedLabels_OverrideAndRestore(t *testing.T) {
	defer SetUnboundedLabels(defaultUnboundedLabels)

	SetUnboundedLabels([]string{"only_this"})
	r := NewRegistry()
	if _, err := r.RegisterCounter(CounterOpts{Name: "ok", Help: "h", Labels: []string{"session_id"}}); err != nil {
		t.Fatalf("after override session_id should be allowed: %v", err)
	}
	_, err := r.RegisterCounter(CounterOpts{Name: "blocked", Help: "h", Labels: []string{"only_this"}})
	if !errors.Is(err, ErrUnboundedLabel) {
		t.Fatalf("expected ErrUnboundedLabel, got %v", err)
	}
}

func TestCardinality_EmptyLabelNameRejected(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_, err := r.RegisterCounter(CounterOpts{Name: "x", Help: "h", Labels: []string{""}})
	if !errors.Is(err, ErrInvalidLabel) {
		t.Fatalf("expected ErrInvalidLabel, got %v", err)
	}
}
