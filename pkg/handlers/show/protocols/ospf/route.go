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
	show.RegisterFactory(NewOSPFRouteHandler)
}

type OSPFRouteHandler struct {
	routing *routing.Component
}

func NewOSPFRouteHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPFRouteHandler{routing: deps.Routing}
}

func (h *OSPFRouteHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	detail, _ := strconv.ParseBool(req.Options["detail"])
	return h.routing.GetOSPFRoute(detail)
}

func (h *OSPFRouteHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPFRoute
}

func (h *OSPFRouteHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPFRouteHandler) Summary() string {
	return "Show OSPFv2 routing table"
}

func (h *OSPFRouteHandler) Description() string {
	return "Display the OSPFv2 SPF-computed routing table keyed by destination prefix."
}

type OSPFRouteOptions struct {
	Detail bool `query:"detail" description:"Include advertising-router information per route"`
}

func (h *OSPFRouteHandler) OptionsType() interface{} {
	return &OSPFRouteOptions{}
}
