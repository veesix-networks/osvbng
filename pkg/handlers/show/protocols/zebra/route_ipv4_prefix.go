// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package zebra

import (
	"context"
	"fmt"
	"net/netip"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewZebraRouteIPv4PrefixHandler)
}

type ZebraRouteIPv4PrefixHandler struct {
	routing *routing.Component
}

func NewZebraRouteIPv4PrefixHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &ZebraRouteIPv4PrefixHandler{routing: deps.Routing}
}

func (h *ZebraRouteIPv4PrefixHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	wildcards, err := paths.Extract(req.Path, paths.ProtocolsZebraRouteIPv4Prefix)
	if err != nil {
		return nil, fmt.Errorf("extract prefix: %w", err)
	}
	if len(wildcards) != 1 {
		return nil, fmt.Errorf("invalid path: expected 1 wildcard, got %d", len(wildcards))
	}
	prefix, err := netip.ParsePrefix(wildcards[0])
	if err != nil {
		return nil, fmt.Errorf("invalid prefix %q: %w", wildcards[0], err)
	}
	vrf := req.Options["vrf"]
	if vrf == "all" {
		return nil, fmt.Errorf("vrf=all is not supported for per-prefix lookups (FRR semantics vary by version)")
	}
	return h.routing.GetZebraRouteIPv4ByPrefix(vrf, prefix)
}

func (h *ZebraRouteIPv4PrefixHandler) PathPattern() paths.Path {
	return paths.ProtocolsZebraRouteIPv4Prefix
}
func (h *ZebraRouteIPv4PrefixHandler) Dependencies() []paths.Path { return nil }
func (h *ZebraRouteIPv4PrefixHandler) Summary() string {
	return "Look up an IPv4 prefix in zebra's RIB"
}
func (h *ZebraRouteIPv4PrefixHandler) Description() string {
	return "Display zebra's matching RIB entries for the requested IPv4 prefix; honors req.Options[\"vrf\"]. vrf=all is rejected for per-prefix lookups."
}

type ZebraRouteIPv4PrefixOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *ZebraRouteIPv4PrefixHandler) OptionsType() interface{} {
	return &ZebraRouteIPv4PrefixOptions{}
}
