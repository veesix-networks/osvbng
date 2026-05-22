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
	show.RegisterFactory(NewBGPNeighborReceivedHandler)
}

type BGPNeighborReceivedHandler struct {
	routing *routing.Component
}

func NewBGPNeighborReceivedHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPNeighborReceivedHandler{routing: deps.Routing}
}

func (h *BGPNeighborReceivedHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	wildcards, err := paths.Extract(req.Path, paths.ProtocolsBGPNeighborReceived)
	if err != nil {
		return nil, fmt.Errorf("extract neighbor address: %w", err)
	}
	if len(wildcards) != 1 {
		return nil, fmt.Errorf("invalid path: expected 1 wildcard, got %d", len(wildcards))
	}
	return h.routing.GetBGPNeighborRoutes(req.Options["vrf"], wildcards[0], "received-routes")
}

func (h *BGPNeighborReceivedHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPNeighborReceived
}

func (h *BGPNeighborReceivedHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPNeighborReceivedHandler) Summary() string {
	return "Show BGP routes received from a neighbor"
}

func (h *BGPNeighborReceivedHandler) Description() string {
	return "Display the BGP routes received from one neighbor (by address); requires soft-reconfiguration inbound. Response forwarded as-is from FRR."
}

type BGPNeighborReceivedOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *BGPNeighborReceivedHandler) OptionsType() interface{} {
	return &BGPNeighborReceivedOptions{}
}
