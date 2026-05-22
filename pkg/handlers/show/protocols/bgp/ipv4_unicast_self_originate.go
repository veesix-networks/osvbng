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
	show.RegisterFactory(NewBGPIPv4UnicastSelfOriginateHandler)
}

type BGPIPv4UnicastSelfOriginateHandler struct {
	routing *routing.Component
}

func NewBGPIPv4UnicastSelfOriginateHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPIPv4UnicastSelfOriginateHandler{routing: deps.Routing}
}

func (h *BGPIPv4UnicastSelfOriginateHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPRIB(req.Options["vrf"], "ipv4", "self-originate")
}

func (h *BGPIPv4UnicastSelfOriginateHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPIPv4UnicastSelfOriginate
}

func (h *BGPIPv4UnicastSelfOriginateHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPIPv4UnicastSelfOriginateHandler) Summary() string {
	return "Show BGP IPv4 unicast self-originated routes"
}

func (h *BGPIPv4UnicastSelfOriginateHandler) Description() string {
	return "Display BGP IPv4 unicast routes originated by this router; response forwarded as-is from FRR."
}

type BGPIPv4UnicastSelfOriginateOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *BGPIPv4UnicastSelfOriginateHandler) OptionsType() interface{} {
	return &BGPIPv4UnicastSelfOriginateOptions{}
}
