// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ipv6

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewBGPVPNIPv6RDHandler)
}

type BGPVPNIPv6RDHandler struct {
	routing *routing.Component
}

func NewBGPVPNIPv6RDHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPVPNIPv6RDHandler{routing: deps.Routing}
}

func (h *BGPVPNIPv6RDHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	wildcards, err := paths.Extract(req.Path, paths.ProtocolsBGPVPNIPv6RD)
	if err != nil {
		return nil, fmt.Errorf("extract route-distinguisher: %w", err)
	}
	if len(wildcards) != 1 {
		return nil, fmt.Errorf("invalid path: expected 1 wildcard, got %d", len(wildcards))
	}
	return h.routing.GetBGPVPNRouteByRD("ipv6", wildcards[0])
}

func (h *BGPVPNIPv6RDHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVPNIPv6RD
}

func (h *BGPVPNIPv6RDHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPVPNIPv6RDHandler) Summary() string {
	return "Show BGP VPNv6 routes for one route-distinguisher"
}

func (h *BGPVPNIPv6RDHandler) Description() string {
	return "Display BGP VPNv6 routes carrying the supplied route-distinguisher; response forwarded as-is from FRR."
}
