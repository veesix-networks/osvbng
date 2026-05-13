// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/models"
)

func init() {
	show.RegisterFactory(NewTunnelsHandler)
}

type TunnelsHandler struct {
	deps *deps.ShowDeps
}

func NewTunnelsHandler(d *deps.ShowDeps) show.ShowHandler {
	return &TunnelsHandler{deps: d}
}

func (h *TunnelsHandler) Collect(_ context.Context, _ *show.Request) (interface{}, error) {
	if h.deps.L2TP == nil {
		return []models.L2TPTunnelSummary{}, nil
	}
	return h.deps.L2TP.SnapshotTunnels(), nil
}

func (h *TunnelsHandler) PathPattern() paths.Path {
	return paths.L2TPTunnels
}

func (h *TunnelsHandler) Dependencies() []paths.Path {
	return nil
}

func (h *TunnelsHandler) Summary() string {
	return "Show L2TPv2 control-channel tunnels"
}

func (h *TunnelsHandler) Description() string {
	return "List active L2TPv2 tunnels with local and peer IPs, tunnel IDs, role, FSM state, and bound session count. Per-subscriber detail is on 'show subscriber sessions' when state is tunneled."
}
