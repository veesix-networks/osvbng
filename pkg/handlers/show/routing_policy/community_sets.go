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
	show.RegisterFactory(NewCommunitySetsHandler)
}

type CommunitySetsHandler struct {
	running deps.RunningConfigReader
}

func NewCommunitySetsHandler(d *deps.ShowDeps) show.ShowHandler {
	return &CommunitySetsHandler{running: d.RunningConfig}
}

func (h *CommunitySetsHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	cfg, err := h.running.GetRunning()
	if err != nil {
		return nil, fmt.Errorf("get running config: %w", err)
	}
	if cfg == nil || cfg.RoutingPolicies == nil || cfg.RoutingPolicies.CommunitySets == nil {
		return map[string]rp.CommunitySet{}, nil
	}
	return cfg.RoutingPolicies.CommunitySets, nil
}

func (h *CommunitySetsHandler) PathPattern() paths.Path {
	return paths.RoutingPolicyCommunitySets
}

func (h *CommunitySetsHandler) Dependencies() []paths.Path {
	return nil
}

func (h *CommunitySetsHandler) Summary() string {
	return "Show all BGP community sets"
}

func (h *CommunitySetsHandler) Description() string {
	return "List every configured BGP community set with its members."
}
