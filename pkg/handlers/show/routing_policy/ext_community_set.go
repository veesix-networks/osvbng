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
	show.RegisterFactory(NewExtCommunitySetHandler)
}

type ExtCommunitySetHandler struct {
	running deps.RunningConfigReader
}

func NewExtCommunitySetHandler(d *deps.ShowDeps) show.ShowHandler {
	return &ExtCommunitySetHandler{running: d.RunningConfig}
}

func (h *ExtCommunitySetHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	wildcards, err := paths.Extract(req.Path, paths.RoutingPolicyExtCommunitySet)
	if err != nil || len(wildcards) != 1 {
		return nil, fmt.Errorf("extract ext-community-set name: %w", err)
	}
	name := wildcards[0]

	cfg, err := h.running.GetRunning()
	if err != nil {
		return nil, fmt.Errorf("get running config: %w", err)
	}
	if cfg == nil || cfg.RoutingPolicies == nil || cfg.RoutingPolicies.ExtCommunitySets == nil {
		return nil, fmt.Errorf("ext-community-set %q not found", name)
	}

	set, ok := cfg.RoutingPolicies.ExtCommunitySets[name]
	if !ok {
		return nil, fmt.Errorf("ext-community-set %q not found", name)
	}
	return set, nil
}

func (h *ExtCommunitySetHandler) PathPattern() paths.Path {
	return paths.RoutingPolicyExtCommunitySet
}

func (h *ExtCommunitySetHandler) Dependencies() []paths.Path {
	return nil
}

func (h *ExtCommunitySetHandler) Summary() string {
	return "Show a single BGP extended community set"
}

func (h *ExtCommunitySetHandler) Description() string {
	return "Display the members of a specific BGP extended community set by name."
}
