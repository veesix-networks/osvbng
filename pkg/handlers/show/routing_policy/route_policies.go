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
	show.RegisterFactory(NewRoutePoliciesHandler)
}

type RoutePoliciesHandler struct {
	running deps.RunningConfigReader
}

func NewRoutePoliciesHandler(d *deps.ShowDeps) show.ShowHandler {
	return &RoutePoliciesHandler{running: d.RunningConfig}
}

func (h *RoutePoliciesHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	cfg, err := h.running.GetRunning()
	if err != nil {
		return nil, fmt.Errorf("get running config: %w", err)
	}
	if cfg == nil || cfg.RoutingPolicies == nil || cfg.RoutingPolicies.RoutePolicies == nil {
		return map[string]rp.RoutePolicy{}, nil
	}
	return cfg.RoutingPolicies.RoutePolicies, nil
}

func (h *RoutePoliciesHandler) PathPattern() paths.Path {
	return paths.RoutingPolicyRoutePolicies
}

func (h *RoutePoliciesHandler) Dependencies() []paths.Path {
	return nil
}

func (h *RoutePoliciesHandler) Summary() string {
	return "Show all route policies"
}

func (h *RoutePoliciesHandler) Description() string {
	return "List every configured route policy (route-map) with its match and set clauses."
}
