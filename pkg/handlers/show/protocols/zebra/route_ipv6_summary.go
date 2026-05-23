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
	show.RegisterFactory(NewZebraRouteIPv6SummaryHandler)
	telemetry.RegisterMetric[zebra.RouteSummary](paths.ProtocolsZebraRouteIPv6Summary)
}

type ZebraRouteIPv6SummaryHandler struct {
	routing *routing.Component
}

func NewZebraRouteIPv6SummaryHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &ZebraRouteIPv6SummaryHandler{routing: deps.Routing}
}

func (h *ZebraRouteIPv6SummaryHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetZebraRouteSummary(req.Options["vrf"], "ipv6")
}

func (h *ZebraRouteIPv6SummaryHandler) PathPattern() paths.Path {
	return paths.ProtocolsZebraRouteIPv6Summary
}
func (h *ZebraRouteIPv6SummaryHandler) Dependencies() []paths.Path { return nil }
func (h *ZebraRouteIPv6SummaryHandler) Summary() string {
	return "Show zebra IPv6 route summary"
}
func (h *ZebraRouteIPv6SummaryHandler) Description() string {
	return "Display per-protocol counts of routes in zebra's IPv6 RIB/FIB; honors req.Options[\"vrf\"]. Backs Prometheus per-protocol gauges."
}

type ZebraRouteIPv6SummaryOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *ZebraRouteIPv6SummaryHandler) OptionsType() interface{} {
	return &ZebraRouteIPv6SummaryOptions{}
}
