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
	show.RegisterFactory(NewBGPVPNIPv4SummaryHandler)
	telemetry.RegisterMetric[bgp.VPNSummary](paths.ProtocolsBGPVPNIPv4Summary)
}

type BGPVPNIPv4SummaryHandler struct {
	routing *routing.Component
}

func NewBGPVPNIPv4SummaryHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPVPNIPv4SummaryHandler{routing: deps.Routing}
}

func (h *BGPVPNIPv4SummaryHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPVPNSummary(req.Options["vrf"], "ipv4")
}

func (h *BGPVPNIPv4SummaryHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVPNIPv4Summary
}

func (h *BGPVPNIPv4SummaryHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPVPNIPv4SummaryHandler) Summary() string {
	return "Show BGP VPNv4 summary"
}

func (h *BGPVPNIPv4SummaryHandler) Description() string {
	return "Display a summary of BGP VPNv4 unicast neighbor sessions."
}

type BGPVPNIPv4SummaryOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *BGPVPNIPv4SummaryHandler) OptionsType() interface{} {
	return &BGPVPNIPv4SummaryOptions{}
}
