// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ldp

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewLDPDiscoveryDetailHandler)
}

type LDPDiscoveryDetailHandler struct {
	routing *routing.Component
}

func NewLDPDiscoveryDetailHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &LDPDiscoveryDetailHandler{routing: deps.Routing}
}

func (h *LDPDiscoveryDetailHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetLDPDiscoveryDetail(req.Options["afi"])
}

func (h *LDPDiscoveryDetailHandler) PathPattern() paths.Path {
	return paths.ProtocolsLDPDiscoveryDetail
}

func (h *LDPDiscoveryDetailHandler) Dependencies() []paths.Path {
	return nil
}

func (h *LDPDiscoveryDetailHandler) Summary() string {
	return "Show detailed LDP discovery state"
}

func (h *LDPDiscoveryDetailHandler) Description() string {
	return "Display LDP hello discovery with full adjacency state, transport addresses, and targeted-hello state; response forwarded as-is from FRR."
}

type LDPDiscoveryDetailOptions struct {
	AFI string `query:"afi" description:"Address family: ipv4 or ipv6; empty means both"`
}

func (h *LDPDiscoveryDetailHandler) OptionsType() interface{} {
	return &LDPDiscoveryDetailOptions{}
}
