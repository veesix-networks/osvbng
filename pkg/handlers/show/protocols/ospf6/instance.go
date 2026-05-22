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
	show.RegisterFactory(NewOSPF6InstanceHandler)
	telemetry.RegisterMetric[ospf6.Instance](paths.ProtocolsOSPF6)
}

type OSPF6InstanceHandler struct {
	routing *routing.Component
}

func NewOSPF6InstanceHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPF6InstanceHandler{routing: deps.Routing}
}

func (h *OSPF6InstanceHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetOSPF6Instance(req.Options["vrf"])
}

func (h *OSPF6InstanceHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPF6
}

func (h *OSPF6InstanceHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPF6InstanceHandler) Summary() string {
	return "Show OSPFv3 instance state"
}

func (h *OSPF6InstanceHandler) Description() string {
	return "Display OSPFv3 router state, SPF timing, AS-scoped LSA count, and per-area LSDB summary."
}

type OSPF6InstanceOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *OSPF6InstanceHandler) OptionsType() interface{} {
	return &OSPF6InstanceOptions{}
}
