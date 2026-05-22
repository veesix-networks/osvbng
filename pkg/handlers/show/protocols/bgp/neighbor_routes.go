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
	show.RegisterFactory(NewBGPNeighborRoutesHandler)
}

type BGPNeighborRoutesHandler struct {
	routing *routing.Component
}

func NewBGPNeighborRoutesHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPNeighborRoutesHandler{routing: deps.Routing}
}

func (h *BGPNeighborRoutesHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	wildcards, err := paths.Extract(req.Path, paths.ProtocolsBGPNeighborRoutes)
	if err != nil {
		return nil, fmt.Errorf("extract neighbor address: %w", err)
	}
	if len(wildcards) != 1 {
		return nil, fmt.Errorf("invalid path: expected 1 wildcard, got %d", len(wildcards))
	}
	return h.routing.GetBGPNeighborRoutes(req.Options["vrf"], wildcards[0], "routes")
}

func (h *BGPNeighborRoutesHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPNeighborRoutes
}

func (h *BGPNeighborRoutesHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPNeighborRoutesHandler) Summary() string {
	return "Show BGP routes learnt from a neighbor"
}

func (h *BGPNeighborRoutesHandler) Description() string {
	return "Display BGP routes installed in the RIB from one neighbor (by address); response forwarded as-is from FRR."
}

type BGPNeighborRoutesOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *BGPNeighborRoutesHandler) OptionsType() interface{} {
	return &BGPNeighborRoutesOptions{}
}
