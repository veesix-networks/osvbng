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
	show.RegisterFactory(NewBGPNexthopIPv6Handler)
}

type BGPNexthopIPv6Handler struct {
	routing *routing.Component
}

func NewBGPNexthopIPv6Handler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPNexthopIPv6Handler{routing: deps.Routing}
}

func (h *BGPNexthopIPv6Handler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	detail, _ := strconv.ParseBool(req.Options["detail"])
	return h.routing.GetBGPNexthop(req.Options["vrf"], "ipv6", detail)
}

func (h *BGPNexthopIPv6Handler) PathPattern() paths.Path {
	return paths.ProtocolsBGPNexthopIPv6
}

func (h *BGPNexthopIPv6Handler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPNexthopIPv6Handler) Summary() string {
	return "Show BGP IPv6 nexthop tracking"
}

func (h *BGPNexthopIPv6Handler) Description() string {
	return "Display BGP IPv6 nexthop tracking state; response forwarded as-is from FRR."
}

type BGPNexthopIPv6Options struct {
	VRF    string `query:"vrf" description:"VRF name; empty means the default routing table"`
	Detail bool   `query:"detail" description:"Return detailed per-nexthop information"`
}

func (h *BGPNexthopIPv6Handler) OptionsType() interface{} {
	return &BGPNexthopIPv6Options{}
}
