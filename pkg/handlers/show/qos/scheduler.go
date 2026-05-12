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

func (h *SchedulerHandler) Collect(_ context.Context, _ *show.Request) (interface{}, error) {
	if h.deps.Southbound == nil {
		return nil, fmt.Errorf("southbound not available")
	}

	return h.deps.Southbound.DumpSchedulers()
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
	return "Display all QoS scheduler policies configured in the dataplane."
}

func (h *SchedulerHandler) SortKey() string {
	return "sw_if_index"
}
