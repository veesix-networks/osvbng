// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ldp

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewLDPNeighborCapabilitiesHandler)
}

type LDPNeighborCapabilitiesHandler struct {
	routing *routing.Component
}

func NewLDPNeighborCapabilitiesHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &LDPNeighborCapabilitiesHandler{routing: deps.Routing}
}

func (h *LDPNeighborCapabilitiesHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	wildcards, err := paths.Extract(req.Path, paths.ProtocolsLDPNeighborCapabilities)
	if err != nil {
		return nil, fmt.Errorf("extract LDP neighbor address: %w", err)
	}
	if len(wildcards) != 1 {
		return nil, fmt.Errorf("invalid path: expected 1 wildcard, got %d", len(wildcards))
	}
	return h.routing.GetLDPNeighborCapabilities(wildcards[0])
}

func (h *LDPNeighborCapabilitiesHandler) PathPattern() paths.Path {
	return paths.ProtocolsLDPNeighborCapabilities
}

func (h *LDPNeighborCapabilitiesHandler) Dependencies() []paths.Path {
	return nil
}

func (h *LDPNeighborCapabilitiesHandler) Summary() string {
	return "Show LDP capabilities for one neighbor"
}

func (h *LDPNeighborCapabilitiesHandler) Description() string {
	return "Display the LDP capability TLVs sent to and received from one LSR-ID."
}
