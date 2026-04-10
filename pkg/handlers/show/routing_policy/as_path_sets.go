// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package routing_policy

import (
	"context"
	"fmt"

	rp "github.com/veesix-networks/osvbng/pkg/config/routing_policy"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewASPathSetsHandler)
}

type ASPathSetsHandler struct {
	running deps.RunningConfigReader
}

func NewASPathSetsHandler(d *deps.ShowDeps) show.ShowHandler {
	return &ASPathSetsHandler{running: d.RunningConfig}
}

func (h *ASPathSetsHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	cfg, err := h.running.GetRunning()
	if err != nil {
		return nil, fmt.Errorf("get running config: %w", err)
	}
	if cfg == nil || cfg.RoutingPolicies == nil || cfg.RoutingPolicies.ASPathSets == nil {
		return map[string]rp.ASPathSet{}, nil
	}
	return cfg.RoutingPolicies.ASPathSets, nil
}

func (h *ASPathSetsHandler) PathPattern() paths.Path {
	return paths.RoutingPolicyASPathSets
}

func (h *ASPathSetsHandler) Dependencies() []paths.Path {
	return nil
}

func (h *ASPathSetsHandler) Summary() string {
	return "Show all BGP AS path sets"
}

func (h *ASPathSetsHandler) Description() string {
	return "List every configured BGP AS path access list with its regex entries."
}
