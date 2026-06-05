// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package dataplane

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

type stubSouthbound struct {
	southbound.Southbound
	calls int32
	stats []southbound.InterfaceStats
	err   error
}

func (s *stubSouthbound) GetInterfaceStats() ([]southbound.InterfaceStats, error) {
	atomic.AddInt32(&s.calls, 1)
	return s.stats, s.err
}

func newHandler(sb *stubSouthbound, now func() time.Time) *InterfacesHandler {
	return &InterfacesHandler{southbound: sb, now: now}
}

func TestCollectCachesWithinTTL(t *testing.T) {
	sb := &stubSouthbound{stats: []southbound.InterfaceStats{{Index: 1, Name: "loop0"}}}
	t0 := time.Unix(1_700_000_000, 0)
	clock := t0
	h := newHandler(sb, func() time.Time { return clock })

	if _, err := h.Collect(context.Background(), &show.Request{}); err != nil {
		t.Fatalf("first Collect: %v", err)
	}
	clock = t0.Add(interfaceStatsTTL / 2)
	if _, err := h.Collect(context.Background(), &show.Request{}); err != nil {
		t.Fatalf("second Collect: %v", err)
	}
	if got := atomic.LoadInt32(&sb.calls); got != 1 {
		t.Fatalf("expected 1 stats read within TTL, got %d", got)
	}
}

func TestCollectRefetchesAfterTTL(t *testing.T) {
	sb := &stubSouthbound{stats: []southbound.InterfaceStats{{Index: 1}}}
	t0 := time.Unix(1_700_000_000, 0)
	clock := t0
	h := newHandler(sb, func() time.Time { return clock })

	if _, err := h.Collect(context.Background(), &show.Request{}); err != nil {
		t.Fatalf("first Collect: %v", err)
	}
	clock = t0.Add(interfaceStatsTTL + time.Millisecond)
	if _, err := h.Collect(context.Background(), &show.Request{}); err != nil {
		t.Fatalf("second Collect: %v", err)
	}
	if got := atomic.LoadInt32(&sb.calls); got != 2 {
		t.Fatalf("expected 2 stats reads across TTL, got %d", got)
	}
}

func TestCollectConcurrentSharesSingleRead(t *testing.T) {
	sb := &stubSouthbound{stats: []southbound.InterfaceStats{{Index: 1}}}
	t0 := time.Unix(1_700_000_000, 0)
	h := newHandler(sb, func() time.Time { return t0 })

	const N = 32
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			if _, err := h.Collect(context.Background(), &show.Request{}); err != nil {
				t.Errorf("Collect: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt32(&sb.calls); got != 1 {
		t.Fatalf("expected 1 stats read across %d concurrent callers, got %d", N, got)
	}
}

func TestCollectError(t *testing.T) {
	sb := &stubSouthbound{err: southbound.ErrUnavailable}
	t0 := time.Unix(1_700_000_000, 0)
	h := newHandler(sb, func() time.Time { return t0 })

	got, err := h.Collect(context.Background(), &show.Request{})
	if err == nil {
		t.Fatalf("expected error, got %v", got)
	}
	if h.cached != nil {
		t.Fatalf("error path should not populate cache, cached=%v", h.cached)
	}
}
