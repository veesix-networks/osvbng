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
	show.RegisterFactory(NewPrefixSetsHandler)
}

type PrefixSetsHandler struct {
	running deps.RunningConfigReader
}

func NewPrefixSetsHandler(d *deps.ShowDeps) show.ShowHandler {
	return &PrefixSetsHandler{running: d.RunningConfig}
}

func (h *PrefixSetsHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	cfg, err := h.running.GetRunning()
	if err != nil {
		return nil, fmt.Errorf("get running config: %w", err)
	}
	if cfg == nil || cfg.RoutingPolicies == nil {
		return map[string]rp.PrefixSet{}, nil
	}
	if cfg.RoutingPolicies.PrefixSets == nil {
		return map[string]rp.PrefixSet{}, nil
	}
	return cfg.RoutingPolicies.PrefixSets, nil
}

func (h *PrefixSetsHandler) PathPattern() paths.Path {
	return paths.RoutingPolicyPrefixSets
}

func (h *PrefixSetsHandler) Dependencies() []paths.Path {
	return nil
}

func (h *PrefixSetsHandler) Summary() string {
	return "Show all IPv4 prefix sets"
}

func (h *PrefixSetsHandler) Description() string {
	return "List every configured IPv4 prefix set with its entries."
}
