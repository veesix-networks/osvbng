// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf

import (
	"context"
	"fmt"
	"strconv"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewOSPFNeighborHandler)
}

type OSPFNeighborHandler struct {
	routing *routing.Component
}

func NewOSPFNeighborHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPFNeighborHandler{routing: deps.Routing}
}

func (h *OSPFNeighborHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	wildcards, err := paths.Extract(req.Path, paths.ProtocolsOSPFNeighbor)
	if err != nil {
		return nil, fmt.Errorf("extract neighbor router-id: %w", err)
	}
	if len(wildcards) != 1 {
		return nil, fmt.Errorf("invalid path: expected 1 wildcard, got %d", len(wildcards))
	}
	detail, _ := strconv.ParseBool(req.Options["detail"])
	return h.routing.GetOSPFNeighbor(req.Options["vrf"], wildcards[0], detail)
}

func (h *OSPFNeighborHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPFNeighbor
}

func (h *OSPFNeighborHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPFNeighborHandler) Summary() string {
	return "Show a single OSPFv2 neighbor by router-id"
}

func (h *OSPFNeighborHandler) Description() string {
	return "Display the state of one OSPFv2 neighbor identified by router-id."
}

type OSPFNeighborOptions struct {
	VRF    string `query:"vrf" description:"VRF name; empty means the default routing table"`
	Detail bool   `query:"detail" description:"Return the detail variant of the response"`
}

func (h *OSPFNeighborHandler) OptionsType() interface{} {
	return &OSPFNeighborOptions{}
}
