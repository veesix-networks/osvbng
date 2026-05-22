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
	show.RegisterFactory(NewBGPNeighborVPNIPv4RoutesHandler)
}

type BGPNeighborVPNIPv4RoutesHandler struct {
	routing *routing.Component
}

func NewBGPNeighborVPNIPv4RoutesHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPNeighborVPNIPv4RoutesHandler{routing: deps.Routing}
}

func (h *BGPNeighborVPNIPv4RoutesHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	wildcards, err := paths.Extract(req.Path, paths.ProtocolsBGPNeighborVPNIPv4Routes)
	if err != nil {
		return nil, fmt.Errorf("extract neighbor address: %w", err)
	}
	if len(wildcards) != 1 {
		return nil, fmt.Errorf("invalid path: expected 1 wildcard, got %d", len(wildcards))
	}
	return h.routing.GetBGPVPNNeighborRoutes(wildcards[0], "ipv4", "routes")
}

func (h *BGPNeighborVPNIPv4RoutesHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPNeighborVPNIPv4Routes
}

func (h *BGPNeighborVPNIPv4RoutesHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPNeighborVPNIPv4RoutesHandler) Summary() string {
	return "Show BGP VPNv4 routes learnt from a neighbor"
}

func (h *BGPNeighborVPNIPv4RoutesHandler) Description() string {
	return "Display BGP VPNv4 routes installed in the RIB from one neighbor (by address); response forwarded as-is from FRR."
}
