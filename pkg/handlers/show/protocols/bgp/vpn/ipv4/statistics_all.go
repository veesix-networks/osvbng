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
	show.RegisterFactory(NewBGPVPNIPv4StatisticsAllHandler)
	telemetry.RegisterMetric[bgp.VPNStatistics](paths.ProtocolsBGPVPNIPv4StatisticsAll)
}

type BGPVPNIPv4StatisticsAllHandler struct {
	routing *routing.Component
}

func NewBGPVPNIPv4StatisticsAllHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPVPNIPv4StatisticsAllHandler{routing: deps.Routing}
}

func (h *BGPVPNIPv4StatisticsAllHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPVPNStatisticsAll("ipv4")
}

func (h *BGPVPNIPv4StatisticsAllHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVPNIPv4StatisticsAll
}

func (h *BGPVPNIPv4StatisticsAllHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPVPNIPv4StatisticsAllHandler) Summary() string {
	return "Show BGP VPNv4 statistics across all VRFs"
}

func (h *BGPVPNIPv4StatisticsAllHandler) Description() string {
	return "Display BGP VPNv4 RIB statistics for every VRF instance; backs Prometheus scrape of per-VRF BGP VPN statistics."
}
