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
	show.RegisterFactory(NewOSPF6SpfTreeHandler)
}

type OSPF6SpfTreeHandler struct {
	routing *routing.Component
}

func NewOSPF6SpfTreeHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPF6SpfTreeHandler{routing: deps.Routing}
}

func (h *OSPF6SpfTreeHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetOSPF6SpfTree(req.Options["vrf"])
}

func (h *OSPF6SpfTreeHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPF6SpfTree
}

func (h *OSPF6SpfTreeHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPF6SpfTreeHandler) Summary() string {
	return "Show OSPFv3 SPF tree"
}

func (h *OSPF6SpfTreeHandler) Description() string {
	return "Display the OSPFv3 SPF tree; response forwarded as-is from FRR."
}

type OSPF6SpfTreeOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *OSPF6SpfTreeHandler) OptionsType() interface{} {
	return &OSPF6SpfTreeOptions{}
}
