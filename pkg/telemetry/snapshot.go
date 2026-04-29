// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

// AppendSnapshot walks the registry's metrics, applies the supplied
// filtering options, and appends matching samples to dst. The caller owns
// dst; AppendSnapshot grows it only when capacity is insufficient. The
// returned slice is dst (possibly grown).
//
// Counter and gauge samples allocate nothing per series. Histogram samples
// allocate one BucketSample slice per series; callers that scrape large
// numbers of histogram series should pool that allocation outside the SDK.
//
// Registry-internal observability metrics (osvbng_telemetry_*) are appended
// after application metrics.
func (r *Registry) AppendSnapshot(dst []Sample, opts SnapshotOptions) []Sample {
	r.metrics.Range(func(_, v any) bool {
		m := v.(metric)
		if !opts.IncludeStreamingOnly && m.streamingOnly() {
			return true
		}
		if !MatchGlob(opts.PathGlob, m.name()) {
			return true
		}
		dst = m.appendSamples(dst)
		return true
	})
	dst = r.appendInternalSamples(dst, opts)
	return dst
}
