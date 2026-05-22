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
	show.RegisterFactory(NewLDPNeighborDetailHandler)
}

type LDPNeighborDetailHandler struct {
	routing *routing.Component
}

func NewLDPNeighborDetailHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &LDPNeighborDetailHandler{routing: deps.Routing}
}

func (h *LDPNeighborDetailHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	wildcards, err := paths.Extract(req.Path, paths.ProtocolsLDPNeighborDetail)
	if err != nil {
		return nil, fmt.Errorf("extract LDP neighbor address: %w", err)
	}
	if len(wildcards) != 1 {
		return nil, fmt.Errorf("invalid path: expected 1 wildcard, got %d", len(wildcards))
	}
	return h.routing.GetLDPNeighborDetail(wildcards[0])
}

func (h *LDPNeighborDetailHandler) PathPattern() paths.Path {
	return paths.ProtocolsLDPNeighborDetail
}

func (h *LDPNeighborDetailHandler) Dependencies() []paths.Path {
	return nil
}

func (h *LDPNeighborDetailHandler) Summary() string {
	return "Show detailed LDP neighbor state"
}

func (h *LDPNeighborDetailHandler) Description() string {
	return "Display detailed LDP neighbor state for one LSR-ID: TCP endpoints, timers, sent/received message counts, and peer address list."
}
