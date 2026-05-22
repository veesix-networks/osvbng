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
	"github.com/veesix-networks/osvbng/pkg/models/protocols/bgp"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

func init() {
	show.RegisterFactory(NewBGPIPv4UnicastSummaryAllHandler)
	telemetry.RegisterMetric[bgp.SummaryAFI](paths.ProtocolsBGPIPv4UnicastSummaryAll)
}

type BGPIPv4UnicastSummaryAllHandler struct {
	routing *routing.Component
}

func NewBGPIPv4UnicastSummaryAllHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPIPv4UnicastSummaryAllHandler{routing: deps.Routing}
}

func (h *BGPIPv4UnicastSummaryAllHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPAFISummaryAll("ipv4")
}

func (h *BGPIPv4UnicastSummaryAllHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPIPv4UnicastSummaryAll
}

func (h *BGPIPv4UnicastSummaryAllHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPIPv4UnicastSummaryAllHandler) Summary() string {
	return "Show BGP IPv4 unicast summary across all VRFs"
}

func (h *BGPIPv4UnicastSummaryAllHandler) Description() string {
	return "Display BGP IPv4 unicast summary for every VRF; backs Prometheus scrape of per-VRF BGP summary metrics."
}
