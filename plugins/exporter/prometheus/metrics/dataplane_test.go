// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package metrics

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

func TestSDKShim_TranslatesCounter(t *testing.T) {
	t.Parallel()
	reg := telemetry.NewRegistry()
	c, err := reg.RegisterCounter(telemetry.CounterOpts{
		Name:   "vpp_test_counter",
		Help:   "test counter",
		Labels: []string{"name"},
	})
	if err != nil {
		t.Fatalf("RegisterCounter: %v", err)
	}
	c.WithLabelValues("a").Add(7)
	c.WithLabelValues("b").Add(3)

	shim := newSDKShim("test", "vpp_test_*", nil, reg)

	ch := make(chan prometheus.Metric, 16)
	if err := shim.Collect(context.Background(), nil, ch); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	close(ch)

	values := map[string]float64{}
	for m := range ch {
		var pb dto.Metric
		if err := m.Write(&pb); err != nil {
			t.Fatalf("Write: %v", err)
		}
		var name string
		for _, lp := range pb.Label {
			if lp.GetName() == "name" {
				name = lp.GetValue()
			}
		}
		values[name] = pb.Counter.GetValue()
	}
	if values["a"] != 7 || values["b"] != 3 {
		t.Fatalf("got %v, want a=7 b=3", values)
	}
}

func TestSDKShim_TranslatesGauge(t *testing.T) {
	t.Parallel()
	reg := telemetry.NewRegistry()
	g, err := reg.RegisterGauge(telemetry.GaugeOpts{
		Name:   "vpp_test_gauge",
		Help:   "test gauge",
		Labels: []string{"pool"},
	})
	if err != nil {
		t.Fatalf("RegisterGauge: %v", err)
	}
	g.WithLabelValues("main").Set(42.5)

	shim := newSDKShim("test", "vpp_test_*", nil, reg)
	ch := make(chan prometheus.Metric, 4)
	if err := shim.Collect(context.Background(), nil, ch); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	close(ch)

	var got float64
	for m := range ch {
		var pb dto.Metric
		if err := m.Write(&pb); err != nil {
			t.Fatalf("Write: %v", err)
		}
		got = pb.Gauge.GetValue()
	}
	if got != 42.5 {
		t.Fatalf("got %v, want 42.5", got)
	}
}

func TestSDKShim_GlobFiltersOut(t *testing.T) {
	t.Parallel()
	reg := telemetry.NewRegistry()
	in, _ := reg.RegisterCounter(telemetry.CounterOpts{Name: "vpp_match", Help: "h"})
	out, _ := reg.RegisterCounter(telemetry.CounterOpts{Name: "other_metric", Help: "h"})
	in.WithLabelValues().Inc()
	out.WithLabelValues().Inc()

	shim := newSDKShim("test", "vpp_*", nil, reg)
	ch := make(chan prometheus.Metric, 8)
	if err := shim.Collect(context.Background(), nil, ch); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	close(ch)

	count := 0
	for m := range ch {
		var pb dto.Metric
		if err := m.Write(&pb); err != nil {
			t.Fatalf("Write: %v", err)
		}
		desc := m.Desc().String()
		if !contains(desc, "vpp_match") {
			t.Errorf("glob leaked non-matching metric: %s", desc)
		}
		count++
	}
	if count != 1 {
		t.Fatalf("got %d metrics, want 1", count)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
