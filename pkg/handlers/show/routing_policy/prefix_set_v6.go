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
	show.RegisterFactory(NewPrefixSetV6Handler)
}

type PrefixSetV6Handler struct {
	running deps.RunningConfigReader
}

func NewPrefixSetV6Handler(d *deps.ShowDeps) show.ShowHandler {
	return &PrefixSetV6Handler{running: d.RunningConfig}
}

func (h *PrefixSetV6Handler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	wildcards, err := paths.Extract(req.Path, paths.RoutingPolicyPrefixSetV6)
	if err != nil || len(wildcards) != 1 {
		return nil, fmt.Errorf("extract prefix-set-v6 name: %w", err)
	}
	name := wildcards[0]

	cfg, err := h.running.GetRunning()
	if err != nil {
		return nil, fmt.Errorf("get running config: %w", err)
	}
	if cfg == nil || cfg.RoutingPolicies == nil || cfg.RoutingPolicies.PrefixSetsV6 == nil {
		return nil, fmt.Errorf("prefix-set-v6 %q not found", name)
	}

	set, ok := cfg.RoutingPolicies.PrefixSetsV6[name]
	if !ok {
		return nil, fmt.Errorf("prefix-set-v6 %q not found", name)
	}
	return set, nil
}

func (h *PrefixSetV6Handler) PathPattern() paths.Path {
	return paths.RoutingPolicyPrefixSetV6
}

func (h *PrefixSetV6Handler) Dependencies() []paths.Path {
	return nil
}

func (h *PrefixSetV6Handler) Summary() string {
	return "Show a single IPv6 prefix set"
}

func (h *PrefixSetV6Handler) Description() string {
	return "Display the entries of a specific IPv6 prefix set by name."
}
