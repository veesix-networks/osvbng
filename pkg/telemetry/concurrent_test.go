// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConcurrent_MixedRegisterEmitSnapshotSubscribe(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.SetTickInterval(2 * time.Millisecond)
	t.Cleanup(func() { _ = r.Shutdown(context.Background()) })

	const (
		emitGoroutines = 16
		emitsEach      = 5000
		snapWorkers    = 4
		subscribers    = 8
	)

	c, _ := r.RegisterCounter(CounterOpts{
		Name:   "osvbng_test_conc_mix",
		Help:   "h",
		Labels: []string{"vrf"},
	})
	g, _ := r.RegisterGauge(GaugeOpts{Name: "osvbng_test_conc_mix_g", Help: "h"})
	hi, _ := r.RegisterHistogram(HistogramOpts{
		Name:    "osvbng_test_conc_mix_h",
		Help:    "h",
		Buckets: []float64{1, 5, 10},
	})

	stop := make(chan struct{})
	var emitWG, bgWG sync.WaitGroup

	for i := 0; i < emitGoroutines; i++ {
		emitWG.Add(1)
		go func(i int) {
			defer emitWG.Done()
			h := c.WithLabelValues("vrf-" + strconv.Itoa(i%4))
			gh := g.WithLabelValues()
			hh := hi.WithLabelValues()
			for j := 0; j < emitsEach; j++ {
				h.Inc()
				gh.Add(1)
				hh.Observe(float64(j % 12))
			}
		}(i)
	}

	for i := 0; i < snapWorkers; i++ {
		bgWG.Add(1)
		go func() {
			defer bgWG.Done()
			var buf []Sample
			for {
				select {
				case <-stop:
					return
				default:
					buf = r.AppendSnapshot(buf[:0], SnapshotOptions{})
				}
			}
		}()
	}

	subs := make([]*Subscription, subscribers)
	for i := range subs {
		subs[i] = r.Subscribe(SubscribeOptions{BufferSize: 64})
	}

	for _, sub := range subs {
		bgWG.Add(1)
		go func(s *Subscription) {
			defer bgWG.Done()
			for {
				select {
				case <-s.Updates():
				case <-stop:
					return
				}
			}
		}(sub)
	}

	emitWG.Wait()
	close(stop)
	for _, sub := range subs {
		sub.Unsubscribe()
	}
	bgWG.Wait()

	expected := uint64(emitGoroutines * emitsEach)
	var total uint64
	c.series.Range(func(_, v any) bool {
		total += v.(*CounterHandle).value.Load()
		return true
	})
	if total != expected {
		t.Fatalf("counter total=%d, want %d", total, expected)
	}
}

func TestConcurrent_ChurnUnderCardinalityCap(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	c, _ := r.RegisterCounter(CounterOpts{
		Name:               "osvbng_test_churn",
		Help:               "h",
		Labels:             []string{"vrf"},
		MaxSeriesPerMetric: 50,
	})

	const (
		creators         = 32
		tuplesPerCreator = 200
	)

	var wg sync.WaitGroup
	var distinctSeenAtomic atomic.Int64
	for i := 0; i < creators; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < tuplesPerCreator; j++ {
				h := c.WithLabelValues("vrf-" + strconv.Itoa(i*tuplesPerCreator+j))
				h.Inc()
				if !h.isTombstone {
					distinctSeenAtomic.Add(1)
				}
			}
		}(i)
	}
	wg.Wait()

	if got := c.seriesCount.Load(); got > 50 {
		t.Fatalf("seriesCount=%d, want <=50", got)
	}
	if c.cardinalityDrops.Load() == 0 {
		t.Fatalf("expected cardinality drops under churn, got 0")
	}
}

func TestConcurrent_NewRegistryIsolation(t *testing.T) {
	t.Parallel()
	r1 := NewRegistry()
	r2 := NewRegistry()

	c1, _ := r1.RegisterCounter(CounterOpts{Name: "shared_name", Help: "h", Labels: []string{"vrf"}})
	c2, _ := r2.RegisterCounter(CounterOpts{Name: "shared_name", Help: "h", Labels: []string{"vrf"}})
	if c1 == c2 {
		t.Fatalf("isolated registries returned the same Counter")
	}

	c1.WithLabelValues("a").Add(10)
	c2.WithLabelValues("a").Add(20)

	if got := c1.WithLabelValues("a").value.Load(); got != 10 {
		t.Fatalf("r1 a=%d, want 10", got)
	}
	if got := c2.WithLabelValues("a").value.Load(); got != 20 {
		t.Fatalf("r2 a=%d, want 20", got)
	}

	out1 := r1.AppendSnapshot(nil, SnapshotOptions{PathGlob: "shared_name"})
	for _, s := range out1 {
		if s.Name == "shared_name" && s.Value != 10 {
			t.Errorf("r1 snapshot leaked r2 value: %v", s.Value)
		}
	}
}

func TestConcurrent_UnsubscribeRaceWithEmit(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.SetTickInterval(1 * time.Millisecond)
	t.Cleanup(func() { _ = r.Shutdown(context.Background()) })

	c, _ := r.RegisterCounter(CounterOpts{Name: "osvbng_test_unsub_race", Help: "h"})
	h := c.WithLabelValues()

	const cycles = 200
	var wg sync.WaitGroup
	wg.Add(1)
	stop := make(chan struct{})
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				h.Inc()
			}
		}
	}()

	for i := 0; i < cycles; i++ {
		sub := r.Subscribe(SubscribeOptions{BufferSize: 4})
		time.Sleep(50 * time.Microsecond)
		sub.Unsubscribe()
	}
	close(stop)
	wg.Wait()
}

func TestConcurrent_DirtyFlagOnceBetweenTicks(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	c, _ := r.RegisterCounter(CounterOpts{Name: "osvbng_test_dirty_once", Help: "h"})

	r.subscriberCount.Add(1)
	t.Cleanup(func() { r.subscriberCount.Add(-1) })

	h := c.WithLabelValues()
	const burst = 5000
	for i := 0; i < burst; i++ {
		h.Inc()
	}

	if !c.swapDirty() {
		t.Fatal("dirty should be true after burst with subscriber present")
	}
	if c.swapDirty() {
		t.Fatal("dirty should be false after consume; burst must coalesce to one tick window")
	}
}
