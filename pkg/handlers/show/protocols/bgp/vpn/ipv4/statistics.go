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
	show.RegisterFactory(NewBGPVPNIPv4StatisticsHandler)
	telemetry.RegisterMetric[bgp.VPNStatistics](paths.ProtocolsBGPVPNIPv4Statistics)
}

type BGPVPNIPv4StatisticsHandler struct {
	routing *routing.Component
}

func NewBGPVPNIPv4StatisticsHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPVPNIPv4StatisticsHandler{routing: deps.Routing}
}

func (h *BGPVPNIPv4StatisticsHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPVPNStatistics(req.Options["vrf"], "ipv4")
}

func (h *BGPVPNIPv4StatisticsHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVPNIPv4Statistics
}

func (h *BGPVPNIPv4StatisticsHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPVPNIPv4StatisticsHandler) Summary() string {
	return "Show BGP VPNv4 statistics"
}

func (h *BGPVPNIPv4StatisticsHandler) Description() string {
	return "Display BGP VPNv4 RIB statistics (advertisements, prefixes, AS-path metrics)."
}

type BGPVPNIPv4StatisticsOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *BGPVPNIPv4StatisticsHandler) OptionsType() interface{} {
	return &BGPVPNIPv4StatisticsOptions{}
}
