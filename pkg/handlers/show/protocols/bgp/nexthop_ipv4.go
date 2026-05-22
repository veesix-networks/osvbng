// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"context"
	"fmt"
	"strconv"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewBGPNexthopIPv4Handler)
}

type BGPNexthopIPv4Handler struct {
	routing *routing.Component
}

func NewBGPNexthopIPv4Handler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPNexthopIPv4Handler{routing: deps.Routing}
}

func (h *BGPNexthopIPv4Handler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	detail, _ := strconv.ParseBool(req.Options["detail"])
	return h.routing.GetBGPNexthop(req.Options["vrf"], "ipv4", detail)
}

func (h *BGPNexthopIPv4Handler) PathPattern() paths.Path {
	return paths.ProtocolsBGPNexthopIPv4
}

func (h *BGPNexthopIPv4Handler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPNexthopIPv4Handler) Summary() string {
	return "Show BGP IPv4 nexthop tracking"
}

func (h *BGPNexthopIPv4Handler) Description() string {
	return "Display BGP IPv4 nexthop tracking state; response forwarded as-is from FRR."
}

type BGPNexthopIPv4Options struct {
	VRF    string `query:"vrf" description:"VRF name; empty means the default routing table"`
	Detail bool   `query:"detail" description:"Return detailed per-nexthop information"`
}

func (h *BGPNexthopIPv4Handler) OptionsType() interface{} {
	return &BGPNexthopIPv4Options{}
}
