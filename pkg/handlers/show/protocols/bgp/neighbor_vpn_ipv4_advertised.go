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
	show.RegisterFactory(NewBGPNeighborVPNIPv4AdvertisedHandler)
}

type BGPNeighborVPNIPv4AdvertisedHandler struct {
	routing *routing.Component
}

func NewBGPNeighborVPNIPv4AdvertisedHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPNeighborVPNIPv4AdvertisedHandler{routing: deps.Routing}
}

func (h *BGPNeighborVPNIPv4AdvertisedHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	wildcards, err := paths.Extract(req.Path, paths.ProtocolsBGPNeighborVPNIPv4Advertised)
	if err != nil {
		return nil, fmt.Errorf("extract neighbor address: %w", err)
	}
	if len(wildcards) != 1 {
		return nil, fmt.Errorf("invalid path: expected 1 wildcard, got %d", len(wildcards))
	}
	return h.routing.GetBGPVPNNeighborRoutes(wildcards[0], "ipv4", "advertised-routes")
}

func (h *BGPNeighborVPNIPv4AdvertisedHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPNeighborVPNIPv4Advertised
}

func (h *BGPNeighborVPNIPv4AdvertisedHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPNeighborVPNIPv4AdvertisedHandler) Summary() string {
	return "Show BGP VPNv4 routes advertised to a neighbor"
}

func (h *BGPNeighborVPNIPv4AdvertisedHandler) Description() string {
	return "Display the BGP VPNv4 routes advertised to one neighbor; response forwarded as-is from FRR."
}
