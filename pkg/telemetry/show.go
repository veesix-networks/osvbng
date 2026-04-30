// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"context"
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

type pendingShowMetric struct {
	build func(reg *Registry) emitFn
}

type emitFn func(snap any)

var (
	pendingShowMu sync.Mutex
	pendingShows  = make(map[string]pendingShowMetric)
)

// RegisterMetricSingle binds metrics for T (carrying `metric:"..."` tags)
// to the show handler at path. The handler's snapshot is emitted as a
// single, no-label instance every time the owning component polls.
//
// Plugin authors call this from their show handler's init(). Polling is
// driven separately by the owning component's PollShowPath call once
// the component is started.
func RegisterMetricSingle[T any](path pathLike) {
	p := path.String()
	pendingShowMu.Lock()
	pendingShows[p] = pendingShowMetric{
		build: func(reg *Registry) emitFn {
			sm := MustRegisterStructWith[T](reg, RegisterOpts{})
			h := sm.WithLabelValues()
			return func(snap any) {
				t, ok := coerceTo[T](snap)
				if !ok {
					return
				}
				sm.EmitFrom(h, t)
			}
		},
	}
	pendingShowMu.Unlock()
}

// RegisterMetricMulti binds metrics for T to a show handler that returns
// []T or []*T. T's `metric:"label"` fields supply per-instance label
// values; remaining tagged fields supply per-instance values.
func RegisterMetricMulti[T any](path pathLike) {
	p := path.String()
	pendingShowMu.Lock()
	pendingShows[p] = pendingShowMetric{
		build: func(reg *Registry) emitFn {
			sm := MustRegisterStructWith[T](reg, RegisterOpts{})
			return func(snap any) {
				forEachAs(snap, sm.EmitInstance)
			}
		},
	}
	pendingShowMu.Unlock()
}

// showPollInterval is the SDK's fixed cadence for show-handler polls.
// Network operating systems do not expose telemetry collection cadence
// as an operator knob; this is the baseline behaviour of the platform.
const showPollInterval = 10 * time.Second

// StartShowPollers wires every queued RegisterMetricSingle / RegisterMetricMulti
// to a poll loop against src, and ticks them at the SDK's fixed cadence.
// cmd/osvbngd/main.go calls this once after the show registry is fully
// populated; plugin authors never call it.
//
// Show handlers whose underlying component is disabled return (nil, nil)
// or empty slices, and the SDK silently skips emit on those. Real
// Snapshot errors are logged at warn through log; pass log=nil to disable.
func StartShowPollers(ctx context.Context, src ShowSource, log Logger) {
	pendingShowMu.Lock()
	pending := make(map[string]pendingShowMetric, len(pendingShows))
	for p, m := range pendingShows {
		pending[p] = m
	}
	pendingShowMu.Unlock()

	for path, m := range pending {
		emit := m.build(defaultRegistry)
		path := path
		go pollLoop(ctx, src, path, emit, log)
	}
}

func pollLoop(ctx context.Context, src ShowSource, path string, emit emitFn, log Logger) {
	defer func() { _ = recover() }()

	pollOnce(ctx, src, path, emit, log)

	ticker := time.NewTicker(showPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pollOnce(ctx, src, path, emit, log)
		}
	}
}

func pollOnce(ctx context.Context, src ShowSource, path string, emit emitFn, log Logger) {
	snap, err := src.Snapshot(ctx, path)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		if log != nil {
			log.Warn("telemetry: show poll failed", "path", path, "error", err)
		}
		return
	}
	if isNilOrEmpty(snap) {
		return
	}
	emit(snap)
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

func coerceTo[T any](v any) (*T, bool) {
	if t, ok := v.(*T); ok {
		return t, true
	}
	if t, ok := v.(T); ok {
		return &t, true
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil, false
		}
		rv = rv.Elem()
	}
	if t, ok := rv.Interface().(T); ok {
		return &t, true
	}
	return nil, false
}

func forEachAs[T any](v any, fn func(*T)) {
	if vs, ok := v.([]T); ok {
		for i := range vs {
			fn(&vs[i])
		}
		return
	}
	if vs, ok := v.([]*T); ok {
		for _, p := range vs {
			if p != nil {
				fn(p)
			}
		}
		return
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Slice {
		return
	}
	for i := 0; i < rv.Len(); i++ {
		el := rv.Index(i)
		for el.Kind() == reflect.Ptr {
			if el.IsNil() {
				goto next
			}
			el = el.Elem()
		}
		if el.Kind() == reflect.Struct {
			if !el.CanAddr() {
				cp := reflect.New(el.Type()).Elem()
				cp.Set(el)
				el = cp
			}
			if t, ok := el.Addr().Interface().(*T); ok {
				fn(t)
			}
		}
	next:
	}
}
