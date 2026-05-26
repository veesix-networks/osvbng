// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package component

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStateFileWriterBasicTransitions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state")
	w := NewStateFileWriter(path, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Run(ctx)

	w.Set(StateFileStarting)
	waitForState(t, path, StateFileStarting)

	w.Set(StateFileReady)
	waitForState(t, path, StateFileReady)

	w.Stop()
}

func TestStateFileWriterStampsVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state")
	w := NewStateFileWriter(path, "0.13.0-rc7")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Run(ctx)

	w.Set(StateFileReady)
	waitForState(t, path, StateFileReady)
	w.Stop()

	p, err := ReadStateFile(path)
	if err != nil {
		t.Fatalf("ReadStateFile: %v", err)
	}
	if p.Version != "0.13.0-rc7" {
		t.Fatalf("Version = %q, want %q", p.Version, "0.13.0-rc7")
	}
}

func TestStateFileWriterEmptyVersionOmitsField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state")
	w := NewStateFileWriter(path, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Run(ctx)

	w.Set(StateFileReady)
	waitForState(t, path, StateFileReady)
	w.Stop()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(raw), "\"version\"") {
		t.Fatalf("empty version should be omitted from JSON, got: %s", string(raw))
	}
}

func TestStateFileWriterSequenceMonotonic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state")
	w := NewStateFileWriter(path, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Run(ctx)

	for _, s := range []string{
		StateFileStarting,
		StateFileRestoring,
		StateFileReady,
		StateFileDegraded,
		StateFileReady,
	} {
		w.Set(s)
		waitForState(t, path, s)
	}

	w.Stop()

	payload, err := ReadStateFile(path)
	if err != nil {
		t.Fatalf("ReadStateFile: %v", err)
	}
	if payload.Sequence < 5 {
		t.Fatalf("Sequence = %d, want >= 5 after 5 transitions", payload.Sequence)
	}
}

func TestStateFileWriterConcurrentSetSerialises(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state")
	w := NewStateFileWriter(path, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Run(ctx)

	// Pre-emit one event so the file exists and readers don't race
	// the writer's first invocation.
	w.Set(StateFileStarting)
	waitForState(t, path, StateFileStarting)

	const goroutines = 100
	const setsPerG = 20

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			states := []string{StateFileRestoring, StateFileReady, StateFileDegraded}
			for j := 0; j < setsPerG; j++ {
				w.Set(states[j%len(states)])
			}
		}(i)
	}

	// Concurrent reader: every observed payload must decode cleanly
	// AND have a sequence value greater than or equal to the
	// previously observed one.
	stopRead := make(chan struct{})
	var readWG sync.WaitGroup
	readWG.Add(1)
	go func() {
		defer readWG.Done()
		var lastSeq uint64
		for {
			select {
			case <-stopRead:
				return
			default:
			}
			p, err := ReadStateFile(path)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				t.Errorf("reader observed bad state file: %v", err)
				return
			}
			if p.Sequence < lastSeq {
				t.Errorf("non-monotonic sequence: %d after %d", p.Sequence, lastSeq)
				return
			}
			lastSeq = p.Sequence
		}
	}()

	wg.Wait()
	close(stopRead)
	readWG.Wait()
	w.Stop()
}

func TestStateFileWriterAtomicWriteIntegrity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state")
	w := NewStateFileWriter(path, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Run(ctx)

	// Hammer Set and concurrently read; the read must always decode
	// cleanly even though many writes are in flight.
	stop := make(chan struct{})
	go func() {
		states := []string{StateFileStarting, StateFileRestoring, StateFileReady}
		i := 0
		for {
			select {
			case <-stop:
				return
			default:
			}
			w.Set(states[i%len(states)])
			i++
		}
	}()

	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		_, err := ReadStateFile(path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("reader saw bad file: %v", err)
		}
	}
	close(stop)
	w.Stop()
}

func TestStateFileWriterStopIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	w := NewStateFileWriter(filepath.Join(dir, "state"), "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Run(ctx)
	w.Set(StateFileReady)

	w.Stop()
	w.Stop() // second Stop must not panic
}

func TestStateFileWriterContextCancellationStopsLoop(t *testing.T) {
	dir := t.TempDir()
	w := NewStateFileWriter(filepath.Join(dir, "state"), "")
	ctx, cancel := context.WithCancel(context.Background())
	w.Run(ctx)

	cancel()
	// Stop blocks until the goroutine exits; if ctx cancel wasn't
	// observed this would hang.
	doneCh := make(chan struct{})
	go func() {
		w.Stop()
		close(doneCh)
	}()
	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return within 2s of ctx cancellation")
	}
}

func TestStateFileWriterSetAfterStopIsNoOp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state")
	w := NewStateFileWriter(path, "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Run(ctx)
	w.Set(StateFileReady)
	waitForState(t, path, StateFileReady)
	w.Stop()

	// Must not panic; the channel is closed by Stop.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Set on stopped writer panicked: %v", r)
		}
	}()
	w.Set(StateFileDegraded)
}

// fakeReadiness flips AllReady's return value on demand. Mimics the
// orchestrator's StateRestoring → StateReady transition without needing
// to instantiate real components.
type fakeReadiness struct {
	mu    sync.Mutex
	ready bool
}

func (f *fakeReadiness) AllReady() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ready
}

func (f *fakeReadiness) setReady(v bool) {
	f.mu.Lock()
	f.ready = v
	f.mu.Unlock()
}

func TestTrackReadinessRestoringToReadyTransition(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state")
	w := NewStateFileWriter(path, "")
	defer w.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Run(ctx)

	w.Set(StateFileRestoring)
	waitForState(t, path, StateFileRestoring)

	src := &fakeReadiness{ready: false}
	go TrackReadiness(ctx, w, src, 5*time.Millisecond)

	// Stay restoring until the source flips.
	time.Sleep(25 * time.Millisecond)
	if p, _ := ReadStateFile(path); p == nil || p.State != StateFileRestoring {
		t.Fatalf("state %v while AllReady=false, want restoring", p)
	}

	src.setReady(true)
	waitForState(t, path, StateFileReady)
}

func TestTrackReadinessReadyToDegradedTransition(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state")
	w := NewStateFileWriter(path, "")
	defer w.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Run(ctx)

	src := &fakeReadiness{ready: true}
	go TrackReadiness(ctx, w, src, 5*time.Millisecond)

	waitForState(t, path, StateFileReady)

	src.setReady(false)
	waitForState(t, path, StateFileDegraded)

	src.setReady(true)
	waitForState(t, path, StateFileReady)
}

func TestTrackReadinessStopsOnContextCancel(t *testing.T) {
	dir := t.TempDir()
	w := NewStateFileWriter(filepath.Join(dir, "state"), "")
	defer w.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	w.Run(ctx)

	src := &fakeReadiness{ready: true}
	done := make(chan struct{})
	go func() {
		TrackReadiness(ctx, w, src, 5*time.Millisecond)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("TrackReadiness did not return within 2s of ctx cancel")
	}
}

func TestReadStateFileMissingReturnsErrNotExist(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadStateFile(filepath.Join(dir, "absent"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("ReadStateFile on absent: err = %v, want fs.ErrNotExist", err)
	}
}

// waitForState polls the state file until it contains the supplied
// state, or the test deadline expires.
func waitForState(t *testing.T, path, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		p, err := ReadStateFile(path)
		if err == nil && p.State == want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("state file never reached %q within 2s", want)
}

// _ silences "import not used" if test names are pruned during
// refactors.
var _ = os.Stat
