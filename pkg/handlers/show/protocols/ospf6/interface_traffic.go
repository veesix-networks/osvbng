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
	show.RegisterFactory(NewOSPF6InterfaceTrafficHandler)
	telemetry.RegisterMetric[ospf6.InterfaceTraffic](paths.ProtocolsOSPF6InterfaceTraffic)
}

type OSPF6InterfaceTrafficHandler struct {
	routing *routing.Component
}

func NewOSPF6InterfaceTrafficHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPF6InterfaceTrafficHandler{routing: deps.Routing}
}

func (h *OSPF6InterfaceTrafficHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetOSPF6InterfaceTraffic(req.Options["vrf"], req.Options["interface"])
}

func (h *OSPF6InterfaceTrafficHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPF6InterfaceTraffic
}

func (h *OSPF6InterfaceTrafficHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPF6InterfaceTrafficHandler) Summary() string {
	return "Show OSPFv3 interface traffic counters"
}

func (h *OSPF6InterfaceTrafficHandler) Description() string {
	return "Display OSPFv3 per-interface packet counters for hello, dbdesc, lsreq, lsupdate, and lsack."
}

type OSPF6InterfaceTrafficOptions struct {
	VRF       string `query:"vrf" description:"VRF name; empty means the default routing table"`
	Interface string `query:"interface" description:"Restrict output to one interface name"`
}

func (h *OSPF6InterfaceTrafficHandler) OptionsType() interface{} {
	return &OSPF6InterfaceTrafficOptions{}
}
