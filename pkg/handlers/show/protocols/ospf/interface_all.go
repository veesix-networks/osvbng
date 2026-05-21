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
	"github.com/veesix-networks/osvbng/pkg/models/protocols/ospf"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

func init() {
	show.RegisterFactory(NewOSPFInterfacesAllHandler)
	telemetry.RegisterMetric[ospf.InterfaceMap](paths.ProtocolsOSPFInterfacesAll)
}

type OSPFInterfacesAllHandler struct {
	routing *routing.Component
}

func NewOSPFInterfacesAllHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPFInterfacesAllHandler{routing: deps.Routing}
}

func (h *OSPFInterfacesAllHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetOSPFInterfacesAll()
}

func (h *OSPFInterfacesAllHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPFInterfacesAll
}

func (h *OSPFInterfacesAllHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPFInterfacesAllHandler) Summary() string {
	return "Show OSPFv2 interfaces across all VRFs"
}

func (h *OSPFInterfacesAllHandler) Description() string {
	return "Display OSPFv2 interface state for every VRF, keyed by VRF name. Backs Prometheus scrape of per-VRF OSPF interface metrics."
}
