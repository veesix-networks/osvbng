// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ipv4

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
	show.RegisterFactory(NewBGPVPNIPv4SummaryAllHandler)
	telemetry.RegisterMetric[bgp.VPNSummary](paths.ProtocolsBGPVPNIPv4SummaryAll)
}

type BGPVPNIPv4SummaryAllHandler struct {
	routing *routing.Component
}

func NewBGPVPNIPv4SummaryAllHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPVPNIPv4SummaryAllHandler{routing: deps.Routing}
}

func (h *BGPVPNIPv4SummaryAllHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPVPNSummaryAll("ipv4")
}

func (h *BGPVPNIPv4SummaryAllHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVPNIPv4SummaryAll
}

func (h *BGPVPNIPv4SummaryAllHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPVPNIPv4SummaryAllHandler) Summary() string {
	return "Show BGP VPNv4 summary across all VRFs"
}

func (h *BGPVPNIPv4SummaryAllHandler) Description() string {
	return "Display BGP VPNv4 summary for every VRF; backs Prometheus scrape of per-VRF BGP VPN summary metrics."
}
