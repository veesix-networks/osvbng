// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"context"
	"strconv"
	"testing"
	"time"
)

func BenchmarkCounter_Inc_Resolved_NoSubs(b *testing.B) {
	r := NewRegistry()
	c, _ := r.RegisterCounter(CounterOpts{Name: "bench_inc_nosubs", Help: "h", Labels: []string{"vrf"}})
	h := c.WithLabelValues("a")
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		h.Inc()
	}
}

func BenchmarkCounter_Inc_Resolved_OneSub(b *testing.B) {
	r := NewRegistry()
	r.SetTickInterval(time.Hour)
	c, _ := r.RegisterCounter(CounterOpts{Name: "bench_inc_onesub", Help: "h", Labels: []string{"vrf"}})
	sub := r.Subscribe(SubscribeOptions{BufferSize: 1024})
	defer sub.Unsubscribe()
	defer func() { _ = r.Shutdown(context.Background()) }()

	h := c.WithLabelValues("a")
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		h.Inc()
	}
}

func BenchmarkCounter_Inc_Variadic_Existing(b *testing.B) {
	r := NewRegistry()
	c, _ := r.RegisterCounter(CounterOpts{Name: "bench_inc_var_existing", Help: "h", Labels: []string{"vrf"}})
	c.WithLabelValues("a")
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c.Inc("a")
	}
}

func BenchmarkCounter_Inc_Variadic_Unknown(b *testing.B) {
	r := NewRegistry()
	c, _ := r.RegisterCounter(CounterOpts{Name: "bench_inc_var_unknown", Help: "h", Labels: []string{"vrf"}})
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c.Inc("unknown")
	}
}

func BenchmarkCounter_Inc_Tombstone(b *testing.B) {
	r := NewRegistry()
	c, _ := r.RegisterCounter(CounterOpts{
		Name:               "bench_inc_tombstone",
		Help:               "h",
		Labels:             []string{"vrf"},
		MaxSeriesPerMetric: 1,
	})
	c.WithLabelValues("first")
	tomb := c.WithLabelValues("over")
	if !tomb.isTombstone {
		b.Fatal("setup failed: handle is not tombstone")
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		tomb.Inc()
	}
}

func BenchmarkGauge_Set_Resolved(b *testing.B) {
	r := NewRegistry()
	g, _ := r.RegisterGauge(GaugeOpts{Name: "bench_gauge_set", Help: "h", Labels: []string{"vrf"}})
	h := g.WithLabelValues("a")
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		h.Set(float64(i))
	}
}

func BenchmarkGauge_Add_FloatCAS(b *testing.B) {
	r := NewRegistry()
	g, _ := r.RegisterGauge(GaugeOpts{Name: "bench_gauge_add", Help: "h"})
	h := g.WithLabelValues()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		h.Add(1)
	}
}

func BenchmarkHistogram_Observe_Resolved(b *testing.B) {
	r := NewRegistry()
	hi, _ := r.RegisterHistogram(HistogramOpts{
		Name:    "bench_hist_observe",
		Help:    "h",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
	})
	h := hi.WithLabelValues()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		h.Observe(0.05)
	}
}

func BenchmarkAppendSnapshot_Counters(b *testing.B) {
	r := NewRegistry()
	const metrics = 20
	const seriesPerMetric = 50
	for m := 0; m < metrics; m++ {
		c, _ := r.RegisterCounter(CounterOpts{
			Name:   "bench_snap_" + strconv.Itoa(m),
			Help:   "h",
			Labels: []string{"vrf"},
		})
		for s := 0; s < seriesPerMetric; s++ {
			c.WithLabelValues(strconv.Itoa(s)).Inc()
		}
	}
	dst := make([]Sample, 0, metrics*seriesPerMetric+16)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		dst = r.AppendSnapshot(dst[:0], SnapshotOptions{})
	}
}

func BenchmarkAppendSnapshot_Histograms(b *testing.B) {
	r := NewRegistry()
	const histograms = 10
	const seriesPerHistogram = 5
	for m := 0; m < histograms; m++ {
		h, _ := r.RegisterHistogram(HistogramOpts{
			Name:    "bench_snap_hist_" + strconv.Itoa(m),
			Help:    "h",
			Labels:  []string{"vrf"},
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1},
		})
		for s := 0; s < seriesPerHistogram; s++ {
			h.WithLabelValues(strconv.Itoa(s)).Observe(0.05)
		}
	}
	dst := make([]Sample, 0, histograms*seriesPerHistogram+16)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		dst = r.AppendSnapshot(dst[:0], SnapshotOptions{})
	}
}

func BenchmarkSubscribe_FanOut(b *testing.B) {
	r := NewRegistry()
	r.SetTickInterval(time.Hour)
	defer func() { _ = r.Shutdown(context.Background()) }()

	const subs = 16
	const metrics = 16
	for i := 0; i < subs; i++ {
		s := r.Subscribe(SubscribeOptions{BufferSize: 4096})
		defer s.Unsubscribe()
	}
	handles := make([]*CounterHandle, metrics)
	for i := 0; i < metrics; i++ {
		c, _ := r.RegisterCounter(CounterOpts{
			Name:   "bench_fanout_" + strconv.Itoa(i),
			Help:   "h",
			Labels: []string{"vrf"},
		})
		handles[i] = c.WithLabelValues("a")
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		handles[i%metrics].Inc()
	}
}

func BenchmarkChurn_NewSeries(b *testing.B) {
	r := NewRegistry()
	c, _ := r.RegisterCounter(CounterOpts{
		Name:               "bench_churn_new",
		Help:               "h",
		Labels:             []string{"vrf"},
		MaxSeriesPerMetric: 1024,
	})
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c.WithLabelValues(strconv.Itoa(i)).Inc()
	}
}

func BenchmarkChurn_Contention(b *testing.B) {
	r := NewRegistry()
	c, _ := r.RegisterCounter(CounterOpts{
		Name:   "bench_churn_contention",
		Help:   "h",
		Labels: []string{"vrf"},
	})
	const distinct = 32
	handles := make([]*CounterHandle, distinct)
	for i := range handles {
		handles[i] = c.WithLabelValues(strconv.Itoa(i))
	}
	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			handles[i%distinct].Inc()
			i++
		}
	})
}
