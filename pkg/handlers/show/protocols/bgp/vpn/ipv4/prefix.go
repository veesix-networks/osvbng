// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ipv4

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewBGPVPNIPv4PrefixHandler)
}

type BGPVPNIPv4PrefixHandler struct {
	routing *routing.Component
}

func NewBGPVPNIPv4PrefixHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPVPNIPv4PrefixHandler{routing: deps.Routing}
}

func (h *BGPVPNIPv4PrefixHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	wildcards, err := paths.Extract(req.Path, paths.ProtocolsBGPVPNIPv4Prefix)
	if err != nil {
		return nil, fmt.Errorf("extract prefix: %w", err)
	}
	if len(wildcards) != 1 {
		return nil, fmt.Errorf("invalid path: expected 1 wildcard, got %d", len(wildcards))
	}
	return h.routing.GetBGPVPNRouteByPrefix("ipv4", wildcards[0])
}

func (h *BGPVPNIPv4PrefixHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVPNIPv4Prefix
}

func (h *BGPVPNIPv4PrefixHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPVPNIPv4PrefixHandler) Summary() string {
	return "Show BGP VPNv4 paths for one prefix"
}

func (h *BGPVPNIPv4PrefixHandler) Description() string {
	return "Display BGP VPNv4 paths matching the supplied prefix (A.B.C.D/M); response forwarded as-is from FRR."
}
