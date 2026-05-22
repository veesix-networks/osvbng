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
	show.RegisterFactory(NewBGPVPNIPv6PrefixHandler)
}

type BGPVPNIPv6PrefixHandler struct {
	routing *routing.Component
}

func NewBGPVPNIPv6PrefixHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPVPNIPv6PrefixHandler{routing: deps.Routing}
}

func (h *BGPVPNIPv6PrefixHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	wildcards, err := paths.Extract(req.Path, paths.ProtocolsBGPVPNIPv6Prefix)
	if err != nil {
		return nil, fmt.Errorf("extract prefix: %w", err)
	}
	if len(wildcards) != 1 {
		return nil, fmt.Errorf("invalid path: expected 1 wildcard, got %d", len(wildcards))
	}
	return h.routing.GetBGPVPNRouteByPrefix("ipv6", wildcards[0])
}

func (h *BGPVPNIPv6PrefixHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVPNIPv6Prefix
}

func (h *BGPVPNIPv6PrefixHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPVPNIPv6PrefixHandler) Summary() string {
	return "Show BGP VPNv6 paths for one prefix"
}

func (h *BGPVPNIPv6PrefixHandler) Description() string {
	return "Display BGP VPNv6 paths matching the supplied prefix (X:X::X:X/M); response forwarded as-is from FRR."
}
