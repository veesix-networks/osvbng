// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf6

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewOSPF6NeighborHandler)
}

type OSPF6NeighborHandler struct {
	routing *routing.Component
}

func NewOSPF6NeighborHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPF6NeighborHandler{routing: deps.Routing}
}

func (h *OSPF6NeighborHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	wildcards, err := paths.Extract(req.Path, paths.ProtocolsOSPF6Neighbor)
	if err != nil {
		return nil, fmt.Errorf("extract neighbor router-id: %w", err)
	}
	if len(wildcards) != 1 {
		return nil, fmt.Errorf("invalid path: expected 1 wildcard, got %d", len(wildcards))
	}
	return h.routing.GetOSPF6Neighbor(req.Options["vrf"], wildcards[0])
}

func (h *OSPF6NeighborHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPF6Neighbor
}

func (h *OSPF6NeighborHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPF6NeighborHandler) Summary() string {
	return "Show a single OSPFv3 neighbor by router-id"
}

func (h *OSPF6NeighborHandler) Description() string {
	return "Display detail for one OSPFv3 neighbor identified by router-id. FRR keys the response by router-id%interface."
}

type OSPF6NeighborOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *OSPF6NeighborHandler) OptionsType() interface{} {
	return &OSPF6NeighborOptions{}
}
