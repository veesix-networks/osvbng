// Copyright 2026 The osvbng Authors
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
	show.RegisterFactory(NewRoutePolicyHandler)
}

type RoutePolicyHandler struct {
	running deps.RunningConfigReader
}

func NewRoutePolicyHandler(d *deps.ShowDeps) show.ShowHandler {
	return &RoutePolicyHandler{running: d.RunningConfig}
}

func (h *RoutePolicyHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	wildcards, err := paths.Extract(req.Path, paths.RoutingPolicyRoutePolicy)
	if err != nil || len(wildcards) != 1 {
		return nil, fmt.Errorf("extract route-policy name: %w", err)
	}
	name := wildcards[0]

	cfg, err := h.running.GetRunning()
	if err != nil {
		return nil, fmt.Errorf("get running config: %w", err)
	}
	if cfg == nil || cfg.RoutingPolicies == nil || cfg.RoutingPolicies.RoutePolicies == nil {
		return nil, fmt.Errorf("route-policy %q not found", name)
	}

	policy, ok := cfg.RoutingPolicies.RoutePolicies[name]
	if !ok {
		return nil, fmt.Errorf("route-policy %q not found", name)
	}
	return policy, nil
}

func (h *RoutePolicyHandler) PathPattern() paths.Path {
	return paths.RoutingPolicyRoutePolicy
}

func (h *RoutePolicyHandler) Dependencies() []paths.Path {
	return nil
}

func (h *RoutePolicyHandler) Summary() string {
	return "Show a single route policy"
}

func (h *RoutePolicyHandler) Description() string {
	return "Display the match and set clauses of a specific route policy by name."
}
