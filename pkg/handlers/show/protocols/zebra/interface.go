// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package zebra

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewZebraInterfaceHandler)
}

type ZebraInterfaceHandler struct {
	routing *routing.Component
}

func NewZebraInterfaceHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &ZebraInterfaceHandler{routing: deps.Routing}
}

func (h *ZebraInterfaceHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetZebraInterfaces("")
}

func (h *ZebraInterfaceHandler) PathPattern() paths.Path    { return paths.ProtocolsZebraInterface }
func (h *ZebraInterfaceHandler) Dependencies() []paths.Path { return nil }
func (h *ZebraInterfaceHandler) Summary() string {
	return "Show zebra interface table"
}
func (h *ZebraInterfaceHandler) Description() string {
	return "Display every interface as zebra sees it, including VRF binding, MTU, hardware address, and IP addresses. Response forwarded as-is from FRR."
}
