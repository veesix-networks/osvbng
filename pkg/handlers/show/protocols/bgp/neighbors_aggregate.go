// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/models/protocols/bgp"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

// BGPNeighborsAggregateHandler exposes all BGP neighbors at a single
// non-wildcard show path (`protocols.bgp.neighbors`) so the new
// telemetry poller can register metrics without needing config-backed
// wildcard expansion. The per-peer wildcard path
// (`protocols.bgp.neighbors.<*:ip>`) stays for CLI lookups (see
// neighbors.go) but is not a metric source.
type BGPNeighborsAggregateHandler struct {
	routing *routing.Component
}

func init() {
	show.RegisterFactory(NewBGPNeighborsAggregateHandler)
	telemetry.RegisterMetric[bgp.Neighbor](paths.ProtocolsBGPNeighborsAggregate)
}

func NewBGPNeighborsAggregateHandler(d *deps.ShowDeps) show.ShowHandler {
	return &BGPNeighborsAggregateHandler{routing: d.Routing}
}

func (h *BGPNeighborsAggregateHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPNeighbors()
}

func (h *BGPNeighborsAggregateHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPNeighborsAggregate
}

func (h *BGPNeighborsAggregateHandler) Dependencies() []paths.Path { return nil }

func (h *BGPNeighborsAggregateHandler) Summary() string {
	return "Show all BGP neighbors"
}

func (h *BGPNeighborsAggregateHandler) Description() string {
	return "Display every BGP neighbor session in a single response. Per-peer detail is available at `show protocols bgp neighbors <ip>`."
}
