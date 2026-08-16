// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package qos

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

func init() {
	show.RegisterFactory(func(d *deps.ShowDeps) show.ShowHandler {
		return &SchedulerHandler{deps: d}
	})
	telemetry.RegisterMetric[southbound.SchedulerState](paths.QoSScheduler)
}

type SchedulerHandler struct {
	deps *deps.ShowDeps
}

// Collect with no options returns every scheduler: the telemetry poller
// calls this path with an empty request, so filters are subtractive only.
func (h *SchedulerHandler) Collect(_ context.Context, req *show.Request) (interface{}, error) {
	if h.deps.Southbound == nil {
		return nil, fmt.Errorf("southbound not available")
	}

	states, err := h.deps.Southbound.DumpSchedulers()
	if err != nil {
		return nil, err
	}
	enrichSessionIDs(h.deps, states)

	if ifOpt := req.Options["interface"]; ifOpt != "" {
		swIfIndex, err := resolveIfIndex(h.deps, ifOpt)
		if err != nil {
			return nil, err
		}
		filtered := states[:0]
		for _, s := range states {
			if s.SwIfIndex == swIfIndex {
				filtered = append(filtered, s)
			}
		}
		states = filtered
	}

	return states, nil
}

type SchedulerListOptions struct {
	Interface string `query:"interface" description:"Limit to one session interface, by name or sw_if_index"`
}

func (h *SchedulerHandler) OptionsType() interface{} {
	return &SchedulerListOptions{}
}

func (h *SchedulerHandler) PathPattern() paths.Path {
	return paths.QoSScheduler
}

func (h *SchedulerHandler) Dependencies() []paths.Path {
	return nil
}

func (h *SchedulerHandler) Summary() string {
	return "Show QoS scheduler policies"
}

func (h *SchedulerHandler) Description() string {
	return "Display all QoS scheduler policies configured in the dataplane. " +
		"The CLI's compact table abbreviates tin modes (be=besteffort, ds3/ds4/ds8=diffserv3/4/8) " +
		"and shows W(EFF) as weight(effective DRR weight), ST as A=active in the parent's " +
		"arbitration, BUF as buffer usage percent, and BLK D/P as drr_blocked/parent_blocked. " +
		"Use '| json' for the full field set."
}

func (h *SchedulerHandler) SortKey() string {
	return "sw_if_index"
}
