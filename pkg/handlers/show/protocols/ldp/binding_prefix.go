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
	show.RegisterFactory(NewLDPBindingPrefixHandler)
}

type LDPBindingPrefixHandler struct {
	routing *routing.Component
}

func NewLDPBindingPrefixHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &LDPBindingPrefixHandler{routing: deps.Routing}
}

func (h *LDPBindingPrefixHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	wildcards, err := paths.Extract(req.Path, paths.ProtocolsLDPBindingPrefix)
	if err != nil {
		return nil, fmt.Errorf("extract LDP binding prefix: %w", err)
	}
	if len(wildcards) != 1 {
		return nil, fmt.Errorf("invalid path: expected 1 wildcard, got %d", len(wildcards))
	}
	_, longerPrefixes := req.Options["longer_prefixes"]
	_, detail := req.Options["detail"]
	return h.routing.GetLDPBindingByPrefix(
		req.Options["afi"],
		wildcards[0],
		longerPrefixes,
		detail,
		routing.BindingFilter{
			Neighbor:    req.Options["neighbor"],
			LocalLabel:  req.Options["local_label"],
			RemoteLabel: req.Options["remote_label"],
		},
	)
}

func (h *LDPBindingPrefixHandler) PathPattern() paths.Path {
	return paths.ProtocolsLDPBindingPrefix
}

func (h *LDPBindingPrefixHandler) Dependencies() []paths.Path {
	return nil
}

func (h *LDPBindingPrefixHandler) Summary() string {
	return "Show LDP label bindings for one prefix"
}

func (h *LDPBindingPrefixHandler) Description() string {
	return "Display LDP label bindings matching one prefix, optionally with longer-prefixes; response forwarded as-is from FRR."
}

type LDPBindingPrefixOptions struct {
	AFI            string `query:"afi" description:"Address family: ipv4 or ipv6; empty means both"`
	LongerPrefixes string `query:"longer_prefixes" description:"Set to include longer prefixes than the supplied one (presence flag)"`
	Detail         string `query:"detail" description:"Set to include detailed binding state (presence flag)"`
	Neighbor       string `query:"neighbor" description:"Filter by LDP neighbor LSR-ID"`
	LocalLabel     string `query:"local_label" description:"Filter by local label (numeric or imp-null)"`
	RemoteLabel    string `query:"remote_label" description:"Filter by remote label (numeric or imp-null)"`
}

func (h *LDPBindingPrefixHandler) OptionsType() interface{} {
	return &LDPBindingPrefixOptions{}
}
