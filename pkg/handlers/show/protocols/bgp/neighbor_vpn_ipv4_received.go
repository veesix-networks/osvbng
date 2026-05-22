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
	show.RegisterFactory(NewBGPNeighborVPNIPv4ReceivedHandler)
}

type BGPNeighborVPNIPv4ReceivedHandler struct {
	routing *routing.Component
}

func NewBGPNeighborVPNIPv4ReceivedHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPNeighborVPNIPv4ReceivedHandler{routing: deps.Routing}
}

func (h *BGPNeighborVPNIPv4ReceivedHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	wildcards, err := paths.Extract(req.Path, paths.ProtocolsBGPNeighborVPNIPv4Received)
	if err != nil {
		return nil, fmt.Errorf("extract neighbor address: %w", err)
	}
	if len(wildcards) != 1 {
		return nil, fmt.Errorf("invalid path: expected 1 wildcard, got %d", len(wildcards))
	}
	return h.routing.GetBGPVPNNeighborRoutes(wildcards[0], "ipv4", "received-routes")
}

func (h *BGPNeighborVPNIPv4ReceivedHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPNeighborVPNIPv4Received
}

func (h *BGPNeighborVPNIPv4ReceivedHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPNeighborVPNIPv4ReceivedHandler) Summary() string {
	return "Show BGP VPNv4 routes received from a neighbor"
}

func (h *BGPNeighborVPNIPv4ReceivedHandler) Description() string {
	return "Display the BGP VPNv4 routes received from one neighbor (requires soft-reconfiguration inbound on the peer); response forwarded as-is from FRR."
}
