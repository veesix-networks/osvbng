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
	show.RegisterFactory(NewLDPBindingsDetailHandler)
}

type LDPBindingsDetailHandler struct {
	routing *routing.Component
}

func NewLDPBindingsDetailHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &LDPBindingsDetailHandler{routing: deps.Routing}
}

func (h *LDPBindingsDetailHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetLDPBindingsDetail(req.Options["afi"])
}

func (h *LDPBindingsDetailHandler) PathPattern() paths.Path {
	return paths.ProtocolsLDPBindingsDetail
}

func (h *LDPBindingsDetailHandler) Dependencies() []paths.Path {
	return nil
}

func (h *LDPBindingsDetailHandler) Summary() string {
	return "Show detailed LDP label bindings"
}

func (h *LDPBindingsDetailHandler) Description() string {
	return "Display LDP label bindings keyed by prefix, with sent-to and remote-label entries per neighbor; response forwarded as-is from FRR."
}

type LDPBindingsDetailOptions struct {
	AFI string `query:"afi" description:"Address family: ipv4 or ipv6; empty means both"`
}

func (h *LDPBindingsDetailHandler) OptionsType() interface{} {
	return &LDPBindingsDetailOptions{}
}
