// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

const (
	fnvOffset64 uint64 = 0xcbf29ce484222325
	fnvPrime64  uint64 = 0x100000001b3

	// labelDelimiter separates label values in the hash to disambiguate
	// tuples like ("ab","c") from ("a","bc"). 0xFF cannot appear in valid
	// UTF-8 so it is safe as an inter-value boundary marker.
	labelDelimiter byte = 0xFF
)

// hashLabelValues computes an FNV-1a hash over the supplied label values
// without allocating. Each string is hashed byte-by-byte; the delimiter
// separates values to disambiguate concatenated tuples.
func hashLabelValues(values []string) uint64 {
	h := fnvOffset64
	for i, v := range values {
		if i > 0 {
			h ^= uint64(labelDelimiter)
			h *= fnvPrime64
		}
		for j := 0; j < len(v); j++ {
			h ^= uint64(v[j])
			h *= fnvPrime64
		}
	}
	return h
}

// labelValuesEqual reports whether two label-value slices are byte-equal.
// Used to verify hash matches on lookup.
func labelValuesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
