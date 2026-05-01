// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeShowSource implements ShowSource for tests.
type fakeShowSource struct {
	mu        sync.Mutex
	responses map[string]any
	errs      map[string]error
	calls     atomic.Int32
}

func newFakeShowSource() *fakeShowSource {
	return &fakeShowSource{
		responses: make(map[string]any),
		errs:      make(map[string]error),
	}
}

func (f *fakeShowSource) set(path string, v any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responses[path] = v
	delete(f.errs, path)
}

func (f *fakeShowSource) setErr(path string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errs[path] = err
	delete(f.responses, path)
}

func (f *fakeShowSource) Snapshot(_ context.Context, path string) (any, error) {
	f.calls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.errs[path]; ok {
		return nil, err
	}
	return f.responses[path], nil
}

type stringPath string

func (p stringPath) String() string { return string(p) }

// recordingLogger captures Warn calls for assertion.
type recordingLogger struct {
	mu    sync.Mutex
	warns []string
}

func (r *recordingLogger) Debug(string, ...any) {}
func (r *recordingLogger) Warn(msg string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := msg
	for i := 0; i+1 < len(args); i += 2 {
		out += " " + toStr(args[i]) + "=" + toStr(args[i+1])
	}
	r.warns = append(r.warns, out)
}

func (r *recordingLogger) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.warns))
	copy(out, r.warns)
	return out
}

func toStr(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case error:
		return x.Error()
	}
	return ""
}

// resetShowRegistrations clears the package-level pendingShows so tests
// stay independent. Call from t.Cleanup or at the start of a test.
func resetShowRegistrations() {
	pendingShowMu.Lock()
	defer pendingShowMu.Unlock()
	pendingShows = make(map[string]*pendingShowMetric)
}

type registerMetricFlat struct {
	V uint64 `metric:"name=rm.flat.v,type=gauge,help=..."`
}

func TestRegisterMetric_FlatStruct(t *testing.T) {
	resetShowRegistrations()
	t.Cleanup(resetShowRegistrations)

	const path = "rm.flat"
	src := newFakeShowSource()
	src.set(path, registerMetricFlat{V: 42})

	RegisterMetric[registerMetricFlat](stringPath(path))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartShowPollers(ctx, src, nil)

	waitFor(t, func() bool { return src.calls.Load() >= 1 })
	got := defaultRegistry.AppendSnapshot(nil, SnapshotOptions{})
	if !sampleHas(got, "rm.flat.v", 42) {
		t.Fatalf("expected rm.flat.v=42, got %v", samplesDump(onlyApp(got)))
	}
}

type registerMetricMulti struct {
	K string `metric:"label"`
	V uint64 `metric:"name=rm.multi.v,type=gauge,help=..."`
}

func TestRegisterMetric_MultiSlice(t *testing.T) {
	resetShowRegistrations()
	t.Cleanup(resetShowRegistrations)

	const path = "rm.multi"
	src := newFakeShowSource()
	src.set(path, []registerMetricMulti{{K: "a", V: 1}, {K: "b", V: 2}})

	RegisterMetric[registerMetricMulti](stringPath(path))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartShowPollers(ctx, src, nil)

	waitFor(t, func() bool { return src.calls.Load() >= 1 })
	got := defaultRegistry.AppendSnapshot(nil, SnapshotOptions{})
	if !sampleHasLabeled(got, "rm.multi.v", "k=a", 1) ||
		!sampleHasLabeled(got, "rm.multi.v", "k=b", 2) {
		t.Fatalf("expected rm.multi.v series, got %v", samplesDump(onlyApp(got)))
	}
}

type registerMetricMap struct {
	A string `json:"area" metric:"label,map_key"`
	V uint64 `metric:"name=rm.mp.v,type=gauge,help=..."`
}

func TestRegisterMetric_Map(t *testing.T) {
	resetShowRegistrations()
	t.Cleanup(resetShowRegistrations)

	const path = "rm.mp"
	src := newFakeShowSource()
	src.set(path, map[string]registerMetricMap{
		"a": {V: 10},
		"b": {V: 20},
	})

	RegisterMetric[registerMetricMap](stringPath(path))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartShowPollers(ctx, src, nil)

	waitFor(t, func() bool { return src.calls.Load() >= 1 })
	got := defaultRegistry.AppendSnapshot(nil, SnapshotOptions{})
	if !sampleHasLabeled(got, "rm.mp.v", "area=a", 10) ||
		!sampleHasLabeled(got, "rm.mp.v", "area=b", 20) {
		t.Fatalf("expected rm.mp.v map series, got %v", samplesDump(onlyApp(got)))
	}
}

type registerMetricIdem struct {
	V uint64 `metric:"name=rm.idem.v,type=gauge,help=..."`
}

func TestRegisterMetric_IdempotentSameType(t *testing.T) {
	resetShowRegistrations()
	t.Cleanup(resetShowRegistrations)
	RegisterMetric[registerMetricIdem](stringPath("rm.idem"))
	RegisterMetric[registerMetricIdem](stringPath("rm.idem"))
}

type registerMetricA struct {
	V uint64 `metric:"name=rm.a.v,type=gauge,help=..."`
}
type registerMetricB struct {
	V uint64 `metric:"name=rm.b.v,type=gauge,help=..."`
}

func TestRegisterMetric_DifferentTypeSamePath_Panics(t *testing.T) {
	resetShowRegistrations()
	t.Cleanup(resetShowRegistrations)

	RegisterMetric[registerMetricA](stringPath("rm.collide"))
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on different-T re-registration")
		}
		err, _ := r.(error)
		if err == nil || !errors.Is(err, ErrTypeMismatch) {
			t.Fatalf("expected ErrTypeMismatch, got %v", r)
		}
	}()
	RegisterMetric[registerMetricB](stringPath("rm.collide"))
}

type decodedItem struct {
	K string `metric:"label"`
	V uint64 `metric:"name=rm.dec.v,type=gauge,help=..."`
}

func TestRegisterMetric_WithDecoder_Happy(t *testing.T) {
	resetShowRegistrations()
	t.Cleanup(resetShowRegistrations)

	const path = "rm.dec"
	src := newFakeShowSource()
	type rawWrapper struct{ payload string }
	src.set(path, rawWrapper{payload: "ignored"})

	RegisterMetric[decodedItem](stringPath(path),
		WithDecoder(func(any) (any, error) {
			return []decodedItem{{K: "x", V: 7}}, nil
		}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartShowPollers(ctx, src, nil)

	waitFor(t, func() bool { return src.calls.Load() >= 1 })
	got := defaultRegistry.AppendSnapshot(nil, SnapshotOptions{})
	if !sampleHasLabeled(got, "rm.dec.v", "k=x", 7) {
		t.Fatalf("expected decoded sample, got %v", samplesDump(onlyApp(got)))
	}
}

type decoderErrItem struct {
	V uint64 `metric:"name=rm.derr.v,type=gauge,help=..."`
}

func TestRegisterMetric_WithDecoder_Error_LogsAndContinues(t *testing.T) {
	resetShowRegistrations()
	t.Cleanup(resetShowRegistrations)

	const path = "rm.derr"
	src := newFakeShowSource()
	src.set(path, "anything")

	logger := &recordingLogger{}
	RegisterMetric[decoderErrItem](stringPath(path),
		WithDecoder(func(any) (any, error) {
			return nil, errors.New("decode failed")
		}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartShowPollers(ctx, src, logger)

	waitFor(t, func() bool { return src.calls.Load() >= 1 })

	warns := logger.snapshot()
	if len(warns) == 0 {
		t.Fatal("expected a warn log for decoder error")
	}
}

type decoderPanicItem struct {
	V uint64 `metric:"name=rm.dpan.v,type=gauge,help=..."`
}

func TestRegisterMetric_WithDecoder_Panic_TickerSurvives(t *testing.T) {
	resetShowRegistrations()
	t.Cleanup(resetShowRegistrations)

	const path = "rm.dpan"
	src := newFakeShowSource()
	src.set(path, "anything")

	logger := &recordingLogger{}
	var panicCount atomic.Int32
	RegisterMetric[decoderPanicItem](stringPath(path),
		WithDecoder(func(any) (any, error) {
			panicCount.Add(1)
			panic("decoder boom")
		}),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartShowPollers(ctx, src, logger)

	waitFor(t, func() bool { return src.calls.Load() >= 1 })
	// give the ticker a tick or two to survive
	time.Sleep(50 * time.Millisecond)

	if src.calls.Load() < 1 {
		t.Fatal("expected at least one Snapshot call")
	}
	if panicCount.Load() == 0 {
		t.Fatal("decoder was expected to panic")
	}

	warns := logger.snapshot()
	foundPanic := false
	for _, w := range warns {
		if contains(w, "panicked") {
			foundPanic = true
			break
		}
	}
	if !foundPanic {
		t.Fatalf("expected a panic warn log, got %v", warns)
	}
}

type snapshotErrItem struct {
	V uint64 `metric:"name=rm.serr.v,type=gauge,help=..."`
}

func TestRegisterMetric_SnapshotError_NotIfCtxCancelled(t *testing.T) {
	resetShowRegistrations()
	t.Cleanup(resetShowRegistrations)

	const path = "rm.serr"
	src := newFakeShowSource()
	src.setErr(path, context.Canceled)

	logger := &recordingLogger{}
	RegisterMetric[snapshotErrItem](stringPath(path))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	StartShowPollers(ctx, src, logger)

	time.Sleep(20 * time.Millisecond)
	if len(logger.snapshot()) != 0 {
		t.Fatalf("ctx-cancelled snapshot should not log, got %v", logger.snapshot())
	}
}

type nonStructT int

func TestRegisterMetric_NonStruct_Panics(t *testing.T) {
	resetShowRegistrations()
	t.Cleanup(resetShowRegistrations)

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on non-struct T")
		}
	}()
	RegisterMetric[nonStructT](stringPath("rm.bad"))
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("waitFor: condition not met within timeout")
}

func sampleHas(got []Sample, name string, value float64) bool {
	for _, s := range got {
		if s.Name == name && len(s.Labels) == 0 && s.Value == value {
			return true
		}
	}
	return false
}

func sampleHasLabeled(got []Sample, name, labelKV string, value float64) bool {
	for _, s := range got {
		if s.Name != name {
			continue
		}
		matched := false
		for _, l := range s.Labels {
			if l.Name+"="+l.Value == labelKV {
				matched = true
				break
			}
		}
		if matched && s.Value == value {
			return true
		}
	}
	return false
}

func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
