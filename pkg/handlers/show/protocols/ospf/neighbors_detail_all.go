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
	show.RegisterFactory(NewOSPFNeighborsDetailAllHandler)
	telemetry.RegisterMetric[ospf.NeighborDetailMap](paths.ProtocolsOSPFNeighborsDetailAll)
}

type OSPFNeighborsDetailAllHandler struct {
	routing *routing.Component
}

func NewOSPFNeighborsDetailAllHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPFNeighborsDetailAllHandler{routing: deps.Routing}
}

func (h *OSPFNeighborsDetailAllHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetOSPFNeighborsDetailAll()
}

func (h *OSPFNeighborsDetailAllHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPFNeighborsDetailAll
}

func (h *OSPFNeighborsDetailAllHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPFNeighborsDetailAllHandler) Summary() string {
	return "Show OSPFv2 neighbor detail across all VRFs"
}

func (h *OSPFNeighborsDetailAllHandler) Description() string {
	return "Display detailed OSPFv2 neighbor state for every VRF, keyed by VRF name. Backs Prometheus scrape of per-VRF neighbor detail metrics."
}
