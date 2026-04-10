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
	show.RegisterFactory(NewCommunitySetHandler)
}

type CommunitySetHandler struct {
	running deps.RunningConfigReader
}

func NewCommunitySetHandler(d *deps.ShowDeps) show.ShowHandler {
	return &CommunitySetHandler{running: d.RunningConfig}
}

func (h *CommunitySetHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	wildcards, err := paths.Extract(req.Path, paths.RoutingPolicyCommunitySet)
	if err != nil || len(wildcards) != 1 {
		return nil, fmt.Errorf("extract community-set name: %w", err)
	}
	name := wildcards[0]

	cfg, err := h.running.GetRunning()
	if err != nil {
		return nil, fmt.Errorf("get running config: %w", err)
	}
	if cfg == nil || cfg.RoutingPolicies == nil || cfg.RoutingPolicies.CommunitySets == nil {
		return nil, fmt.Errorf("community-set %q not found", name)
	}

	set, ok := cfg.RoutingPolicies.CommunitySets[name]
	if !ok {
		return nil, fmt.Errorf("community-set %q not found", name)
	}
	return set, nil
}

func (h *CommunitySetHandler) PathPattern() paths.Path {
	return paths.RoutingPolicyCommunitySet
}

func (h *CommunitySetHandler) Dependencies() []paths.Path {
	return nil
}

func (h *CommunitySetHandler) Summary() string {
	return "Show a single BGP community set"
}

func (h *CommunitySetHandler) Description() string {
	return "Display the members of a specific BGP community set by name."
}
