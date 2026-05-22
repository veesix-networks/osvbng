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
	show.RegisterFactory(NewBGPVPNIPv6StatisticsHandler)
	telemetry.RegisterMetric[bgp.VPNStatistics](paths.ProtocolsBGPVPNIPv6Statistics)
}

type BGPVPNIPv6StatisticsHandler struct {
	routing *routing.Component
}

func NewBGPVPNIPv6StatisticsHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPVPNIPv6StatisticsHandler{routing: deps.Routing}
}

func (h *BGPVPNIPv6StatisticsHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPVPNStatistics(req.Options["vrf"], "ipv6")
}

func (h *BGPVPNIPv6StatisticsHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVPNIPv6Statistics
}

func (h *BGPVPNIPv6StatisticsHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPVPNIPv6StatisticsHandler) Summary() string {
	return "Show BGP VPNv6 statistics"
}

func (h *BGPVPNIPv6StatisticsHandler) Description() string {
	return "Display BGP VPNv6 RIB statistics (advertisements, prefixes, AS-path metrics)."
}

type BGPVPNIPv6StatisticsOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *BGPVPNIPv6StatisticsHandler) OptionsType() interface{} {
	return &BGPVPNIPv6StatisticsOptions{}
}
