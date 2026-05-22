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
	show.RegisterFactory(NewBGPIPv6UnicastCIDROnlyHandler)
}

type BGPIPv6UnicastCIDROnlyHandler struct {
	routing *routing.Component
}

func NewBGPIPv6UnicastCIDROnlyHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPIPv6UnicastCIDROnlyHandler{routing: deps.Routing}
}

func (h *BGPIPv6UnicastCIDROnlyHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPRIB(req.Options["vrf"], "ipv6", "cidr-only")
}

func (h *BGPIPv6UnicastCIDROnlyHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPIPv6UnicastCIDROnly
}

func (h *BGPIPv6UnicastCIDROnlyHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPIPv6UnicastCIDROnlyHandler) Summary() string {
	return "Show BGP IPv6 unicast CIDR-only routes"
}

func (h *BGPIPv6UnicastCIDROnlyHandler) Description() string {
	return "Display BGP IPv6 unicast routes excluding classful prefixes; response forwarded as-is from FRR."
}

type BGPIPv6UnicastCIDROnlyOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *BGPIPv6UnicastCIDROnlyHandler) OptionsType() interface{} {
	return &BGPIPv6UnicastCIDROnlyOptions{}
}
