// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewOSPFReachableHandler)
}

type OSPFReachableHandler struct {
	routing *routing.Component
}

func NewOSPFReachableHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPFReachableHandler{routing: deps.Routing}
}

func (h *OSPFReachableHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetOSPFReachableRouters(req.Options["vrf"])
}

func (h *OSPFReachableHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPFReachable
}

func (h *OSPFReachableHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPFReachableHandler) Summary() string {
	return "Show OSPFv2 reachable routers"
}

func (h *OSPFReachableHandler) Description() string {
	return "Display the OSPFv2 reachable-routers table. FRR returns plain text for this command; the response is forwarded as-is."
}

type OSPFReachableOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table, 'all' returns every VRF"`
}

func (h *OSPFReachableHandler) OptionsType() interface{} {
	return &OSPFReachableOptions{}
}
