// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package zebra

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/models/protocols/zebra"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

func init() {
	show.RegisterFactory(NewZebraRouteIPv4SummaryAllHandler)
	telemetry.RegisterMetric[zebra.RouteSummaryAll](paths.ProtocolsZebraRouteIPv4SummaryAll)
}

type ZebraRouteIPv4SummaryAllHandler struct {
	routing *routing.Component
}

func NewZebraRouteIPv4SummaryAllHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &ZebraRouteIPv4SummaryAllHandler{routing: deps.Routing}
}

func (h *ZebraRouteIPv4SummaryAllHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetZebraRouteSummaryAll("ipv4")
}

func (h *ZebraRouteIPv4SummaryAllHandler) PathPattern() paths.Path {
	return paths.ProtocolsZebraRouteIPv4SummaryAll
}
func (h *ZebraRouteIPv4SummaryAllHandler) Dependencies() []paths.Path { return nil }
func (h *ZebraRouteIPv4SummaryAllHandler) Summary() string {
	return "Show zebra IPv4 route summary across all VRFs"
}
func (h *ZebraRouteIPv4SummaryAllHandler) Description() string {
	return "Display per-protocol counts of routes in zebra's IPv4 RIB/FIB for every VRF; iterates per-VRF because FRR's `vrf all summary` output is concatenated JSON without VRF identifiers."
}
