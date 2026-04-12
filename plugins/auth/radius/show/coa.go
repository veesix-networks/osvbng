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
	radiusplugin "github.com/veesix-networks/osvbng/plugins/auth/radius"
)

func init() {
	show.RegisterFactory(NewCoAHandler)
}

type CoAHandler struct {
	deps *deps.ShowDeps
}

func NewCoAHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &CoAHandler{deps: deps}
}

func (h *CoAHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	comp, ok := h.deps.PluginComponents[radiusplugin.CoANamespace]
	if !ok {
		return nil, fmt.Errorf("RADIUS CoA component not loaded")
	}
	coaComp, ok := comp.(*radiusplugin.CoAComponent)
	if !ok {
		return nil, fmt.Errorf("invalid CoA component type")
	}
	return coaComp.GetStats().GetAllStatsSnapshot(), nil
}

func (h *CoAHandler) PathPattern() paths.Path {
	return paths.Path(radiusplugin.ShowCoAPath)
}

func (h *CoAHandler) Dependencies() []paths.Path {
	return nil
}

func (h *CoAHandler) Summary() string {
	return "Show RADIUS CoA statistics"
}

func (h *CoAHandler) Description() string {
	return "Display per-client statistics for RADIUS CoA and Disconnect-Message handling."
}
