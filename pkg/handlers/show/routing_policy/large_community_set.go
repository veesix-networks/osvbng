// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package routing_policy

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewLargeCommunitySetHandler)
}

type LargeCommunitySetHandler struct {
	running deps.RunningConfigReader
}

func NewLargeCommunitySetHandler(d *deps.ShowDeps) show.ShowHandler {
	return &LargeCommunitySetHandler{running: d.RunningConfig}
}

func (h *LargeCommunitySetHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	wildcards, err := paths.Extract(req.Path, paths.RoutingPolicyLargeCommunitySet)
	if err != nil || len(wildcards) != 1 {
		return nil, fmt.Errorf("extract large-community-set name: %w", err)
	}
	name := wildcards[0]

	cfg, err := h.running.GetRunning()
	if err != nil {
		return nil, fmt.Errorf("get running config: %w", err)
	}
	if cfg == nil || cfg.RoutingPolicies == nil || cfg.RoutingPolicies.LargeCommunitySets == nil {
		return nil, fmt.Errorf("large-community-set %q not found", name)
	}

	set, ok := cfg.RoutingPolicies.LargeCommunitySets[name]
	if !ok {
		return nil, fmt.Errorf("large-community-set %q not found", name)
	}
	return set, nil
}

func (h *LargeCommunitySetHandler) PathPattern() paths.Path {
	return paths.RoutingPolicyLargeCommunitySet
}

func (h *LargeCommunitySetHandler) Dependencies() []paths.Path {
	return nil
}

func (h *LargeCommunitySetHandler) Summary() string {
	return "Show a single BGP large community set"
}

func (h *LargeCommunitySetHandler) Description() string {
	return "Display the members of a specific BGP large community set by name."
}
