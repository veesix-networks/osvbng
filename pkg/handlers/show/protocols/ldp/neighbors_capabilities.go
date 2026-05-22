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
	show.RegisterFactory(NewLDPNeighborsCapabilitiesHandler)
}

type LDPNeighborsCapabilitiesHandler struct {
	routing *routing.Component
}

func NewLDPNeighborsCapabilitiesHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &LDPNeighborsCapabilitiesHandler{routing: deps.Routing}
}

func (h *LDPNeighborsCapabilitiesHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetLDPNeighborsCapabilities()
}

func (h *LDPNeighborsCapabilitiesHandler) PathPattern() paths.Path {
	return paths.ProtocolsLDPNeighborsCapabilities
}

func (h *LDPNeighborsCapabilitiesHandler) Dependencies() []paths.Path {
	return nil
}

func (h *LDPNeighborsCapabilitiesHandler) Summary() string {
	return "Show LDP capabilities for every neighbor"
}

func (h *LDPNeighborsCapabilitiesHandler) Description() string {
	return "Display the LDP capability TLVs sent and received per neighbor LSR-ID."
}
