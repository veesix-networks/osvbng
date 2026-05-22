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
	"github.com/veesix-networks/osvbng/pkg/models/protocols/ospf6"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

func init() {
	show.RegisterFactory(NewOSPF6InterfacesHandler)
	telemetry.RegisterMetric[ospf6.Interface](paths.ProtocolsOSPF6Interfaces)
}

type OSPF6InterfacesHandler struct {
	routing *routing.Component
}

func NewOSPF6InterfacesHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPF6InterfacesHandler{routing: deps.Routing}
}

func (h *OSPF6InterfacesHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetOSPF6Interfaces(req.Options["vrf"], req.Options["interface"])
}

func (h *OSPF6InterfacesHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPF6Interfaces
}

func (h *OSPF6InterfacesHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPF6InterfacesHandler) Summary() string {
	return "Show OSPFv3 interfaces"
}

func (h *OSPF6InterfacesHandler) Description() string {
	return "Display OSPFv3 per-interface state, MTU, area, cost, timers, and LSA counts."
}

type OSPF6InterfacesOptions struct {
	VRF       string `query:"vrf" description:"VRF name; empty means the default routing table"`
	Interface string `query:"interface" description:"Restrict output to one interface name"`
}

func (h *OSPF6InterfacesHandler) OptionsType() interface{} {
	return &OSPF6InterfacesOptions{}
}
