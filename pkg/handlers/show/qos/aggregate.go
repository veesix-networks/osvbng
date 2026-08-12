// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package qos

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

func init() {
	show.RegisterFactory(func(d *deps.ShowDeps) show.ShowHandler {
		return &AggregateHandler{deps: d}
	})
	telemetry.RegisterMetric[southbound.AggregateState](paths.QoSAggregate)
}

// AggregateHandler reports both tiers of the shaping hierarchy.
//
// A port and the S-VLANs beneath it are both keyed by the port's sw_if_index,
// so entries are distinguished by their level and tag range rather than by
// interface alone.
type AggregateHandler struct {
	deps *deps.ShowDeps
}

func (h *AggregateHandler) Collect(_ context.Context, _ *show.Request) (interface{}, error) {
	if h.deps.Southbound == nil {
		return nil, fmt.Errorf("southbound not available")
	}

	return h.deps.Southbound.DumpAggregates()
}

func (h *AggregateHandler) PathPattern() paths.Path {
	return paths.QoSAggregate
}

func (h *AggregateHandler) Dependencies() []paths.Path {
	return nil
}

func (h *AggregateHandler) Summary() string {
	return "Show QoS aggregate shapers"
}

func (h *AggregateHandler) Description() string {
	return "Display port and S-VLAN aggregate shapers, their active children, and why children are being held back."
}

func (h *AggregateHandler) SortKey() string {
	return "sw_if_index"
}
