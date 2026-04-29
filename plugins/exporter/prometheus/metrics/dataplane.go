// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package metrics

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

const dataplaneGlob = "vpp_*"

func init() {
	Register("dataplane.vpp", func(log *logger.Logger) (MetricHandler, error) {
		return newSDKShim("dataplane.vpp", dataplaneGlob, log, telemetry.Default()), nil
	})
}

// sdkShim translates samples from the telemetry SDK registry into
// prometheus.Metric values. One shim covers every metric matching its glob;
// per-name prometheus.Desc objects are cached on first observation. This is
// transitional code: the full Prometheus exporter rewrite (osvbng-context #52)
// replaces the entire prometheusCollector with an SDK-backed implementation.
type sdkShim struct {
	name     string
	glob     string
	log      *logger.Logger
	registry *telemetry.Registry

	mu    sync.RWMutex
	descs map[string]*prometheus.Desc

	scratch []telemetry.Sample
}

func newSDKShim(name, glob string, log *logger.Logger, reg *telemetry.Registry) *sdkShim {
	return &sdkShim{
		name:     name,
		glob:     glob,
		log:      log,
		registry: reg,
		descs:    make(map[string]*prometheus.Desc),
	}
}

func (s *sdkShim) Name() string    { return s.name }
func (s *sdkShim) Paths() []string { return []string{s.glob} }
func (s *sdkShim) Describe(_ chan<- *prometheus.Desc) {
	// Descs are constructed lazily on first Collect (unchecked collector).
}

func (s *sdkShim) Collect(_ context.Context, _ cache.Cache, ch chan<- prometheus.Metric) error {
	s.scratch = s.registry.AppendSnapshot(s.scratch[:0], telemetry.SnapshotOptions{
		PathGlob: s.glob,
	})
	for i := range s.scratch {
		s.emit(&s.scratch[i], ch)
	}
	return nil
}

func (s *sdkShim) emit(sample *telemetry.Sample, ch chan<- prometheus.Metric) {
	desc := s.descFor(sample)

	switch sample.Type {
	case telemetry.MetricCounter:
		ch <- prometheus.MustNewConstMetric(desc, prometheus.CounterValue, sample.Value, labelValuesOf(sample.Labels)...)
	case telemetry.MetricGauge:
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, sample.Value, labelValuesOf(sample.Labels)...)
	case telemetry.MetricHistogram:
		ch <- prometheus.MustNewConstHistogram(desc,
			sample.Histogram.Count,
			sample.Histogram.Sum,
			bucketsToProm(sample.Histogram.Buckets),
			labelValuesOf(sample.Labels)...)
	}
}

func (s *sdkShim) descFor(sample *telemetry.Sample) *prometheus.Desc {
	key := descKey(sample.Name, sample.Labels)
	s.mu.RLock()
	d, ok := s.descs[key]
	s.mu.RUnlock()
	if ok {
		return d
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.descs[key]; ok {
		return d
	}
	d = prometheus.NewDesc(sample.Name, sample.Name, labelNamesOf(sample.Labels), nil)
	s.descs[key] = d
	return d
}

func descKey(name string, labels []telemetry.LabelPair) string {
	if len(labels) == 0 {
		return name
	}
	buf := make([]byte, 0, len(name)+len(labels)*16)
	buf = append(buf, name...)
	for _, l := range labels {
		buf = append(buf, '|')
		buf = append(buf, l.Name...)
	}
	return string(buf)
}

func labelNamesOf(labels []telemetry.LabelPair) []string {
	if len(labels) == 0 {
		return nil
	}
	out := make([]string, len(labels))
	for i, l := range labels {
		out[i] = l.Name
	}
	return out
}

func labelValuesOf(labels []telemetry.LabelPair) []string {
	if len(labels) == 0 {
		return nil
	}
	out := make([]string, len(labels))
	for i, l := range labels {
		out[i] = l.Value
	}
	return out
}

func bucketsToProm(buckets []telemetry.BucketSample) map[float64]uint64 {
	if len(buckets) == 0 {
		return nil
	}
	out := make(map[float64]uint64, len(buckets))
	for _, b := range buckets {
		out[b.UpperBound] = b.Count
	}
	return out
}
