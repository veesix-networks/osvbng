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
	show.RegisterFactory(NewBGPNeighborVPNIPv6RoutesHandler)
}

type BGPNeighborVPNIPv6RoutesHandler struct {
	routing *routing.Component
}

func NewBGPNeighborVPNIPv6RoutesHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPNeighborVPNIPv6RoutesHandler{routing: deps.Routing}
}

func (h *BGPNeighborVPNIPv6RoutesHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	wildcards, err := paths.Extract(req.Path, paths.ProtocolsBGPNeighborVPNIPv6Routes)
	if err != nil {
		return nil, fmt.Errorf("extract neighbor address: %w", err)
	}
	if len(wildcards) != 1 {
		return nil, fmt.Errorf("invalid path: expected 1 wildcard, got %d", len(wildcards))
	}
	return h.routing.GetBGPVPNNeighborRoutes(wildcards[0], "ipv6", "routes")
}

func (h *BGPNeighborVPNIPv6RoutesHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPNeighborVPNIPv6Routes
}

func (h *BGPNeighborVPNIPv6RoutesHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPNeighborVPNIPv6RoutesHandler) Summary() string {
	return "Show BGP VPNv6 routes learnt from a neighbor"
}

func (h *BGPNeighborVPNIPv6RoutesHandler) Description() string {
	return "Display BGP VPNv6 routes installed in the RIB from one neighbor (by address); response forwarded as-is from FRR."
}
