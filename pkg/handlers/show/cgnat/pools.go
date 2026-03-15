// Copyright 2026 Veesix Networks Ltd
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
		return &PoolsHandler{deps: d}
	})

	state.RegisterMetric(statepaths.CGNATPools, paths.CGNATPools)
}

type PoolsHandler struct {
	deps *deps.ShowDeps
}

func (h *PoolsHandler) Collect(_ context.Context, _ *show.Request) (interface{}, error) {
	if h.deps.CGNAT == nil {
		return []models.CGNATPoolStats{}, nil
	}

	cfg, err := h.deps.CGNAT.GetRunningConfig()
	if err != nil || cfg == nil {
		return []models.CGNATPoolStats{}, nil
	}

	var stats []models.CGNATPoolStats
	pm := h.deps.CGNAT.GetPoolManager()
	for name := range cfg.Pools {
		if s := pm.GetPoolStats(name); s != nil {
			stats = append(stats, *s)
		}
	}

	return stats, nil
}

func (h *PoolsHandler) PathPattern() paths.Path {
	return paths.CGNATPools
}

func (h *PoolsHandler) Dependencies() []paths.Path {
	return nil
}
