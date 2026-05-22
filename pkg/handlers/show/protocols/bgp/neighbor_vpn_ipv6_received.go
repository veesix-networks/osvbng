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
	show.RegisterFactory(NewBGPNeighborVPNIPv6ReceivedHandler)
}

type BGPNeighborVPNIPv6ReceivedHandler struct {
	routing *routing.Component
}

func NewBGPNeighborVPNIPv6ReceivedHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPNeighborVPNIPv6ReceivedHandler{routing: deps.Routing}
}

func (h *BGPNeighborVPNIPv6ReceivedHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	wildcards, err := paths.Extract(req.Path, paths.ProtocolsBGPNeighborVPNIPv6Received)
	if err != nil {
		return nil, fmt.Errorf("extract neighbor address: %w", err)
	}
	if len(wildcards) != 1 {
		return nil, fmt.Errorf("invalid path: expected 1 wildcard, got %d", len(wildcards))
	}
	return h.routing.GetBGPVPNNeighborRoutes(wildcards[0], "ipv6", "received-routes")
}

func (h *BGPNeighborVPNIPv6ReceivedHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPNeighborVPNIPv6Received
}

func (h *BGPNeighborVPNIPv6ReceivedHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPNeighborVPNIPv6ReceivedHandler) Summary() string {
	return "Show BGP VPNv6 routes received from a neighbor"
}

func (h *BGPNeighborVPNIPv6ReceivedHandler) Description() string {
	return "Display the BGP VPNv6 routes received from one neighbor (requires soft-reconfiguration inbound on the peer); response forwarded as-is from FRR."
}
