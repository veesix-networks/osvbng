// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package component

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StateFileState enumerates the values the state file may hold. The
// upgrade health-poll consumes these to decide whether a starting
// daemon is making progress, has reached steady state, or has
// degraded.
const (
	StateFileStarting  = "starting"
	StateFileRestoring = "restoring"
	StateFileReady     = "ready"
	StateFileDegraded  = "degraded"
	StateFileStopping  = "stopping"
)

// StateFilePayload is the on-disk JSON shape osvbngd writes to its
// runtime state file (typically /run/osvbng/state). Sequence is a
// monotonic in-memory counter incremented per write; the upgrade
// health-poll uses it as the primary stall-detection signal so
// wall-clock adjustments and same-tick writes do not confuse the
// "is the daemon making progress" check.
type StateFilePayload struct {
	State     string    `json:"state"`
	Sequence  uint64    `json:"sequence"`
	UpdatedAt time.Time `json:"updated_at"`
	// Version is the running daemon's pkg/version.Version. The upgrade
	// health-poll uses this to distinguish a new daemon's writes from a
	// previous daemon's stale state during the post-swap race window —
	// otherwise the poller can read the outgoing daemon's last write and
	// short-circuit before the new daemon publishes.
	Version string `json:"version,omitempty"`
}

// StateFileWriter is a single-goroutine writer for /run/osvbng/state.
// All Set calls funnel through a buffered channel; the writer
// goroutine serialises and timestamps writes. Atomic tmp+rename ensures
// readers never see a partial payload.
//
// The writer goroutine runs until ctx is cancelled or Stop is called.
// Set on a stopped writer is a no-op so callers do not need to track
// the lifecycle precisely.
type StateFileWriter struct {
	path    string
	version string
	mu      sync.Mutex
	ch      chan string
	done    chan struct{}
	seq     uint64

	startOnce sync.Once
	stopOnce  sync.Once
	started   bool
	stopped   bool
}

// NewStateFileWriter returns a writer bound to a path, stamping the
// supplied version into every write so external readers (the upgrade
// health-poll) can tell which daemon process owns the on-disk state.
// Pass "" to omit the version field — tests and pre-version-tracking
// callers may rely on that.
//
// The parent directory must exist before Run is called — osvbng.service
// uses `RuntimeDirectory=osvbng` so systemd handles /run/osvbng/
// lifecycle automatically; the writer does not MkdirAll.
//
// Bufsize controls the channel capacity. The default (8) is enough for
// the small number of transitions that happen in a normal lifecycle;
// callers in a hot path should not be calling Set directly.
func NewStateFileWriter(path, version string) *StateFileWriter {
	return &StateFileWriter{
		path:    path,
		version: version,
		ch:      make(chan string, 8),
		done:    make(chan struct{}),
	}
}

// Run starts the writer goroutine. Blocks until ctx is cancelled or
// Stop is called. Returns when the channel is fully drained and the
// final write (if any) has completed.
//
// Run is safe to call once per writer; subsequent calls are no-ops.
func (w *StateFileWriter) Run(ctx context.Context) {
	w.startOnce.Do(func() {
		w.started = true
		go w.loop(ctx)
	})
}

// Set requests a state transition. Non-blocking: if the channel is
// full (writer falling behind), the most recent value wins after the
// channel drains. Calling Set on a stopped writer is a silent no-op.
func (w *StateFileWriter) Set(state string) {
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return
	}
	w.mu.Unlock()
	select {
	case w.ch <- state:
	default:
		// Channel full — drop the intermediate; readers will see the
		// next transition that successfully enqueues.
	}
}

// Stop signals the writer to drain and exit. Safe to call multiple
// times. Blocks until the goroutine has exited so cleanup is
// deterministic.
func (w *StateFileWriter) Stop() {
	w.stopOnce.Do(func() {
		w.mu.Lock()
		w.stopped = true
		w.mu.Unlock()
		close(w.ch)
	})
	if w.started {
		<-w.done
	}
}

// Path returns the on-disk path. Useful for diagnostics.
func (w *StateFileWriter) Path() string { return w.path }

func (w *StateFileWriter) loop(ctx context.Context) {
	defer close(w.done)
	for {
		select {
		case <-ctx.Done():
			return
		case state, ok := <-w.ch:
			if !ok {
				return
			}
			w.write(state)
		}
	}
}

func (w *StateFileWriter) write(state string) {
	w.mu.Lock()
	w.seq++
	payload := StateFilePayload{
		State:     state,
		Sequence:  w.seq,
		UpdatedAt: time.Now().UTC(),
		Version:   w.version,
	}
	w.mu.Unlock()

	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	dir := filepath.Dir(w.path)
	tmp, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		cleanup()
		return
	}
	if err := os.Rename(tmpPath, w.path); err != nil {
		cleanup()
		return
	}
}

// ReadinessSource is the minimal surface TrackReadiness needs from an
// orchestrator. Defined as an interface so tests can drive the tracker
// without standing up a real orchestrator and so the writer package
// stays decoupled from the orchestrator's full method set.
type ReadinessSource interface {
	AllReady() bool
}

// TrackReadiness polls source.AllReady() at the supplied interval and
// updates the state file as the daemon transitions through restoring
// → ready → degraded.
//
// Initial transition (StateFileRestoring → StateFileReady) happens
// when the first observed AllReady() returns true — this is what
// distinguishes "daemon synchronous start finished" (the orchestrator's
// WaitReady) from "every component's opdb recovery goroutine has also
// finished and the punt path is now accepting new sessions" (AllReady).
//
// Steady-state ready → degraded transition is observed and written so
// the upgrade health-poll can react to a component fault between
// upgrades.
//
// Returns when ctx is cancelled. Does not call w.Stop; the caller owns
// the writer lifecycle.
func TrackReadiness(ctx context.Context, w *StateFileWriter, source ReadinessSource, interval time.Duration) {
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var wasReady bool
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		ready := source.AllReady()
		if ready && !wasReady {
			w.Set(StateFileReady)
			wasReady = true
		}
		if !ready && wasReady {
			w.Set(StateFileDegraded)
			wasReady = false
		}
	}
}

// ReadStateFile decodes the on-disk payload. Used by tests and by
// out-of-process readers (the upgrade health-poll reads its own copy
// inside pkg/upgrade to avoid a reverse import).
func ReadStateFile(path string) (*StateFilePayload, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("state file is empty")
	}
	var p StateFilePayload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("decode state file %s: %w", path, err)
	}
	return &p, nil
}
