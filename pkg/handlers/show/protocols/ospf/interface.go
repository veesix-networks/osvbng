// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewOSPFInterfacesHandler)
}

type OSPFInterfacesHandler struct {
	routing *routing.Component
}

func NewOSPFInterfacesHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPFInterfacesHandler{routing: deps.Routing}
}

func (h *OSPFInterfacesHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetOSPFInterfaces(req.Options["vrf"], req.Options["interface"])
}

func (h *OSPFInterfacesHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPFInterfaces
}

func (h *OSPFInterfacesHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPFInterfacesHandler) Summary() string {
	return "Show OSPFv2 interfaces"
}

func (h *OSPFInterfacesHandler) Description() string {
	return "Display OSPFv2 per-interface state, timers, costs, and neighbor counts."
}

type OSPFInterfacesOptions struct {
	VRF       string `query:"vrf" description:"VRF name; empty means the default routing table, 'all' returns every VRF"`
	Interface string `query:"interface" description:"Restrict output to one interface name"`
}

func (h *OSPFInterfacesHandler) OptionsType() interface{} {
	return &OSPFInterfacesOptions{}
}
