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
	show.RegisterFactory(NewLargeCommunitySetsHandler)
}

type LargeCommunitySetsHandler struct {
	running deps.RunningConfigReader
}

func NewLargeCommunitySetsHandler(d *deps.ShowDeps) show.ShowHandler {
	return &LargeCommunitySetsHandler{running: d.RunningConfig}
}

func (h *LargeCommunitySetsHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	cfg, err := h.running.GetRunning()
	if err != nil {
		return nil, fmt.Errorf("get running config: %w", err)
	}
	if cfg == nil || cfg.RoutingPolicies == nil || cfg.RoutingPolicies.LargeCommunitySets == nil {
		return map[string]rp.LargeCommunitySet{}, nil
	}
	return cfg.RoutingPolicies.LargeCommunitySets, nil
}

func (h *LargeCommunitySetsHandler) PathPattern() paths.Path {
	return paths.RoutingPolicyLargeCommunitySets
}

func (h *LargeCommunitySetsHandler) Dependencies() []paths.Path {
	return nil
}

func (h *LargeCommunitySetsHandler) Summary() string {
	return "Show all BGP large community sets"
}

func (h *LargeCommunitySetsHandler) Description() string {
	return "List every configured BGP large community set with its members."
}
