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
	show.RegisterFactory(NewLDPCapabilitiesHandler)
}

type LDPCapabilitiesHandler struct {
	routing *routing.Component
}

func NewLDPCapabilitiesHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &LDPCapabilitiesHandler{routing: deps.Routing}
}

func (h *LDPCapabilitiesHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetLDPCapabilities()
}

func (h *LDPCapabilitiesHandler) PathPattern() paths.Path {
	return paths.ProtocolsLDPCapabilities
}

func (h *LDPCapabilitiesHandler) Dependencies() []paths.Path {
	return nil
}

func (h *LDPCapabilitiesHandler) Summary() string {
	return "Show LSR-wide LDP capabilities"
}

func (h *LDPCapabilitiesHandler) Description() string {
	return "Display the LDP capability TLVs advertised by this LSR."
}
