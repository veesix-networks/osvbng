// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package metrics

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

func TestSDKShim_TranslatesCounter(t *testing.T) {
	t.Parallel()
	reg := telemetry.NewRegistry()
	c, err := reg.RegisterCounter(telemetry.CounterOpts{
		Name:   "test.counter",
		Help:   "test counter",
		Labels: []string{"name"},
	})
	if err != nil {
		t.Fatalf("RegisterCounter: %v", err)
	}
	c.WithLabelValues("a").Add(7)
	c.WithLabelValues("b").Add(3)

	shim := newSDKShim(nil, reg)
	ch := make(chan prometheus.Metric, 16)
	if err := shim.Collect(context.Background(), nil, ch); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	close(ch)

	values := map[string]float64{}
	sawTranslatedDesc := false
	for m := range ch {
		desc := m.Desc().String()
		if !strings.Contains(desc, "osvbng_test_counter") {
			continue
		}
		sawTranslatedDesc = true
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
	if !sawTranslatedDesc {
		t.Fatalf("never saw a desc with translated Prom name osvbng_test_counter")
	}
	if values["a"] != 7 || values["b"] != 3 {
		t.Fatalf("got %v, want a=7 b=3", values)
	}
}

func TestSDKShim_TranslatesGauge(t *testing.T) {
	t.Parallel()
	reg := telemetry.NewRegistry()
	g, err := reg.RegisterGauge(telemetry.GaugeOpts{
		Name:   "test.gauge",
		Help:   "test gauge",
		Labels: []string{"pool"},
	})
	if err != nil {
		t.Fatalf("RegisterGauge: %v", err)
	}
	g.WithLabelValues("main").Set(42.5)

	shim := newSDKShim(nil, reg)
	ch := make(chan prometheus.Metric, 4)
	if err := shim.Collect(context.Background(), nil, ch); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	close(ch)

	got := math.NaN()
	for m := range ch {
		if !strings.Contains(m.Desc().String(), "osvbng_test_gauge") {
			continue
		}
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

func TestSDKShim_RendersOsvbngPrefix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		internal string
		want     string
	}{
		{"aaa.radius.auth.requests", "osvbng_aaa_radius_auth_requests"},
		{"dataplane.vpp.interface.rx_packets", "osvbng_dataplane_vpp_interface_rx_packets"},
		{"telemetry.cardinality_drops", "osvbng_telemetry_cardinality_drops"},
		{"osvbng_already_rendered", "osvbng_already_rendered"},
	}
	for _, c := range cases {
		got := internalToProm(c.internal)
		if got != c.want {
			t.Errorf("internalToProm(%q) = %q, want %q", c.internal, got, c.want)
		}
	}
}
