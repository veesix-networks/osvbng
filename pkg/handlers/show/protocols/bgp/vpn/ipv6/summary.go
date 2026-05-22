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
	show.RegisterFactory(NewBGPVPNIPv6SummaryHandler)
	telemetry.RegisterMetric[bgp.VPNSummary](paths.ProtocolsBGPVPNIPv6Summary)
}

type BGPVPNIPv6SummaryHandler struct {
	routing *routing.Component
}

func NewBGPVPNIPv6SummaryHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPVPNIPv6SummaryHandler{routing: deps.Routing}
}

func (h *BGPVPNIPv6SummaryHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPVPNSummary(req.Options["vrf"], "ipv6")
}

func (h *BGPVPNIPv6SummaryHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVPNIPv6Summary
}

func (h *BGPVPNIPv6SummaryHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPVPNIPv6SummaryHandler) Summary() string {
	return "Show BGP VPNv6 summary"
}

func (h *BGPVPNIPv6SummaryHandler) Description() string {
	return "Display a summary of BGP VPNv6 unicast neighbor sessions."
}

type BGPVPNIPv6SummaryOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *BGPVPNIPv6SummaryHandler) OptionsType() interface{} {
	return &BGPVPNIPv6SummaryOptions{}
}
