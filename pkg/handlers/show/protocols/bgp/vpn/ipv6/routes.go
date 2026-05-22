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
	show.RegisterFactory(NewBGPVPNIPv6Handler)
	telemetry.RegisterMetric[bgp.VPNRoutes](paths.ProtocolsBGPVPNIPv6)
}

type BGPVPNIPv6Handler struct {
	routing *routing.Component
}

func NewBGPVPNIPv6Handler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPVPNIPv6Handler{routing: deps.Routing}
}

func (h *BGPVPNIPv6Handler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPVPNRoutes(req.Options["vrf"], "ipv6")
}

func (h *BGPVPNIPv6Handler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVPNIPv6
}

func (h *BGPVPNIPv6Handler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPVPNIPv6Handler) Summary() string {
	return "Show BGP VPNv6 routes"
}

func (h *BGPVPNIPv6Handler) Description() string {
	return "Display the BGP VPNv6 unicast routing table."
}

type BGPVPNIPv6Options struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *BGPVPNIPv6Handler) OptionsType() interface{} {
	return &BGPVPNIPv6Options{}
}
