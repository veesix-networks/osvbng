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
	show.RegisterFactory(NewOSPFRouterInfoHandler)
}

type OSPFRouterInfoHandler struct {
	routing *routing.Component
}

func NewOSPFRouterInfoHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPFRouterInfoHandler{routing: deps.Routing}
}

func (h *OSPFRouterInfoHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	pce, _ := strconv.ParseBool(req.Options["pce"])
	return h.routing.GetOSPFRouterInfo(pce)
}

func (h *OSPFRouterInfoHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPFRouterInfo
}

func (h *OSPFRouterInfoHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPFRouterInfoHandler) Summary() string {
	return "Show OSPFv2 Router Information (RFC 4970) state"
}

func (h *OSPFRouterInfoHandler) Description() string {
	return "Display OSPF Router Information LSA capability state, or PCE parameters when pce is set. FRR returns plain text; the response is forwarded as-is."
}

type OSPFRouterInfoOptions struct {
	PCE bool `query:"pce" description:"Show PCE (Path Computation Element) parameters"`
}

func (h *OSPFRouterInfoHandler) OptionsType() interface{} {
	return &OSPFRouterInfoOptions{}
}
