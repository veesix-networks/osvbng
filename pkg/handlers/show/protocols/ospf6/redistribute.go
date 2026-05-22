// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf6

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewOSPF6RedistributeHandler)
}

type OSPF6RedistributeHandler struct {
	routing *routing.Component
}

func NewOSPF6RedistributeHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPF6RedistributeHandler{routing: deps.Routing}
}

func (h *OSPF6RedistributeHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetOSPF6Redistribute(req.Options["vrf"])
}

func (h *OSPF6RedistributeHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPF6Redistribute
}

func (h *OSPF6RedistributeHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPF6RedistributeHandler) Summary() string {
	return "Show OSPFv3 redistribute configuration"
}

func (h *OSPF6RedistributeHandler) Description() string {
	return "Display OSPFv3 route-redistribute configuration; response forwarded as-is from FRR."
}

type OSPF6RedistributeOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *OSPF6RedistributeHandler) OptionsType() interface{} {
	return &OSPF6RedistributeOptions{}
}
