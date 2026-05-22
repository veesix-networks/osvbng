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
	show.RegisterFactory(NewOSPF6NeighborsDetailHandler)
}

type OSPF6NeighborsDetailHandler struct {
	routing *routing.Component
}

func NewOSPF6NeighborsDetailHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPF6NeighborsDetailHandler{routing: deps.Routing}
}

func (h *OSPF6NeighborsDetailHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetOSPF6NeighborsDetail(req.Options["vrf"])
}

func (h *OSPF6NeighborsDetailHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPF6NeighborsDetail
}

func (h *OSPF6NeighborsDetailHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPF6NeighborsDetailHandler) Summary() string {
	return "Show OSPFv3 neighbor detail"
}

func (h *OSPF6NeighborsDetailHandler) Description() string {
	return "Display detailed OSPFv3 neighbor state including DB-desc, request, retransmit list counts and pending LSA queues. FRR keys the response by router-id%interface."
}

type OSPF6NeighborsDetailOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *OSPF6NeighborsDetailHandler) OptionsType() interface{} {
	return &OSPF6NeighborsDetailOptions{}
}
