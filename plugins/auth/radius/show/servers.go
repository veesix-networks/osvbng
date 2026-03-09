// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package show

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/state"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
	radiusplugin "github.com/veesix-networks/osvbng/plugins/auth/radius"
)

func init() {
	show.RegisterFactory(NewServersHandler)
	state.RegisterMetric(statepaths.AAARadiusServers, paths.Path(radiusplugin.ShowServersPath))
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
