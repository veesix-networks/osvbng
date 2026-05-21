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
	show.RegisterFactory(NewOSPFNeighborsDetailHandler)
}

type OSPFNeighborsDetailHandler struct {
	routing *routing.Component
}

func NewOSPFNeighborsDetailHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPFNeighborsDetailHandler{routing: deps.Routing}
}

func (h *OSPFNeighborsDetailHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetOSPFNeighborsDetail(req.Options["vrf"], req.Options["interface"])
}

func (h *OSPFNeighborsDetailHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPFNeighborsDetail
}

func (h *OSPFNeighborsDetailHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPFNeighborsDetailHandler) Summary() string {
	return "Show OSPFv2 neighbor detail"
}

func (h *OSPFNeighborsDetailHandler) Description() string {
	return "Display detailed OSPFv2 neighbor state including area, state-change counters, LSA retransmissions, and queue depths."
}

type OSPFNeighborsDetailOptions struct {
	VRF       string `query:"vrf" description:"VRF name; empty means the default routing table, 'all' returns every VRF"`
	Interface string `query:"interface" description:"Restrict output to neighbors on one interface"`
}

func (h *OSPFNeighborsDetailHandler) OptionsType() interface{} {
	return &OSPFNeighborsDetailOptions{}
}
