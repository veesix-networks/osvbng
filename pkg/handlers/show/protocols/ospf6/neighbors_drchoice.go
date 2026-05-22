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
	show.RegisterFactory(NewOSPF6NeighborsDRChoiceHandler)
}

type OSPF6NeighborsDRChoiceHandler struct {
	routing *routing.Component
}

func NewOSPF6NeighborsDRChoiceHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPF6NeighborsDRChoiceHandler{routing: deps.Routing}
}

func (h *OSPF6NeighborsDRChoiceHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetOSPF6NeighborsDRChoice(req.Options["vrf"])
}

func (h *OSPF6NeighborsDRChoiceHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPF6NeighborsDRChoice
}

func (h *OSPF6NeighborsDRChoiceHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPF6NeighborsDRChoiceHandler) Summary() string {
	return "Show OSPFv3 neighbor DR-election choices"
}

func (h *OSPF6NeighborsDRChoiceHandler) Description() string {
	return "Display OSPFv3 neighbor DR / BDR election choices per interface."
}

type OSPF6NeighborsDRChoiceOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *OSPF6NeighborsDRChoiceHandler) OptionsType() interface{} {
	return &OSPF6NeighborsDRChoiceOptions{}
}
