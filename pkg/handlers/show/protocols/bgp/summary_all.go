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
	show.RegisterFactory(NewBGPSummaryAllHandler)
}

type BGPSummaryAllHandler struct {
	routing *routing.Component
}

func NewBGPSummaryAllHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPSummaryAllHandler{routing: deps.Routing}
}

func (h *BGPSummaryAllHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPSummaryAll()
}

func (h *BGPSummaryAllHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPSummaryAll
}

func (h *BGPSummaryAllHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPSummaryAllHandler) Summary() string {
	return "Show BGP summary across all VRFs and AFIs"
}

func (h *BGPSummaryAllHandler) Description() string {
	return "Display BGP summary for every VRF and configured AFI; response forwarded as-is from FRR."
}
