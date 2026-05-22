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
	show.RegisterFactory(NewBGPIPv6UnicastSummaryAllHandler)
	telemetry.RegisterMetric[bgp.SummaryAFI](paths.ProtocolsBGPIPv6UnicastSummaryAll)
}

type BGPIPv6UnicastSummaryAllHandler struct {
	routing *routing.Component
}

func NewBGPIPv6UnicastSummaryAllHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPIPv6UnicastSummaryAllHandler{routing: deps.Routing}
}

func (h *BGPIPv6UnicastSummaryAllHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPAFISummaryAll("ipv6")
}

func (h *BGPIPv6UnicastSummaryAllHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPIPv6UnicastSummaryAll
}

func (h *BGPIPv6UnicastSummaryAllHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPIPv6UnicastSummaryAllHandler) Summary() string {
	return "Show BGP IPv6 unicast summary across all VRFs"
}

func (h *BGPIPv6UnicastSummaryAllHandler) Description() string {
	return "Display BGP IPv6 unicast summary for every VRF; backs Prometheus scrape of per-VRF BGP summary metrics."
}
