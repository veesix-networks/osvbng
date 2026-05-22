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
	show.RegisterFactory(NewBGPIPv4UnicastHandler)
}

type BGPIPv4UnicastHandler struct {
	routing *routing.Component
}

func NewBGPIPv4UnicastHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPIPv4UnicastHandler{routing: deps.Routing}
}

func (h *BGPIPv4UnicastHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPRIB(req.Options["vrf"], "ipv4", "")
}

func (h *BGPIPv4UnicastHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPIPv4Unicast
}

func (h *BGPIPv4UnicastHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPIPv4UnicastHandler) Summary() string {
	return "Show BGP IPv4 unicast RIB"
}

func (h *BGPIPv4UnicastHandler) Description() string {
	return "Display BGP IPv4 unicast RIB; response forwarded as-is from FRR."
}

type BGPIPv4UnicastOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *BGPIPv4UnicastHandler) OptionsType() interface{} {
	return &BGPIPv4UnicastOptions{}
}
