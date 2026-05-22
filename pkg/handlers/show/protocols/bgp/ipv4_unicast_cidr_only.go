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
)

func init() {
	show.RegisterFactory(NewBGPIPv4UnicastCIDROnlyHandler)
}

type BGPIPv4UnicastCIDROnlyHandler struct {
	routing *routing.Component
}

func NewBGPIPv4UnicastCIDROnlyHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPIPv4UnicastCIDROnlyHandler{routing: deps.Routing}
}

func (h *BGPIPv4UnicastCIDROnlyHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPRIB(req.Options["vrf"], "ipv4", "cidr-only")
}

func (h *BGPIPv4UnicastCIDROnlyHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPIPv4UnicastCIDROnly
}

func (h *BGPIPv4UnicastCIDROnlyHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPIPv4UnicastCIDROnlyHandler) Summary() string {
	return "Show BGP IPv4 unicast CIDR-only routes"
}

func (h *BGPIPv4UnicastCIDROnlyHandler) Description() string {
	return "Display BGP IPv4 unicast routes excluding classful prefixes; response forwarded as-is from FRR."
}

type BGPIPv4UnicastCIDROnlyOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *BGPIPv4UnicastCIDROnlyHandler) OptionsType() interface{} {
	return &BGPIPv4UnicastCIDROnlyOptions{}
}
