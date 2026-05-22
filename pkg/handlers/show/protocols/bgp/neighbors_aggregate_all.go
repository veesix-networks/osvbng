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
)

func init() {
	show.RegisterFactory(NewBGPNeighborsAggregateAllHandler)
}

type BGPNeighborsAggregateAllHandler struct {
	routing *routing.Component
}

func NewBGPNeighborsAggregateAllHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPNeighborsAggregateAllHandler{routing: deps.Routing}
}

func (h *BGPNeighborsAggregateAllHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPNeighborsAll()
}

func (h *BGPNeighborsAggregateAllHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPNeighborsAggregateAll
}

func (h *BGPNeighborsAggregateAllHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPNeighborsAggregateAllHandler) Summary() string {
	return "Show BGP neighbors across all VRFs"
}

func (h *BGPNeighborsAggregateAllHandler) Description() string {
	return "Display BGP neighbors for every VRF keyed by VRF name; response forwarded as-is from FRR."
}
