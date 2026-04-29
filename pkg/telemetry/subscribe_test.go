// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"context"
	"testing"
	"time"
)

// fastTickRegistry returns a registry with a sub-millisecond tick so tests
// don't have to wait whole seconds.
func fastTickRegistry(t *testing.T) *Registry {
	t.Helper()
	r := NewRegistry()
	r.SetTickInterval(5 * time.Millisecond)
	t.Cleanup(func() { _ = r.Shutdown(context.Background()) })
	return r
}

func waitForUpdate(t *testing.T, sub *Subscription) Update {
	t.Helper()
	select {
	case u := <-sub.Updates():
		return u
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for update")
		return Update{}
	}
}

func TestSubscribe_DeliversUpdates(t *testing.T) {
	t.Parallel()
	r := fastTickRegistry(t)
	c, _ := r.RegisterCounter(CounterOpts{Name: "osvbng_test_sub_basic", Help: "h", Labels: []string{"vrf"}})
	sub := r.Subscribe(SubscribeOptions{})

	c.WithLabelValues("a").Inc()

	u := waitForUpdate(t, sub)
	if u.Name != "osvbng_test_sub_basic" {
		t.Fatalf("got name %q, want osvbng_test_sub_basic", u.Name)
	}
	if u.Value != 1 {
		t.Fatalf("got value %v, want 1", u.Value)
	}
	if u.Timestamp.IsZero() {
		t.Errorf("update timestamp not populated")
	}
}

func TestSubscribe_GlobFilter(t *testing.T) {
	t.Parallel()
	r := fastTickRegistry(t)
	aaa, _ := r.RegisterCounter(CounterOpts{Name: "osvbng_aaa_x", Help: "h"})
	dhcp, _ := r.RegisterCounter(CounterOpts{Name: "osvbng_dhcp_x", Help: "h"})
	sub := r.Subscribe(SubscribeOptions{PathGlob: "osvbng_aaa_*"})

	aaa.WithLabelValues().Inc()
	dhcp.WithLabelValues().Inc()

	u := waitForUpdate(t, sub)
	if u.Name != "osvbng_aaa_x" {
		t.Fatalf("expected aaa update, got %q", u.Name)
	}

	select {
	case extra := <-sub.Updates():
		if extra.Name != "osvbng_aaa_x" {
			t.Fatalf("glob leaked: got %q", extra.Name)
		}
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSubscribe_StreamingOnlyExcludedByDefault(t *testing.T) {
	t.Parallel()
	r := fastTickRegistry(t)
	regular, _ := r.RegisterCounter(CounterOpts{Name: "osvbng_test_sub_regular", Help: "h"})
	streaming, _ := r.RegisterCounter(CounterOpts{
		Name:          "osvbng_test_sub_streaming",
		Help:          "h",
		Labels:        []string{"session_id"},
		StreamingOnly: true,
	})

	defaultSub := r.Subscribe(SubscribeOptions{})
	allSub := r.Subscribe(SubscribeOptions{IncludeStreamingOnly: true})

	regular.WithLabelValues().Inc()
	streaming.WithLabelValues("s-1").Inc()

	deadline := time.After(200 * time.Millisecond)
	defaultNames := map[string]bool{}
	allNames := map[string]bool{}
	for len(defaultNames) < 1 || len(allNames) < 2 {
		select {
		case u := <-defaultSub.Updates():
			defaultNames[u.Name] = true
		case u := <-allSub.Updates():
			allNames[u.Name] = true
		case <-deadline:
			t.Fatalf("timed out: defaultNames=%v allNames=%v", defaultNames, allNames)
		}
	}

	if defaultNames["osvbng_test_sub_streaming"] {
		t.Errorf("default subscriber received streaming-only metric")
	}
	if !allNames["osvbng_test_sub_regular"] || !allNames["osvbng_test_sub_streaming"] {
		t.Errorf("all subscriber missed metrics: %v", allNames)
	}
}

func TestSubscribe_DropOnOverflow(t *testing.T) {
	t.Parallel()
	r := fastTickRegistry(t)
	c, _ := r.RegisterCounter(CounterOpts{Name: "osvbng_test_sub_drop", Help: "h", Labels: []string{"vrf"}})
	sub := r.Subscribe(SubscribeOptions{BufferSize: 2})

	for i := 0; i < 50; i++ {
		c.WithLabelValues("a").Inc()
		time.Sleep(10 * time.Millisecond)
	}

	if sub.Dropped() == 0 {
		t.Fatalf("expected drops on a 2-buffer subscription, got 0")
	}
}

func TestSubscribe_SlowSubscriberDoesNotBlockOthers(t *testing.T) {
	t.Parallel()
	r := fastTickRegistry(t)
	c, _ := r.RegisterCounter(CounterOpts{Name: "osvbng_test_sub_slow", Help: "h", Labels: []string{"vrf"}})

	slow := r.Subscribe(SubscribeOptions{BufferSize: 2})
	fast := r.Subscribe(SubscribeOptions{BufferSize: 256})

	const emits = 30
	for i := 0; i < emits; i++ {
		c.WithLabelValues("a").Inc()
		time.Sleep(8 * time.Millisecond)
	}

	deadline := time.After(time.Second)
	got := 0
	for got == 0 || len(fast.Updates()) > 0 {
		select {
		case <-fast.Updates():
			got++
		case <-deadline:
			t.Fatalf("timed out waiting for fast subscriber updates")
		}
		if got >= 5 {
			break
		}
	}

	if got < 5 {
		t.Fatalf("fast subscriber should have received at least 5 updates, got %d", got)
	}
	if slow.Dropped() == 0 {
		t.Fatalf("expected slow subscriber to drop updates")
	}
}

func TestSubscribe_UnsubscribeStopsDelivery(t *testing.T) {
	t.Parallel()
	r := fastTickRegistry(t)
	c, _ := r.RegisterCounter(CounterOpts{Name: "osvbng_test_sub_unsub", Help: "h"})
	sub := r.Subscribe(SubscribeOptions{})

	c.WithLabelValues().Inc()
	waitForUpdate(t, sub)

	sub.Unsubscribe()

	for i := 0; i < 5; i++ {
		c.WithLabelValues().Inc()
	}

	time.Sleep(50 * time.Millisecond)

	drained := 0
	for {
		select {
		case _, ok := <-sub.Updates():
			if !ok {
				return
			}
			drained++
		default:
			if drained > 5 {
				t.Fatalf("subscriber received %d updates after unsubscribe", drained)
			}
			return
		}
	}
}

func TestSubscribe_TickStartsAndStops(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.SetTickInterval(5 * time.Millisecond)
	defer func() { _ = r.Shutdown(context.Background()) }()

	r.tickMu.Lock()
	if r.tickRunning {
		t.Fatalf("tick should not run before Subscribe")
	}
	r.tickMu.Unlock()

	sub := r.Subscribe(SubscribeOptions{})

	r.tickMu.Lock()
	running := r.tickRunning
	r.tickMu.Unlock()
	if !running {
		t.Fatalf("tick should run after Subscribe")
	}

	sub.Unsubscribe()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		r.tickMu.Lock()
		running = r.tickRunning
		r.tickMu.Unlock()
		if !running {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if running {
		t.Fatalf("tick should stop after last Unsubscribe")
	}

	sub2 := r.Subscribe(SubscribeOptions{})
	defer sub2.Unsubscribe()
	r.tickMu.Lock()
	running = r.tickRunning
	r.tickMu.Unlock()
	if !running {
		t.Fatalf("tick should restart on next Subscribe")
	}
}

func TestSubscribe_DirtyFlagSkipsCASOnZeroSubscribers(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	c, _ := r.RegisterCounter(CounterOpts{Name: "osvbng_test_sub_dirty_zero", Help: "h"})
	h := c.WithLabelValues()

	for i := 0; i < 1000; i++ {
		h.Inc()
	}
	if c.dirty.Load() {
		t.Fatalf("dirty flag should not be set when subscriber count is 0")
	}
}

func TestSubscribe_DirtyFlagSetByEmitWhenSubscriberPresent(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	c, _ := r.RegisterCounter(CounterOpts{Name: "osvbng_test_sub_dirty_set", Help: "h"})
	h := c.WithLabelValues()

	r.subscriberCount.Add(1)
	t.Cleanup(func() { r.subscriberCount.Add(-1) })

	h.Inc()
	if !c.dirty.Load() {
		t.Fatalf("dirty flag should be set after Inc with subscriber present")
	}

	if !c.swapDirty() {
		t.Fatalf("swapDirty should return true on a dirty metric")
	}
	if c.dirty.Load() {
		t.Fatalf("dirty flag should be cleared after swapDirty")
	}
}

func TestSubscribe_InternalMetrics(t *testing.T) {
	t.Parallel()
	r := fastTickRegistry(t)
	sub := r.Subscribe(SubscribeOptions{BufferSize: 2})
	c, _ := r.RegisterCounter(CounterOpts{Name: "osvbng_test_sub_internal", Help: "h", Labels: []string{"vrf"}})

	for i := 0; i < 30; i++ {
		c.WithLabelValues("a").Inc()
		time.Sleep(8 * time.Millisecond)
	}

	out := r.AppendSnapshot(nil, SnapshotOptions{})
	var sawSubsTotal, sawDrops bool
	for _, s := range out {
		switch s.Name {
		case internalMetricSubscriptionsTotal:
			sawSubsTotal = true
			if s.Value < 1 {
				t.Errorf("subscriptions_total = %v, want >=1", s.Value)
			}
		case internalMetricSubscriptionDrops:
			sawDrops = true
		}
	}
	if !sawSubsTotal {
		t.Errorf("missing %q in snapshot", internalMetricSubscriptionsTotal)
	}
	if sub.Dropped() > 0 && !sawDrops {
		t.Errorf("missing %q despite drops=%d", internalMetricSubscriptionDrops, sub.Dropped())
	}
}
