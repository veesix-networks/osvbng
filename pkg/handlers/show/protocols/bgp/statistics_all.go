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
	show.RegisterFactory(NewBGPStatisticsAllHandler)
	telemetry.RegisterMetric[bgp.Statistics](paths.ProtocolsBGPStatisticsAll)
}

type BGPStatisticsAllHandler struct {
	routing *routing.Component
}

func NewBGPStatisticsAllHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPStatisticsAllHandler{routing: deps.Routing}
}

func (h *BGPStatisticsAllHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPStatisticsAll(true)
}

func (h *BGPStatisticsAllHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPStatisticsAll
}

func (h *BGPStatisticsAllHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPStatisticsAllHandler) Summary() string {
	return "Show BGP IPv4 unicast statistics across all VRFs"
}

func (h *BGPStatisticsAllHandler) Description() string {
	return "Display BGP IPv4 unicast statistics for every VRF; response forwarded as-is from FRR."
}
