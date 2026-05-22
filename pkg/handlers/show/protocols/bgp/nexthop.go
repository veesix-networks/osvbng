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
	show.RegisterFactory(NewBGPNexthopHandler)
}

type BGPNexthopHandler struct {
	routing *routing.Component
}

func NewBGPNexthopHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPNexthopHandler{routing: deps.Routing}
}

func (h *BGPNexthopHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	detail, _ := strconv.ParseBool(req.Options["detail"])
	return h.routing.GetBGPNexthop(req.Options["vrf"], "", detail)
}

func (h *BGPNexthopHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPNexthop
}

func (h *BGPNexthopHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPNexthopHandler) Summary() string {
	return "Show BGP nexthop tracking"
}

func (h *BGPNexthopHandler) Description() string {
	return "Display BGP nexthop tracking state across both AFIs; response forwarded as-is from FRR."
}

type BGPNexthopOptions struct {
	VRF    string `query:"vrf" description:"VRF name; empty means the default routing table"`
	Detail bool   `query:"detail" description:"Return detailed per-nexthop information"`
}

func (h *BGPNexthopHandler) OptionsType() interface{} {
	return &BGPNexthopOptions{}
}
