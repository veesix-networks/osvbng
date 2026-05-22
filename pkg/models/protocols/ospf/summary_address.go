// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf

type SummaryAddress struct {
	AggregationDelayInterval int                       `json:"aggregationDelayInterval"`
	Summaries                map[string]SummaryEntry   `json:"summaries,omitempty"`
}

type SummaryEntry struct {
	Tag      int    `json:"tag,omitempty"`
	Metric   int    `json:"metric,omitempty"`
	Type     string `json:"type,omitempty"`
	External int    `json:"external,omitempty"`
}
