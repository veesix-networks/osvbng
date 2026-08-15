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
	"github.com/veesix-networks/osvbng/pkg/models"
)

func init() {
	show.RegisterFactory(func(d *deps.ShowDeps) show.ShowHandler {
		return &SchedulerDetailHandler{deps: d}
	})
}

// SchedulerDetailHandler is the interface-keyed twin of the session view:
// same output, addressed by the scheduler's own interface rather than by
// subscriber identity.
type SchedulerDetailHandler struct {
	deps *deps.ShowDeps
}

type SchedulerDetailOptions struct {
	Interface string `query:"interface" description:"Session interface name or sw_if_index"`
}

func (h *SchedulerDetailHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.deps.Southbound == nil {
		return nil, fmt.Errorf("southbound not available")
	}

	ifOpt := req.Options["interface"]
	if ifOpt == "" {
		return nil, fmt.Errorf("--interface is required")
	}
	swIfIndex, err := resolveIfIndex(h.deps, ifOpt)
	if err != nil {
		return nil, err
	}

	// The reverse index turns the interface back into a session so the view
	// carries subscriber identity when there is one.
	var sess models.SubscriberSession
	if h.deps.Subscriber != nil {
		if id, ok := h.deps.Subscriber.SessionIDByIfIndex(swIfIndex); ok {
			sess, _ = h.deps.Subscriber.SessionSnapshot(ctx, id)
		}
	}

	return buildSchedulerView(h.deps, sess, swIfIndex)
}

func (h *SchedulerDetailHandler) PathPattern() paths.Path {
	return paths.QoSSchedulerDetail
}

func (h *SchedulerDetailHandler) Dependencies() []paths.Path {
	return nil
}

func (h *SchedulerDetailHandler) Summary() string {
	return "Show one QoS scheduler with its shaping hierarchy"
}

func (h *SchedulerDetailHandler) Description() string {
	return "Display a single CAKE scheduler by interface, with per-tin stats, DRR state, the owning session when known, and the aggregate tiers above it."
}

func (h *SchedulerDetailHandler) OptionsType() interface{} {
	return &SchedulerDetailOptions{}
}
