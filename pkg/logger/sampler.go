// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package logger

import (
	"sync"
	"sync/atomic"
	"time"
)

// Sampler rate-limits storm-shaped logging per key. The first event for a
// key logs immediately and opens a window; events inside the window are
// counted, not logged; the first event after the window closes logs again,
// carrying the count as a "suppressed" field. There is no background timer:
// a count with no follow-up event stays pending until the next event for
// that key, which keeps the sampler off the scheduler entirely.
//
// The hot path is one sync.Map load and one or two atomics, no allocation
// when suppressing. Keys follow the same bounded-cardinality discipline as
// metric labels: key by server, pool or operation, never by subscriber,
// session or MAC. Entries are never removed, so an unbounded key set is a
// leak as well as a signal problem.
type Sampler struct {
	log      *Logger
	interval time.Duration
	entries  sync.Map // key string -> *sampleEntry
}

type sampleEntry struct {
	windowStart atomic.Int64 // unix nanos; zero means never logged
	suppressed  atomic.Uint64
}

// NewSampler returns a Sampler emitting through log at most one line per
// key per interval.
func NewSampler(log *Logger, interval time.Duration) *Sampler {
	return &Sampler{log: log, interval: interval}
}

// allow reports whether this event should log, and if so how many events
// for the key were suppressed since the last logged one. Exactly one
// concurrent caller wins an expired window (CAS); losers are counted.
func (s *Sampler) allow(key string) (uint64, bool) {
	v, ok := s.entries.Load(key)
	if !ok {
		v, _ = s.entries.LoadOrStore(key, &sampleEntry{})
	}
	e := v.(*sampleEntry)

	now := time.Now().UnixNano()
	start := e.windowStart.Load()
	if now-start >= int64(s.interval) {
		if e.windowStart.CompareAndSwap(start, now) {
			return e.suppressed.Swap(0), true
		}
	}
	e.suppressed.Add(1)
	return 0, false
}

func (s *Sampler) Warn(key, msg string, args ...any) {
	if n, ok := s.allow(key); ok {
		s.log.Warn(msg, withSuppressed(args, n)...)
	}
}

func (s *Sampler) Error(key, msg string, args ...any) {
	if n, ok := s.allow(key); ok {
		s.log.Error(msg, withSuppressed(args, n)...)
	}
}

// withSuppressed appends the count without aliasing the caller's backing
// array (the full slice expression forces a copy on append).
func withSuppressed(args []any, n uint64) []any {
	if n == 0 {
		return args
	}
	return append(args[:len(args):len(args)], "suppressed", n)
}
