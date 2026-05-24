// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package system

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

// RecoveryStatusHandler exposes the per-component readiness lifecycle
// state at /api/show/system/recovery/status. Operators use it during
// osvbngd restart and VPP recovery to see whether IPoE / PPPoE / CGNAT
// have finished replaying opdb checkpoints and are accepting new
// subscriber traffic.
type RecoveryStatusHandler struct {
	orch *component.Orchestrator
}

// RecoveryStatus is the JSON body of the response: a map of component
// name -> readiness state. Empty components map (Orchestrator unset)
// means recovery tracking is not wired in this build.
type RecoveryStatus struct {
	AllReady   bool                                     `json:"all_ready"`
	Components map[string]component.ComponentReadiness `json:"components"`
}

func init() {
	show.RegisterFactory(func(d *deps.ShowDeps) show.ShowHandler {
		return &RecoveryStatusHandler{orch: d.Orchestrator}
	})
}

func (h *RecoveryStatusHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.orch == nil {
		return &RecoveryStatus{
			AllReady:   false,
			Components: map[string]component.ComponentReadiness{},
		}, nil
	}
	return &RecoveryStatus{
		AllReady:   h.orch.AllReady(),
		Components: h.orch.ReadinessSnapshot(),
	}, nil
}

func (h *RecoveryStatusHandler) PathPattern() paths.Path {
	return paths.SystemRecoveryStatus
}

func (h *RecoveryStatusHandler) Dependencies() []paths.Path {
	return nil
}

func (h *RecoveryStatusHandler) Summary() string {
	return "Show component recovery / readiness status"
}

func (h *RecoveryStatusHandler) Description() string {
	return "Display per-component readiness state across the recovery lifecycle. Components transition not_ready -> restoring -> ready as they replay opdb checkpoints into the dataplane on startup; reports the aggregated all_ready flag for quick health checks."
}
