// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ipv6

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
	show.RegisterFactory(NewBGPVPNIPv6SummaryAllHandler)
	telemetry.RegisterMetric[bgp.VPNSummary](paths.ProtocolsBGPVPNIPv6SummaryAll)
}

type BGPVPNIPv6SummaryAllHandler struct {
	routing *routing.Component
}

func NewBGPVPNIPv6SummaryAllHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPVPNIPv6SummaryAllHandler{routing: deps.Routing}
}

func (h *BGPVPNIPv6SummaryAllHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPVPNSummaryAll("ipv6")
}

func (h *BGPVPNIPv6SummaryAllHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVPNIPv6SummaryAll
}

func (h *BGPVPNIPv6SummaryAllHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPVPNIPv6SummaryAllHandler) Summary() string {
	return "Show BGP VPNv6 summary across all VRFs"
}

func (h *BGPVPNIPv6SummaryAllHandler) Description() string {
	return "Display BGP VPNv6 summary for every VRF; backs Prometheus scrape of per-VRF BGP VPN summary metrics."
}
