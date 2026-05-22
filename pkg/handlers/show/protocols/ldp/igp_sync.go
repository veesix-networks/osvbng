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
	show.RegisterFactory(NewLDPIGPSyncHandler)
	telemetry.RegisterMetric[ldp.IGPSync](paths.ProtocolsLDPIGPSync)
}

type LDPIGPSyncHandler struct {
	routing *routing.Component
}

func NewLDPIGPSyncHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &LDPIGPSyncHandler{routing: deps.Routing}
}

func (h *LDPIGPSyncHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetLDPIGPSync()
}

func (h *LDPIGPSyncHandler) PathPattern() paths.Path {
	return paths.ProtocolsLDPIGPSync
}

func (h *LDPIGPSyncHandler) Dependencies() []paths.Path {
	return nil
}

func (h *LDPIGPSyncHandler) Summary() string {
	return "Show LDP IGP-sync state"
}

func (h *LDPIGPSyncHandler) Description() string {
	return "Display per-interface LDP IGP-sync state, wait timer, and peer LSR-ID."
}
