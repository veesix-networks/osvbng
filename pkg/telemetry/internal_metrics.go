// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

const (
	internalMetricCardinalityDrops   = "osvbng_telemetry_cardinality_drops_total"
	internalMetricUnknownEmits       = "osvbng_telemetry_unknown_series_emits_total"
	internalMetricStaleEmits         = "osvbng_telemetry_stale_handle_emits_total"
	internalMetricRegistrationErrs   = "osvbng_telemetry_registration_errors_total"
	internalMetricMetricsTotal       = "osvbng_telemetry_metrics_total"
	internalMetricSeriesTotal        = "osvbng_telemetry_series_total"
	internalMetricSubscriptionsTotal = "osvbng_telemetry_subscriptions_total"
	internalMetricSubscriptionDrops  = "osvbng_telemetry_subscription_drops_total"
)

const internalLabelMetric = "metric"
const internalLabelReason = "reason"
const internalLabelSubscriber = "subscriber_id"

var internalRegistrationErrorLabels = []LabelPair{{Name: internalLabelReason, Value: "all"}}

// appendInternalSamples synthesizes registry-internal observability samples
// from the per-Counter atomic accounting fields. Called by AppendSnapshot
// after walking application metrics. The synthetic samples carry no
// allocations beyond the per-call LabelPair slices that other internal
// metrics share with application metrics.
func (r *Registry) appendInternalSamples(dst []Sample, opts SnapshotOptions) []Sample {
	if MatchGlob(opts.PathGlob, internalMetricMetricsTotal) {
		dst = append(dst, Sample{
			Name:  internalMetricMetricsTotal,
			Type:  MetricGauge,
			Value: float64(r.MetricCount()),
		})
	}

	r.metrics.Range(func(_, v any) bool {
		m, ok := v.(metric)
		if !ok {
			return true
		}

		labels := m.internalLabelsRef()

		if MatchGlob(opts.PathGlob, internalMetricSeriesTotal) {
			dst = append(dst, Sample{
				Name:   internalMetricSeriesTotal,
				Type:   MetricGauge,
				Labels: labels,
				Value:  float64(m.seriesCountLoad()),
			})
		}

		if d := m.cardinalityDropsLoad(); d > 0 && MatchGlob(opts.PathGlob, internalMetricCardinalityDrops) {
			dst = append(dst, Sample{
				Name:   internalMetricCardinalityDrops,
				Type:   MetricCounter,
				Labels: labels,
				Value:  float64(d),
			})
		}

		if e := m.unknownSeriesEmitsLoad(); e > 0 && MatchGlob(opts.PathGlob, internalMetricUnknownEmits) {
			dst = append(dst, Sample{
				Name:   internalMetricUnknownEmits,
				Type:   MetricCounter,
				Labels: labels,
				Value:  float64(e),
			})
		}

		if s := m.staleHandleEmitsLoad(); s > 0 && MatchGlob(opts.PathGlob, internalMetricStaleEmits) {
			dst = append(dst, Sample{
				Name:   internalMetricStaleEmits,
				Type:   MetricCounter,
				Labels: labels,
				Value:  float64(s),
			})
		}
		return true
	})

	if errs := r.registrationErrors.Load(); errs > 0 && MatchGlob(opts.PathGlob, internalMetricRegistrationErrs) {
		dst = append(dst, Sample{
			Name:   internalMetricRegistrationErrs,
			Type:   MetricCounter,
			Labels: internalRegistrationErrorLabels,
			Value:  float64(errs),
		})
	}

	if MatchGlob(opts.PathGlob, internalMetricSubscriptionsTotal) {
		dst = append(dst, Sample{
			Name:  internalMetricSubscriptionsTotal,
			Type:  MetricGauge,
			Value: float64(r.subscriberCount.Load()),
		})
	}

	if MatchGlob(opts.PathGlob, internalMetricSubscriptionDrops) {
		r.subscribers.Range(func(_, v any) bool {
			sub := v.(*Subscription)
			if d := sub.dropped.Load(); d > 0 {
				dst = append(dst, Sample{
					Name:   internalMetricSubscriptionDrops,
					Type:   MetricCounter,
					Labels: sub.internalLabels,
					Value:  float64(d),
				})
			}
			return true
		})
	}

	return dst
}

// InternalCounts is a convenience read of the registry's own observability
// counters. Useful for tests; production code should snapshot the metrics
// through AppendSnapshot.
type InternalCounts struct {
	CardinalityDrops   uint64
	UnknownSeriesEmits uint64
	StaleHandleEmits   uint64
	RegistrationErrors uint64
	MetricsTotal       int
	SeriesTotal        int64
}

// SnapshotInternal aggregates the per-metric internal counters into one
// summary. Useful for tests; allocates one InternalCounts.
func (r *Registry) SnapshotInternal() InternalCounts {
	out := InternalCounts{
		RegistrationErrors: r.registrationErrors.Load(),
		MetricsTotal:       r.MetricCount(),
		SeriesTotal:        r.SeriesCount(),
	}
	r.metrics.Range(func(_, v any) bool {
		m, ok := v.(metric)
		if !ok {
			return true
		}
		out.CardinalityDrops += m.cardinalityDropsLoad()
		out.UnknownSeriesEmits += m.unknownSeriesEmitsLoad()
		out.StaleHandleEmits += m.staleHandleEmitsLoad()
		return true
	})
	return out
}
