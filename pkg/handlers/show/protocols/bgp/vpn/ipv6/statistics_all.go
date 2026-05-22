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
	show.RegisterFactory(NewBGPVPNIPv6StatisticsAllHandler)
	telemetry.RegisterMetric[bgp.VPNStatistics](paths.ProtocolsBGPVPNIPv6StatisticsAll)
}

type BGPVPNIPv6StatisticsAllHandler struct {
	routing *routing.Component
}

func NewBGPVPNIPv6StatisticsAllHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPVPNIPv6StatisticsAllHandler{routing: deps.Routing}
}

func (h *BGPVPNIPv6StatisticsAllHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPVPNStatisticsAll("ipv6")
}

func (h *BGPVPNIPv6StatisticsAllHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVPNIPv6StatisticsAll
}

func (h *BGPVPNIPv6StatisticsAllHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPVPNIPv6StatisticsAllHandler) Summary() string {
	return "Show BGP VPNv6 statistics across all VRFs"
}

func (h *BGPVPNIPv6StatisticsAllHandler) Description() string {
	return "Display BGP VPNv6 RIB statistics for every VRF instance; backs Prometheus scrape of per-VRF BGP VPN statistics."
}
