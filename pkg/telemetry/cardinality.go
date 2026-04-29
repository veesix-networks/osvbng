// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"fmt"
	"sync"
)

const DefaultMaxSeriesPerMetric = 10000

// defaultUnboundedLabels lists label names that produce unbounded cardinality
// in BNG deployments. Registration with any of these names is rejected
// unless the metric sets StreamingOnly=true (in which case the metric is
// excluded from the Prometheus-safe snapshot path by default).
var defaultUnboundedLabels = []string{
	"session_id", "subscriber_id", "session", "subscriber",
	"auth_session_id", "acct_session_id",
	"ip", "ipv4", "ipv6", "mac", "calling_station_id",
	"username", "hostname",
	"circuit_id", "remote_id", "agent_circuit_id", "agent_remote_id",
	"nas_port_id",
}

var (
	unboundedLabelsMu sync.RWMutex
	unboundedLabels   = newLabelSet(defaultUnboundedLabels)
)

func newLabelSet(labels []string) map[string]struct{} {
	m := make(map[string]struct{}, len(labels))
	for _, l := range labels {
		m[l] = struct{}{}
	}
	return m
}

// SetUnboundedLabels replaces the package-level unbounded label list. Call
// before any metric registration; existing registrations are not re-validated.
func SetUnboundedLabels(labels []string) {
	m := newLabelSet(labels)
	unboundedLabelsMu.Lock()
	unboundedLabels = m
	unboundedLabelsMu.Unlock()
}

// validateLabels returns nil if the declared label names are acceptable
// under the current cardinality policy. streamingOnly bypasses the check.
func validateLabels(labels []string, streamingOnly bool) error {
	for _, l := range labels {
		if l == "" {
			return fmt.Errorf("%w: empty label name", ErrInvalidLabel)
		}
	}
	if streamingOnly {
		return nil
	}
	unboundedLabelsMu.RLock()
	defer unboundedLabelsMu.RUnlock()
	for _, l := range labels {
		if _, isUnbounded := unboundedLabels[l]; isUnbounded {
			return fmt.Errorf("%w: %q", ErrUnboundedLabel, l)
		}
	}
	return nil
}
