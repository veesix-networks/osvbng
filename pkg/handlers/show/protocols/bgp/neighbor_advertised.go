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
	show.RegisterFactory(NewBGPNeighborAdvertisedHandler)
}

type BGPNeighborAdvertisedHandler struct {
	routing *routing.Component
}

func NewBGPNeighborAdvertisedHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPNeighborAdvertisedHandler{routing: deps.Routing}
}

func (h *BGPNeighborAdvertisedHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	wildcards, err := paths.Extract(req.Path, paths.ProtocolsBGPNeighborAdvertised)
	if err != nil {
		return nil, fmt.Errorf("extract neighbor address: %w", err)
	}
	if len(wildcards) != 1 {
		return nil, fmt.Errorf("invalid path: expected 1 wildcard, got %d", len(wildcards))
	}
	return h.routing.GetBGPNeighborRoutes(req.Options["vrf"], wildcards[0], "advertised-routes")
}

func (h *BGPNeighborAdvertisedHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPNeighborAdvertised
}

func (h *BGPNeighborAdvertisedHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPNeighborAdvertisedHandler) Summary() string {
	return "Show BGP routes advertised to a neighbor"
}

func (h *BGPNeighborAdvertisedHandler) Description() string {
	return "Display the BGP routes advertised to one neighbor (by address); response forwarded as-is from FRR."
}

type BGPNeighborAdvertisedOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *BGPNeighborAdvertisedHandler) OptionsType() interface{} {
	return &BGPNeighborAdvertisedOptions{}
}
