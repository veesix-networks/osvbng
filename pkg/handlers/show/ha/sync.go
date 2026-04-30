// Copyright 2025 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/ha"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

func init() {
	show.RegisterFactory(func(d *deps.ShowDeps) show.ShowHandler {
		return &SyncHandler{deps: d}
	})
	telemetry.RegisterMetricMulti[ha.SyncSRGStatus](paths.HASync)
}

type SyncHandler struct {
	deps *deps.ShowDeps
}

func (h *SyncHandler) Collect(_ context.Context, _ *show.Request) (interface{}, error) {
	if h.deps.HAManager == nil {
		return []interface{}{}, nil
	}
	return h.deps.HAManager.GetSyncStatus(), nil
}

func (h *SyncHandler) PathPattern() paths.Path {
	return paths.HASync
}

func (h *SyncHandler) Dependencies() []paths.Path {
	return nil
}

func (h *SyncHandler) Summary() string {
	return "Show HA session sync status"
}

func (h *SyncHandler) Description() string {
	return "Display the current session synchronization status between HA peers."
}
