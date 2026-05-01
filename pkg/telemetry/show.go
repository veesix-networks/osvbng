// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"
)

// pathLike accepts any string-typed show-path constant
// (pkg/handlers/show/paths.Path, pkg/state/paths.Path, etc.) so plugin
// authors can pass their existing typed path values directly.
type pathLike interface {
	String() string
}

// ShowSource is the contract telemetry needs from the show registry: hand
// back the snapshot at this path. pkg/handlers/show.Registry satisfies it
// via its Snapshot method. Defined here so telemetry stays independent of
// the show package.
type ShowSource interface {
	Snapshot(ctx context.Context, path string) (any, error)
}

// Logger is the minimal logging contract telemetry needs. *logger.Logger
// satisfies it implicitly so telemetry doesn't import pkg/logger. Pass
// nil to disable poll logging entirely.
type Logger interface {
	Debug(msg string, args ...any)
	Warn(msg string, args ...any)
}

// RegisterOption configures RegisterMetric registration. Currently only
// WithDecoder is exposed; future shape-specific options can be added here
// without changing the call signature for existing callers.
type RegisterOption func(*registration)

type registration struct {
	decoder func(any) (any, error)
}

// WithDecoder adapts a show handler that returns interface{} or
// json.RawMessage. The decoder runs before the walker each poll; its
// return must be a value the walker can iterate as T (or a collection
// thereof). Panics from decoder OR walker are recovered per poll with a
// one-shot warn log; the ticker survives. Documented as a stop-gap for
// handlers awaiting a typed-return refactor.
func WithDecoder(fn func(any) (any, error)) RegisterOption {
	return func(r *registration) { r.decoder = fn }
}

type pendingShowMetric struct {
	typ     reflect.Type
	metrics *structMetrics
	decoder func(any) (any, error)

	lifecycleMu sync.Mutex
	prev        *pollState
}

var (
	pendingShowMu sync.Mutex
	pendingShows  = make(map[string]*pendingShowMetric)
)

// RegisterMetric binds metrics for T to the show handler at path. T is
// always the element type; the walker auto-detects the handler's return
// shape (struct, pointer, slice, slice-of-pointer, map[K]V, map[K][]V).
//
// Plugin authors call this from their show handler's init(). Polling is
// driven separately by StartShowPollers (one call from main.go).
//
// Programmer errors panic at registration time:
//   - Calling RegisterMetric twice for the same path with a different T
//     panics with ErrTypeMismatch.
//   - Idempotent re-registration with the same T is a no-op.
//   - Tag conflicts, label-name collisions, duplicate metric names across
//     flatten paths, cyclic flatten types, dual map_key fields, and
//     invalid flatten kinds all panic via bindShowType.
func RegisterMetric[T any](path pathLike, opts ...RegisterOption) {
	var reg registration
	for _, o := range opts {
		o(&reg)
	}

	var zero T
	typ := reflect.TypeOf(zero)
	if typ == nil || typ.Kind() != reflect.Struct {
		panic(fmt.Errorf("telemetry: RegisterMetric: T must be a struct, got %T", zero))
	}

	p := path.String()
	pendingShowMu.Lock()
	defer pendingShowMu.Unlock()

	if existing, ok := pendingShows[p]; ok {
		if existing.typ != typ {
			panic(fmt.Errorf("%w: path=%q existing=%s new=%s", ErrTypeMismatch, p, existing.typ, typ))
		}
		return
	}

	sm := bindShowType(defaultRegistry, typ)
	pendingShows[p] = &pendingShowMetric{
		typ:     typ,
		metrics: sm,
		decoder: reg.decoder,
	}
}

// showPollInterval is the SDK's fixed cadence for show-handler polls.
// Network operating systems do not expose telemetry collection cadence
// as an operator knob; this is the baseline behaviour of the platform.
const showPollInterval = 10 * time.Second

// StartShowPollers wires every queued RegisterMetric to a poll loop
// against src and ticks them at the SDK's fixed cadence. cmd/osvbngd/main.go
// calls this once after the show registry is fully populated; plugin
// authors never call it.
//
// Show handlers whose underlying component is disabled return (nil, nil)
// or empty slices, and the SDK silently skips emit on those. Real
// Snapshot errors and decoder errors log warn through log; pass log=nil
// to disable. Panics in decoder, walker, or any reflection step are
// recovered per poll with a one-shot warn; the ticker survives.
func StartShowPollers(ctx context.Context, src ShowSource, log Logger) {
	pendingShowMu.Lock()
	pending := make([]*pendingShowMetric, 0, len(pendingShows))
	paths := make([]string, 0, len(pendingShows))
	for p, m := range pendingShows {
		pending = append(pending, m)
		paths = append(paths, p)
	}
	pendingShowMu.Unlock()

	for i := range pending {
		m := pending[i]
		path := paths[i]
		go pollLoop(ctx, src, path, m, log)
	}
}

func pollLoop(ctx context.Context, src ShowSource, path string, m *pendingShowMetric, log Logger) {
	pollOnce(ctx, src, path, m, log)

	ticker := time.NewTicker(showPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pollOnce(ctx, src, path, m, log)
		}
	}
}

func pollOnce(ctx context.Context, src ShowSource, path string, m *pendingShowMetric, log Logger) {
	defer func() {
		if r := recover(); r != nil {
			warnOnce(path, "panic", log, "telemetry: show poll panicked", "panic", fmt.Sprintf("%v", r))
		}
	}()

	snap, err := src.Snapshot(ctx, path)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		warnOnce(path, "snapshot_error", log, "telemetry: show poll failed", "error", err)
		return
	}
	if isNilOrEmpty(snap) {
		return
	}

	if m.decoder != nil {
		decoded, derr := m.decoder(snap)
		if derr != nil {
			warnOnce(path, "decoder_error", log, "telemetry: show decoder failed", "error", derr)
			return
		}
		if isNilOrEmpty(decoded) {
			return
		}
		snap = decoded
	}

	rv := reflect.ValueOf(snap)
	if !rv.IsValid() {
		return
	}

	current := newPollState()
	m.metrics.walk(rv, current)

	m.lifecycleMu.Lock()
	m.prev = reconcile(m.prev, current)
	m.lifecycleMu.Unlock()
}

func isNilOrEmpty(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Interface:
		return rv.IsNil()
	case reflect.Slice, reflect.Map:
		return rv.Len() == 0
	}
	return false
}

// warnOnce emits a warn log for (path, kind) at most once per process.
// kind is a short tag like "snapshot_error", "decoder_error", "panic" so
// distinct failure modes against the same path are each surfaced once.
var warnLogged sync.Map // key: path+"|"+kind, value: struct{}

func warnOnce(path, kind string, log Logger, msg string, kv ...any) {
	if log == nil {
		return
	}
	key := path + "|" + kind
	if _, loaded := warnLogged.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	args := make([]any, 0, len(kv)+2)
	args = append(args, "path", path)
	args = append(args, kv...)
	log.Warn(msg, args...)
}
