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
	show.RegisterFactory(NewPrefixSetHandler)
}

type PrefixSetHandler struct {
	running deps.RunningConfigReader
}

func NewPrefixSetHandler(d *deps.ShowDeps) show.ShowHandler {
	return &PrefixSetHandler{running: d.RunningConfig}
}

func (h *PrefixSetHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	wildcards, err := paths.Extract(req.Path, paths.RoutingPolicyPrefixSet)
	if err != nil || len(wildcards) != 1 {
		return nil, fmt.Errorf("extract prefix-set name: %w", err)
	}
	name := wildcards[0]

	cfg, err := h.running.GetRunning()
	if err != nil {
		return nil, fmt.Errorf("get running config: %w", err)
	}
	if cfg == nil || cfg.RoutingPolicies == nil || cfg.RoutingPolicies.PrefixSets == nil {
		return nil, fmt.Errorf("prefix-set %q not found", name)
	}

	set, ok := cfg.RoutingPolicies.PrefixSets[name]
	if !ok {
		return nil, fmt.Errorf("prefix-set %q not found", name)
	}
	return set, nil
}

func (h *PrefixSetHandler) PathPattern() paths.Path {
	return paths.RoutingPolicyPrefixSet
}

func (h *PrefixSetHandler) Dependencies() []paths.Path {
	return nil
}

func (h *PrefixSetHandler) Summary() string {
	return "Show a single IPv4 prefix set"
}

func (h *PrefixSetHandler) Description() string {
	return "Display the entries of a specific IPv4 prefix set by name."
}
