// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package show

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	radiusplugin "github.com/veesix-networks/osvbng/plugins/auth/radius"
)

func init() {
	show.RegisterFactory(NewServersHandler)
}

type ServersHandler struct{}

func NewServersHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &ServersHandler{}
}

func (h *ServersHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	provider := radiusplugin.GetProvider()
	if provider == nil {
		return nil, fmt.Errorf("RADIUS auth provider not initialized")
	}
	return provider.Stats().GetAllStatsSnapshot(), nil
}

func (h *ServersHandler) PathPattern() paths.Path {
	return paths.Path(radiusplugin.ShowServersPath)
}

func (h *ServersHandler) Dependencies() []paths.Path {
	return nil
}

func (h *ServersHandler) Summary() string {
	return "Show RADIUS server statistics"
}

func (h *ServersHandler) Description() string {
	return "Display statistics for all configured RADIUS servers including request and response counters."
}
