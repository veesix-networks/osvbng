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
	show.RegisterFactory(NewBGPIPv6UnicastSelfOriginateHandler)
}

type BGPIPv6UnicastSelfOriginateHandler struct {
	routing *routing.Component
}

func NewBGPIPv6UnicastSelfOriginateHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPIPv6UnicastSelfOriginateHandler{routing: deps.Routing}
}

func (h *BGPIPv6UnicastSelfOriginateHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPRIB(req.Options["vrf"], "ipv6", "self-originate")
}

func (h *BGPIPv6UnicastSelfOriginateHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPIPv6UnicastSelfOriginate
}

func (h *BGPIPv6UnicastSelfOriginateHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPIPv6UnicastSelfOriginateHandler) Summary() string {
	return "Show BGP IPv6 unicast self-originated routes"
}

func (h *BGPIPv6UnicastSelfOriginateHandler) Description() string {
	return "Display BGP IPv6 unicast routes originated by this router; response forwarded as-is from FRR."
}

type BGPIPv6UnicastSelfOriginateOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *BGPIPv6UnicastSelfOriginateHandler) OptionsType() interface{} {
	return &BGPIPv6UnicastSelfOriginateOptions{}
}
