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
	show.RegisterFactory(NewPrefixSetsV6Handler)
}

type PrefixSetsV6Handler struct {
	running deps.RunningConfigReader
}

func NewPrefixSetsV6Handler(d *deps.ShowDeps) show.ShowHandler {
	return &PrefixSetsV6Handler{running: d.RunningConfig}
}

func (h *PrefixSetsV6Handler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	cfg, err := h.running.GetRunning()
	if err != nil {
		return nil, fmt.Errorf("get running config: %w", err)
	}
	if cfg == nil || cfg.RoutingPolicies == nil {
		return map[string]rp.PrefixSet{}, nil
	}
	if cfg.RoutingPolicies.PrefixSetsV6 == nil {
		return map[string]rp.PrefixSet{}, nil
	}
	return cfg.RoutingPolicies.PrefixSetsV6, nil
}

func (h *PrefixSetsV6Handler) PathPattern() paths.Path {
	return paths.RoutingPolicyPrefixSetsV6
}

func (h *PrefixSetsV6Handler) Dependencies() []paths.Path {
	return nil
}

func (h *PrefixSetsV6Handler) Summary() string {
	return "Show all IPv6 prefix sets"
}

func (h *PrefixSetsV6Handler) Description() string {
	return "List every configured IPv6 prefix set with its entries."
}
