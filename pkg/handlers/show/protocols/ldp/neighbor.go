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
	show.RegisterFactory(NewLDPNeighborHandler)
}

type LDPNeighborHandler struct {
	routing *routing.Component
}

func NewLDPNeighborHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &LDPNeighborHandler{routing: deps.Routing}
}

func (h *LDPNeighborHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	wildcards, err := paths.Extract(req.Path, paths.ProtocolsLDPNeighbor)
	if err != nil {
		return nil, fmt.Errorf("extract LDP neighbor address: %w", err)
	}
	if len(wildcards) != 1 {
		return nil, fmt.Errorf("invalid path: expected 1 wildcard, got %d", len(wildcards))
	}
	return h.routing.GetLDPNeighbor(wildcards[0])
}

func (h *LDPNeighborHandler) PathPattern() paths.Path {
	return paths.ProtocolsLDPNeighbor
}

func (h *LDPNeighborHandler) Dependencies() []paths.Path {
	return nil
}

func (h *LDPNeighborHandler) Summary() string {
	return "Show one LDP neighbor"
}

func (h *LDPNeighborHandler) Description() string {
	return "Display LDP neighbor session state for one LSR-ID (by address)."
}
