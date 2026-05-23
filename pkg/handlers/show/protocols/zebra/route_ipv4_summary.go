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
	show.RegisterFactory(NewZebraRouteIPv4SummaryHandler)
	telemetry.RegisterMetric[zebra.RouteSummary](paths.ProtocolsZebraRouteIPv4Summary)
}

type ZebraRouteIPv4SummaryHandler struct {
	routing *routing.Component
}

func NewZebraRouteIPv4SummaryHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &ZebraRouteIPv4SummaryHandler{routing: deps.Routing}
}

func (h *ZebraRouteIPv4SummaryHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetZebraRouteSummary(req.Options["vrf"], "ipv4")
}

func (h *ZebraRouteIPv4SummaryHandler) PathPattern() paths.Path {
	return paths.ProtocolsZebraRouteIPv4Summary
}
func (h *ZebraRouteIPv4SummaryHandler) Dependencies() []paths.Path { return nil }
func (h *ZebraRouteIPv4SummaryHandler) Summary() string {
	return "Show zebra IPv4 route summary"
}
func (h *ZebraRouteIPv4SummaryHandler) Description() string {
	return "Display per-protocol counts of routes in zebra's IPv4 RIB/FIB; honors req.Options[\"vrf\"]. Backs Prometheus per-protocol gauges."
}

type ZebraRouteIPv4SummaryOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *ZebraRouteIPv4SummaryHandler) OptionsType() interface{} {
	return &ZebraRouteIPv4SummaryOptions{}
}
