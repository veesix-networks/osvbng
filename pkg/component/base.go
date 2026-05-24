// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package component

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// ReadyState is the punt-side lifecycle stage a component reports for its
// acceptance of new subscriber events. Distinct from the Ready() channel,
// which the orchestrator uses as a one-shot "Start() has finished" gate:
//
//   - The channel says "the orchestrator can move on to the next
//     component" (closed once via SignalReady).
//   - ReadyState says "the component is currently accepting / refusing new
//     subscriber traffic" and transitions multiple times across the
//     component's lifetime (NotReady -> Restoring -> Ready -> Draining).
//
// A component that does not opt into recovery stays at StateReady from
// construction and the gating check is a no-op for it. Components that
// participate in opdb-restore (IPoE / PPPoE / CGNAT) flip themselves into
// StateRestoring at the top of Start() and back to StateReady once their
// restore goroutine completes.
type ReadyState int32

const (
	// StateNotReady is the pre-Start state for components constructed via
	// NewBaseAsync (the orchestrator-gated kind). Punt handlers refuse new
	// session initiation; CPE protocol-level retry covers the gap.
	StateNotReady ReadyState = iota

	// StateRestoring is the recovery window: Start() has begun and the
	// component is replaying opdb checkpoints into the dataplane. New
	// session initiation is still refused so the replay does not race
	// against fresh subscriber events.
	StateRestoring

	// StateReady is steady-state: punt-side handlers accept new session
	// initiation. NewBase() constructs components directly in this state.
	StateReady

	// StateDraining is graceful shutdown: existing sessions continue to
	// operate; punt handlers refuse new initiation. Set by the
	// orchestrator when teardown begins.
	StateDraining
)

// String returns the lowercase state name suitable for API output and
// telemetry labels.
func (s ReadyState) String() string {
	switch s {
	case StateNotReady:
		return "not_ready"
	case StateRestoring:
		return "restoring"
	case StateReady:
		return "ready"
	case StateDraining:
		return "draining"
	}
	return "unknown"
}

// RestoreProgress is the operator-visible snapshot of how many opdb
// checkpoints the component has replayed so far. Surfaced by the
// /api/show/system/recovery/status handler and the watchdog timeout
// trigger.
type RestoreProgress struct {
	Total     int       `json:"total"`
	Restored  int       `json:"restored"`
	Failed    int       `json:"failed"`
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Base is the shared component skeleton: name, lifecycle context, ready
// channel for orchestrator sequencing, and a per-component ReadyState
// machine that punt-side handlers gate on.
type Base struct {
	name     string
	Ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	readyCh  chan struct{}
	readyOn  sync.Once
	state    atomic.Int32
	progress atomic.Pointer[RestoreProgress]
}

// NewBase constructs a component that is immediately ready: the Ready
// channel is already closed and the ReadyState is StateReady. Use for
// components that have no async startup work and no recovery window.
func NewBase(name string) *Base {
	ch := make(chan struct{})
	close(ch)
	b := &Base{
		name:    name,
		readyCh: ch,
	}
	b.state.Store(int32(StateReady))
	return b
}

// NewBaseAsync constructs a component whose Start() runs asynchronously.
// The Ready channel stays open until SignalReady() is called, blocking
// Orchestrator.WaitReady() for ordered startup. ReadyState begins at
// StateNotReady; the component is responsible for transitioning it as
// recovery progresses.
func NewBaseAsync(name string) *Base {
	b := &Base{
		name:    name,
		readyCh: make(chan struct{}),
	}
	b.state.Store(int32(StateNotReady))
	return b
}

// Ready returns a channel that is closed once SignalReady() has been
// called. Used by Orchestrator.WaitReady() to gate ordered startup.
func (b *Base) Ready() <-chan struct{} {
	return b.readyCh
}

// SignalReady marks Start() as finished. Idempotent: the underlying
// sync.Once guarantees a single channel-close even under concurrent
// callers. Does NOT modify the ReadyState — recovery-aware components
// manage ReadyState independently via SetReadyState(). Use SignalReady()
// when the synchronous portion of Start() is done, regardless of whether
// any async recovery goroutine is still running.
func (b *Base) SignalReady() {
	b.readyOn.Do(func() {
		close(b.readyCh)
	})
}

// SetReadyState transitions the punt-side acceptance state. Safe for
// concurrent reads from any goroutine; transitions should happen from
// the component's own start / recovery goroutine.
func (b *Base) SetReadyState(s ReadyState) {
	b.state.Store(int32(s))
}

// ReadyState returns the current punt-side acceptance state.
func (b *Base) ReadyState() ReadyState {
	return ReadyState(b.state.Load())
}

// IsReady is the fast-path check used by punt-side handlers in the hot
// path of every subscriber-initiation packet. Returns true only when the
// component is in StateReady; every other state means the handler should
// drop the packet so the CPE retransmits when the component is ready.
func (b *Base) IsReady() bool {
	return b.ReadyState() == StateReady
}

// SetProgress publishes the latest restore-progress snapshot. The
// recovery-status API reads this to expose "N restored / M total /
// K failed" to operators.
func (b *Base) SetProgress(p RestoreProgress) {
	p.UpdatedAt = time.Now()
	b.progress.Store(&p)
}

// Progress returns the most recent RestoreProgress snapshot, or nil if
// none has been published. Treat nil as "no recovery cycle has run yet"
// for this component.
func (b *Base) Progress() *RestoreProgress {
	return b.progress.Load()
}

func (b *Base) Name() string {
	return b.name
}

func (b *Base) StartContext(parentCtx context.Context) {
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	b.Ctx, b.cancel = context.WithCancel(parentCtx)
}

func (b *Base) StopContext() {
	if b.cancel != nil {
		b.cancel()
	}
	b.wg.Wait()
}

func (b *Base) Go(fn func()) {
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		fn()
	}()
}
