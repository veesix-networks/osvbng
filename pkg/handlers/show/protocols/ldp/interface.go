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
	"github.com/veesix-networks/osvbng/pkg/models/protocols/ldp"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

func init() {
	show.RegisterFactory(NewLDPInterfaceHandler)
	telemetry.RegisterMetric[ldp.Interface](paths.ProtocolsLDPInterface)
}

type LDPInterfaceHandler struct {
	routing *routing.Component
}

func NewLDPInterfaceHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &LDPInterfaceHandler{routing: deps.Routing}
}

func (h *LDPInterfaceHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetLDPInterface(req.Options["afi"])
}

func (h *LDPInterfaceHandler) PathPattern() paths.Path {
	return paths.ProtocolsLDPInterface
}

func (h *LDPInterfaceHandler) Dependencies() []paths.Path {
	return nil
}

func (h *LDPInterfaceHandler) Summary() string {
	return "Show LDP interfaces"
}

func (h *LDPInterfaceHandler) Description() string {
	return "Display per-interface LDP state: hello timers, adjacency count, address family."
}

type LDPInterfaceOptions struct {
	AFI string `query:"afi" description:"Address family: ipv4 or ipv6; empty means both"`
}

func (h *LDPInterfaceHandler) OptionsType() interface{} {
	return &LDPInterfaceOptions{}
}
