// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func testLogger(buf *bytes.Buffer) *Logger {
	return &Logger{zl: zerolog.New(buf)}
}

func logLines(buf *bytes.Buffer) []map[string]any {
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			continue
		}
		out = append(out, m)
	}
	return out
}

func TestSamplerFirstEventLogsThenSuppresses(t *testing.T) {
	var buf bytes.Buffer
	s := NewSampler(testLogger(&buf), time.Hour)

	for i := 0; i < 100; i++ {
		s.Error("radius", "auth failed", "server", "10.0.0.1")
	}

	lines := logLines(&buf)
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	if _, ok := lines[0]["suppressed"]; ok {
		t.Errorf("first line must not carry suppressed: %v", lines[0])
	}
}

func TestSamplerSummaryCarriesSuppressedCount(t *testing.T) {
	var buf bytes.Buffer
	s := NewSampler(testLogger(&buf), 30*time.Millisecond)

	for i := 0; i < 100; i++ {
		s.Warn("pool", "exhausted")
	}
	time.Sleep(40 * time.Millisecond)
	s.Warn("pool", "exhausted")

	lines := logLines(&buf)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if got := lines[1]["suppressed"]; got != float64(99) {
		t.Errorf("suppressed = %v, want 99", got)
	}
}

func TestSamplerKeysAreIndependent(t *testing.T) {
	var buf bytes.Buffer
	s := NewSampler(testLogger(&buf), time.Hour)

	s.Error("a", "m")
	s.Error("b", "m")
	s.Error("a", "m")

	if lines := logLines(&buf); len(lines) != 2 {
		t.Fatalf("got %d lines, want 2 (one per key)", len(lines))
	}
}

// Conservation under concurrency: every event is either logged or counted
// into a suppressed field, exactly once.
func TestSamplerConservesEvents(t *testing.T) {
	var buf bytes.Buffer
	var mu sync.Mutex
	s := NewSampler(&Logger{zl: zerolog.New(lockedWriter{&mu, &buf})}, 5*time.Millisecond)

	const goroutines, events = 8, 500
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < events; i++ {
				s.Error("k", "m")
				if i%100 == 0 {
					time.Sleep(time.Millisecond)
				}
			}
		}()
	}
	wg.Wait()
	time.Sleep(10 * time.Millisecond)
	s.Error("k", "m") // flush any pending count

	mu.Lock()
	lines := logLines(&buf)
	mu.Unlock()

	total := 0
	for _, l := range lines {
		total++
		if v, ok := l["suppressed"]; ok {
			total += int(v.(float64))
		}
	}
	// Residual: events suppressed after the flush line's window opened.
	pending := goroutines*events + 1 - total
	if pending != 0 {
		t.Errorf("conservation: %d events unaccounted (logged+suppressed=%d of %d)",
			pending, total, goroutines*events+1)
	}
	if len(lines) >= goroutines*events {
		t.Errorf("sampling ineffective: %d lines for %d events", len(lines), goroutines*events)
	}
}

type lockedWriter struct {
	mu  *sync.Mutex
	buf *bytes.Buffer
}

func (w lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}
