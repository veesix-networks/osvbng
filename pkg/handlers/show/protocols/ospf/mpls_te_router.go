// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewOSPFMPLSTERouterHandler)
}

type OSPFMPLSTERouterHandler struct {
	routing *routing.Component
}

func NewOSPFMPLSTERouterHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPFMPLSTERouterHandler{routing: deps.Routing}
}

func (h *OSPFMPLSTERouterHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetOSPFMPLSTERouter()
}

func (h *OSPFMPLSTERouterHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPFMPLSTERouter
}

func (h *OSPFMPLSTERouterHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPFMPLSTERouterHandler) Summary() string {
	return "Show OSPFv2 MPLS-TE router parameters"
}

func (h *OSPFMPLSTERouterHandler) Description() string {
	return "Display MPLS-TE router-level parameters. FRR returns plain text; the response is forwarded as-is."
}
