// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package dataplane

import (
	"context"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

func init() {
	show.RegisterFactory(NewInterfacesHandler)
	telemetry.RegisterMetric[southbound.InterfaceStats](paths.SystemDataplaneInterfaces)
}

// interfaceStatsTTL coalesces same-tick consumers (telemetry, AAA bucket
// processor, operator show calls) onto one stats-segment read. 1s keeps
// the freshness well inside the 10s telemetry cadence.
const interfaceStatsTTL = 1 * time.Second

type InterfacesHandler struct {
	southbound southbound.Southbound

	mu       sync.Mutex
	cached   []southbound.InterfaceStats
	cachedAt time.Time
	now      func() time.Time
}

func NewInterfacesHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &InterfacesHandler{southbound: deps.Southbound, now: time.Now}
}

func (h *InterfacesHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := h.now()
	if h.cached != nil && now.Sub(h.cachedAt) < interfaceStatsTTL {
		return h.cached, nil
	}

	stats, err := h.southbound.GetInterfaceStats()
	if err != nil {
		return nil, err
	}
	h.cached = stats
	h.cachedAt = now
	return stats, nil
}

func (h *InterfacesHandler) PathPattern() paths.Path {
	return paths.SystemDataplaneInterfaces
}

func (h *InterfacesHandler) Dependencies() []paths.Path {
	return nil
}

func (h *InterfacesHandler) Summary() string {
	return "Show VPP interface counters"
}

func (h *InterfacesHandler) Description() string {
	return "Display per-interface packet and byte counters from the VPP stats segment."
}

func (h *InterfacesHandler) SortKey() string {
	return "name"
}
