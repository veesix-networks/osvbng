// Copyright 2026 The osvbng Authors
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
	show.RegisterFactory(NewExtCommunitySetsHandler)
}

type ExtCommunitySetsHandler struct {
	running deps.RunningConfigReader
}

func NewExtCommunitySetsHandler(d *deps.ShowDeps) show.ShowHandler {
	return &ExtCommunitySetsHandler{running: d.RunningConfig}
}

func (h *ExtCommunitySetsHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	cfg, err := h.running.GetRunning()
	if err != nil {
		return nil, fmt.Errorf("get running config: %w", err)
	}
	if cfg == nil || cfg.RoutingPolicies == nil || cfg.RoutingPolicies.ExtCommunitySets == nil {
		return map[string]rp.ExtCommunitySet{}, nil
	}
	return cfg.RoutingPolicies.ExtCommunitySets, nil
}

func (h *ExtCommunitySetsHandler) PathPattern() paths.Path {
	return paths.RoutingPolicyExtCommunitySets
}

func (h *ExtCommunitySetsHandler) Dependencies() []paths.Path {
	return nil
}

func (h *ExtCommunitySetsHandler) Summary() string {
	return "Show all BGP extended community sets"
}

func (h *ExtCommunitySetsHandler) Description() string {
	return "List every configured BGP extended community set with its members."
}
