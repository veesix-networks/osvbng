// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf6

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
	show.RegisterFactory(NewOSPF6RouteHandler)
}

type OSPF6RouteHandler struct {
	routing *routing.Component
}

func NewOSPF6RouteHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPF6RouteHandler{routing: deps.Routing}
}

func (h *OSPF6RouteHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	detail, _ := strconv.ParseBool(req.Options["detail"])
	match, _ := strconv.ParseBool(req.Options["match"])
	return h.routing.GetOSPF6Route(
		req.Options["vrf"],
		req.Options["filter"],
		req.Options["prefix"],
		detail, match,
	)
}

func (h *OSPF6RouteHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPF6Route
}

func (h *OSPF6RouteHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPF6RouteHandler) Summary() string {
	return "Show OSPFv3 routing table"
}

func (h *OSPF6RouteHandler) Description() string {
	return "Display the OSPFv3 SPF-computed routing table; filter accepts intra-area, inter-area, external-1, external-2, detail, summary."
}

type OSPF6RouteOptions struct {
	VRF    string `query:"vrf" description:"VRF name; empty means the default routing table"`
	Filter string `query:"filter" description:"intra-area | inter-area | external-1 | external-2 | detail | summary"`
	Prefix string `query:"prefix" description:"IPv6 prefix to look up"`
	Detail bool   `query:"detail" description:"Detailed output (alternate to filter=detail)"`
	Match  bool   `query:"match" description:"Treat prefix as a longest-match query"`
}

func (h *OSPF6RouteHandler) OptionsType() interface{} {
	return &OSPF6RouteOptions{}
}
