// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/state"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	show.RegisterFactory(func(d *deps.ShowDeps) show.ShowHandler {
		return &StatisticsHandler{deps: d}
	})

	state.RegisterMetric(statepaths.CGNATStatistics, paths.CGNATStatistics)
}

type StatisticsHandler struct {
	deps *deps.ShowDeps
}

type CGNATStatisticsResponse struct {
	Pools []models.CGNATPoolStats `json:"pools"`
}

func (h *StatisticsHandler) Collect(_ context.Context, _ *show.Request) (interface{}, error) {
	if h.deps.CGNAT == nil {
		return &CGNATStatisticsResponse{}, nil
	}

	cfg, err := h.deps.CGNAT.GetRunningConfig()
	if err != nil || cfg == nil {
		return &CGNATStatisticsResponse{}, nil
	}

	var stats []models.CGNATPoolStats
	pm := h.deps.CGNAT.GetPoolManager()
	for name := range cfg.Pools {
		if s := pm.GetPoolStats(name); s != nil {
			stats = append(stats, *s)
		}
	}

	return &CGNATStatisticsResponse{Pools: stats}, nil
}

func (h *StatisticsHandler) PathPattern() paths.Path {
	return paths.CGNATStatistics
}

func (h *StatisticsHandler) Dependencies() []paths.Path {
	return nil
}

func (h *StatisticsHandler) Summary() string {
	return "Show CGNAT aggregate statistics"
}

func (h *StatisticsHandler) Description() string {
	return "Display aggregate CGNAT statistics across all configured pools."
}
