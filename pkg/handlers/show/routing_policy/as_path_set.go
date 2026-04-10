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
	show.RegisterFactory(NewASPathSetHandler)
}

type ASPathSetHandler struct {
	running deps.RunningConfigReader
}

func NewASPathSetHandler(d *deps.ShowDeps) show.ShowHandler {
	return &ASPathSetHandler{running: d.RunningConfig}
}

func (h *ASPathSetHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	wildcards, err := paths.Extract(req.Path, paths.RoutingPolicyASPathSet)
	if err != nil || len(wildcards) != 1 {
		return nil, fmt.Errorf("extract as-path-set name: %w", err)
	}
	name := wildcards[0]

	cfg, err := h.running.GetRunning()
	if err != nil {
		return nil, fmt.Errorf("get running config: %w", err)
	}
	if cfg == nil || cfg.RoutingPolicies == nil || cfg.RoutingPolicies.ASPathSets == nil {
		return nil, fmt.Errorf("as-path-set %q not found", name)
	}

	set, ok := cfg.RoutingPolicies.ASPathSets[name]
	if !ok {
		return nil, fmt.Errorf("as-path-set %q not found", name)
	}
	return set, nil
}

func (h *ASPathSetHandler) PathPattern() paths.Path {
	return paths.RoutingPolicyASPathSet
}

func (h *ASPathSetHandler) Dependencies() []paths.Path {
	return nil
}

func (h *ASPathSetHandler) Summary() string {
	return "Show a single BGP AS path set"
}

func (h *ASPathSetHandler) Description() string {
	return "Display the regex entries of a specific BGP AS path set by name."
}
