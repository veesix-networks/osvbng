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
	show.RegisterFactory(NewBGPIPv6StatisticsAllHandler)
	telemetry.RegisterMetric[bgp.Statistics](paths.ProtocolsBGPIPv6StatisticsAll)
}

type BGPIPv6StatisticsAllHandler struct {
	routing *routing.Component
}

func NewBGPIPv6StatisticsAllHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPIPv6StatisticsAllHandler{routing: deps.Routing}
}

func (h *BGPIPv6StatisticsAllHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPStatisticsAll(false)
}

func (h *BGPIPv6StatisticsAllHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPIPv6StatisticsAll
}

func (h *BGPIPv6StatisticsAllHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPIPv6StatisticsAllHandler) Summary() string {
	return "Show BGP IPv6 unicast statistics across all VRFs"
}

func (h *BGPIPv6StatisticsAllHandler) Description() string {
	return "Display BGP IPv6 unicast statistics for every VRF; response forwarded as-is from FRR."
}
