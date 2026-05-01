// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type lifeItem struct {
	K string `metric:"label"`
	V uint64 `metric:"name=life.v,type=gauge,help=..."`
}

func TestLifecycle_DefaultUnregisterAbsentTuples(t *testing.T) {
	resetShowRegistrations()
	t.Cleanup(resetShowRegistrations)

	const path = "life.default"
	src := newFakeShowSource()
	src.set(path, []lifeItem{{K: "a", V: 1}, {K: "b", V: 2}, {K: "c", V: 3}})

	RegisterMetric[lifeItem](stringPath(path))
	pendingShowMu.Lock()
	m := pendingShows[path]
	pendingShowMu.Unlock()

	ctx := context.Background()
	pollOnce(ctx, src, path, m, nil)

	// All three series visible.
	got := defaultRegistry.AppendSnapshot(nil, SnapshotOptions{})
	for _, k := range []string{"a", "b", "c"} {
		if !sampleHasLabeled(got, "life.v", "k="+k, float64(1+indexOf(k, "abc"))) {
			t.Fatalf("expected life.v{k=%s}, got %v", k, samplesDump(onlyApp(got)))
		}
	}

	// Drop "b" from the snapshot.
	src.set(path, []lifeItem{{K: "a", V: 1}, {K: "c", V: 3}})
	pollOnce(ctx, src, path, m, nil)

	got = defaultRegistry.AppendSnapshot(nil, SnapshotOptions{})
	if sampleHasLabeled(got, "life.v", "k=b", 2) {
		t.Fatalf("expected life.v{k=b} to be unregistered, got %v", samplesDump(onlyApp(got)))
	}
	if !sampleHasLabeled(got, "life.v", "k=a", 1) || !sampleHasLabeled(got, "life.v", "k=c", 3) {
		t.Fatalf("expected a and c to remain, got %v", samplesDump(onlyApp(got)))
	}
}

func TestLifecycle_TransientErrorDoesNotMassUnregister(t *testing.T) {
	resetShowRegistrations()
	t.Cleanup(resetShowRegistrations)

	const path = "life.err"
	src := newFakeShowSource()
	src.set(path, []lifeItem{{K: "a", V: 1}, {K: "b", V: 2}})

	RegisterMetric[lifeItem](stringPath(path))
	pendingShowMu.Lock()
	m := pendingShows[path]
	pendingShowMu.Unlock()

	ctx := context.Background()
	pollOnce(ctx, src, path, m, nil)

	// Two error polls in a row.
	src.setErr(path, errors.New("transient"))
	pollOnce(ctx, src, path, m, nil)
	pollOnce(ctx, src, path, m, nil)

	// Recover.
	src.set(path, []lifeItem{{K: "a", V: 1}, {K: "b", V: 2}})
	pollOnce(ctx, src, path, m, nil)

	got := defaultRegistry.AppendSnapshot(nil, SnapshotOptions{})
	if !sampleHasLabeled(got, "life.v", "k=a", 1) || !sampleHasLabeled(got, "life.v", "k=b", 2) {
		t.Fatalf("expected both series to survive transient error, got %v", samplesDump(onlyApp(got)))
	}
}

type retainItem struct {
	K string `metric:"label"`
	V uint64 `metric:"name=life.retain.v,type=gauge,help=...,retain_stale"`
}

func TestLifecycle_RetainStale_KeepsAbsentTuples(t *testing.T) {
	resetShowRegistrations()
	t.Cleanup(resetShowRegistrations)

	const path = "life.retain"
	src := newFakeShowSource()
	src.set(path, []retainItem{{K: "a", V: 1}, {K: "b", V: 1}})

	RegisterMetric[retainItem](stringPath(path))
	pendingShowMu.Lock()
	m := pendingShows[path]
	pendingShowMu.Unlock()

	ctx := context.Background()
	pollOnce(ctx, src, path, m, nil)

	// Drop "b" from the snapshot. Default would unregister; retain_stale keeps it.
	src.set(path, []retainItem{{K: "a", V: 1}})
	pollOnce(ctx, src, path, m, nil)

	got := defaultRegistry.AppendSnapshot(nil, SnapshotOptions{})
	if !sampleHasLabeled(got, "life.retain.v", "k=b", 1) {
		t.Fatalf("expected retain_stale to keep life.retain.v{k=b}, got %v", samplesDump(onlyApp(got)))
	}
}

type isolA struct {
	K string `metric:"label"`
	V uint64 `metric:"name=life.iso.a,type=gauge,help=..."`
}
type isolB struct {
	K string `metric:"label"`
	V uint64 `metric:"name=life.iso.b,type=gauge,help=..."`
}
type isolParent struct {
	A []isolA `metric:"flatten"`
	B []isolB `metric:"flatten"`
}

func TestLifecycle_IsolationAcrossFlattenBranches(t *testing.T) {
	resetShowRegistrations()
	t.Cleanup(resetShowRegistrations)

	const path = "life.iso"
	src := newFakeShowSource()
	src.set(path, isolParent{
		A: []isolA{{K: "a1", V: 10}},
		B: []isolB{{K: "b1", V: 20}, {K: "b2", V: 30}},
	})

	RegisterMetric[isolParent](stringPath(path))
	pendingShowMu.Lock()
	m := pendingShows[path]
	pendingShowMu.Unlock()

	ctx := context.Background()
	pollOnce(ctx, src, path, m, nil)

	// Drop b2 only — a1 must remain unaffected.
	src.set(path, isolParent{
		A: []isolA{{K: "a1", V: 10}},
		B: []isolB{{K: "b1", V: 20}},
	})
	pollOnce(ctx, src, path, m, nil)

	got := defaultRegistry.AppendSnapshot(nil, SnapshotOptions{})
	if !sampleHasLabeled(got, "life.iso.a", "k=a1", 10) {
		t.Fatalf("expected life.iso.a{k=a1} preserved, got %v", samplesDump(onlyApp(got)))
	}
	if sampleHasLabeled(got, "life.iso.b", "k=b2", 30) {
		t.Fatalf("expected life.iso.b{k=b2} unregistered, got %v", samplesDump(onlyApp(got)))
	}
	if !sampleHasLabeled(got, "life.iso.b", "k=b1", 20) {
		t.Fatalf("expected life.iso.b{k=b1} preserved, got %v", samplesDump(onlyApp(got)))
	}
}

// indexOf is a tiny test helper for ordered-string lookups.
func indexOf(s, in string) int {
	for i := 0; i < len(in); i++ {
		if string(in[i]) == s {
			return i
		}
	}
	return -1
}

func init() {
	// Make sure reflect is referenced in tests file even if no test uses it
	// (silences linters when running individual tests).
	_ = reflect.TypeOf(0)
}
