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
	show.RegisterFactory(NewOSPFInstanceAllHandler)
	telemetry.RegisterMetric[ospf.Instance](paths.ProtocolsOSPFAll)
}

type OSPFInstanceAllHandler struct {
	routing *routing.Component
}

func NewOSPFInstanceAllHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPFInstanceAllHandler{routing: deps.Routing}
}

func (h *OSPFInstanceAllHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetOSPFInstanceAll()
}

func (h *OSPFInstanceAllHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPFAll
}

func (h *OSPFInstanceAllHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPFInstanceAllHandler) Summary() string {
	return "Show OSPFv2 instance state across all VRFs"
}

func (h *OSPFInstanceAllHandler) Description() string {
	return "Display OSPFv2 instance state for every VRF, keyed by VRF name. Backs Prometheus scrape of per-VRF OSPF metrics."
}
