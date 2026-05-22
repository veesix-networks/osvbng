// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf

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
	show.RegisterFactory(NewOSPFSegmentRoutingHandler)
}

type OSPFSegmentRoutingHandler struct {
	routing *routing.Component
}

func NewOSPFSegmentRoutingHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPFSegmentRoutingHandler{routing: deps.Routing}
}

func (h *OSPFSegmentRoutingHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	selfOriginate, _ := strconv.ParseBool(req.Options["self_originate"])
	return h.routing.GetOSPFSegmentRouting(req.Options["adv_router"], selfOriginate)
}

func (h *OSPFSegmentRoutingHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPFSegmentRouting
}

func (h *OSPFSegmentRoutingHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPFSegmentRoutingHandler) Summary() string {
	return "Show OSPFv2 segment-routing database"
}

func (h *OSPFSegmentRoutingHandler) Description() string {
	return "Display the OSPFv2 segment-routing database for all SR nodes, one advertising router, or self-originated entries. FRR returns plain text when segment-routing is disabled and JSON otherwise; the response is forwarded as-is."
}

type OSPFSegmentRoutingOptions struct {
	AdvRouter     string `query:"adv_router" description:"Filter by advertising router (IPv4)"`
	SelfOriginate bool   `query:"self_originate" description:"Restrict to self-originated SR entries"`
}

func (h *OSPFSegmentRoutingHandler) OptionsType() interface{} {
	return &OSPFSegmentRoutingOptions{}
}
