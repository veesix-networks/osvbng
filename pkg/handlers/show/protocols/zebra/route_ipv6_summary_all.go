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
	show.RegisterFactory(NewZebraRouteIPv6SummaryAllHandler)
	telemetry.RegisterMetric[zebra.RouteSummaryAll](paths.ProtocolsZebraRouteIPv6SummaryAll)
}

type ZebraRouteIPv6SummaryAllHandler struct {
	routing *routing.Component
}

func NewZebraRouteIPv6SummaryAllHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &ZebraRouteIPv6SummaryAllHandler{routing: deps.Routing}
}

func (h *ZebraRouteIPv6SummaryAllHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetZebraRouteSummaryAll("ipv6")
}

func (h *ZebraRouteIPv6SummaryAllHandler) PathPattern() paths.Path {
	return paths.ProtocolsZebraRouteIPv6SummaryAll
}
func (h *ZebraRouteIPv6SummaryAllHandler) Dependencies() []paths.Path { return nil }
func (h *ZebraRouteIPv6SummaryAllHandler) Summary() string {
	return "Show zebra IPv6 route summary across all VRFs"
}
func (h *ZebraRouteIPv6SummaryAllHandler) Description() string {
	return "Display per-protocol counts of routes in zebra's IPv6 RIB/FIB for every VRF; iterates per-VRF because FRR's `vrf all summary` output is concatenated JSON without VRF identifiers."
}
