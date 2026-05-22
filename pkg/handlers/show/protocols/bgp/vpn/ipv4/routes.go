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
	show.RegisterFactory(NewBGPVPNIPv4Handler)
	telemetry.RegisterMetric[bgp.VPNRoutes](paths.ProtocolsBGPVPNIPv4)
}

type BGPVPNIPv4Handler struct {
	routing *routing.Component
}

func NewBGPVPNIPv4Handler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPVPNIPv4Handler{routing: deps.Routing}
}

func (h *BGPVPNIPv4Handler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPVPNRoutes(req.Options["vrf"], "ipv4")
}

func (h *BGPVPNIPv4Handler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVPNIPv4
}

func (h *BGPVPNIPv4Handler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPVPNIPv4Handler) Summary() string {
	return "Show BGP VPNv4 routes"
}

func (h *BGPVPNIPv4Handler) Description() string {
	return "Display the BGP VPNv4 unicast routing table."
}

type BGPVPNIPv4Options struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *BGPVPNIPv4Handler) OptionsType() interface{} {
	return &BGPVPNIPv4Options{}
}
