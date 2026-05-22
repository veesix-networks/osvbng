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
	show.RegisterFactory(NewBGPIPv6UnicastHandler)
}

type BGPIPv6UnicastHandler struct {
	routing *routing.Component
}

func NewBGPIPv6UnicastHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPIPv6UnicastHandler{routing: deps.Routing}
}

func (h *BGPIPv6UnicastHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPRIB(req.Options["vrf"], "ipv6", "")
}

func (h *BGPIPv6UnicastHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPIPv6Unicast
}

func (h *BGPIPv6UnicastHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPIPv6UnicastHandler) Summary() string {
	return "Show BGP IPv6 unicast RIB"
}

func (h *BGPIPv6UnicastHandler) Description() string {
	return "Display BGP IPv6 unicast RIB; response forwarded as-is from FRR."
}

type BGPIPv6UnicastOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *BGPIPv6UnicastHandler) OptionsType() interface{} {
	return &BGPIPv6UnicastOptions{}
}
