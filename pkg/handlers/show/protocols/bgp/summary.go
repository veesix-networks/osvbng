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
	show.RegisterFactory(NewBGPSummaryHandler)
}

type BGPSummaryHandler struct {
	routing *routing.Component
}

func NewBGPSummaryHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPSummaryHandler{routing: deps.Routing}
}

func (h *BGPSummaryHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPSummary(req.Options["vrf"])
}

func (h *BGPSummaryHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPSummary
}

func (h *BGPSummaryHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPSummaryHandler) Summary() string {
	return "Show BGP summary across configured AFIs"
}

func (h *BGPSummaryHandler) Description() string {
	return "Display BGP summary for all configured address families; response forwarded as-is from FRR."
}

type BGPSummaryOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table, 'all' returns every VRF"`
}

func (h *BGPSummaryHandler) OptionsType() interface{} {
	return &BGPSummaryOptions{}
}
