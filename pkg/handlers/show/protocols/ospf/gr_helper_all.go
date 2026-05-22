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
	show.RegisterFactory(NewOSPFGRHelperAllHandler)
	telemetry.RegisterMetric[ospf.GRHelper](paths.ProtocolsOSPFGRHelperAll)
}

type OSPFGRHelperAllHandler struct {
	routing *routing.Component
}

func NewOSPFGRHelperAllHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPFGRHelperAllHandler{routing: deps.Routing}
}

func (h *OSPFGRHelperAllHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetOSPFGRHelperAll()
}

func (h *OSPFGRHelperAllHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPFGRHelperAll
}

func (h *OSPFGRHelperAllHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPFGRHelperAllHandler) Summary() string {
	return "Show OSPFv2 graceful-restart helper state across all VRFs"
}

func (h *OSPFGRHelperAllHandler) Description() string {
	return "Display OSPFv2 graceful-restart helper state for every VRF, keyed by VRF name. Backs Prometheus scrape of per-VRF GR-helper metrics."
}
