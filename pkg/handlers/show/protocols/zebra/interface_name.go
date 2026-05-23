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
	show.RegisterFactory(NewZebraInterfaceNameHandler)
}

type ZebraInterfaceNameHandler struct {
	routing *routing.Component
}

func NewZebraInterfaceNameHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &ZebraInterfaceNameHandler{routing: deps.Routing}
}

func (h *ZebraInterfaceNameHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	wildcards, err := paths.Extract(req.Path, paths.ProtocolsZebraInterfaceName)
	if err != nil {
		return nil, fmt.Errorf("extract interface name: %w", err)
	}
	if len(wildcards) != 1 {
		return nil, fmt.Errorf("invalid path: expected 1 wildcard, got %d", len(wildcards))
	}
	return h.routing.GetZebraInterfaces(wildcards[0])
}

func (h *ZebraInterfaceNameHandler) PathPattern() paths.Path {
	return paths.ProtocolsZebraInterfaceName
}
func (h *ZebraInterfaceNameHandler) Dependencies() []paths.Path { return nil }
func (h *ZebraInterfaceNameHandler) Summary() string {
	return "Show one zebra interface"
}
func (h *ZebraInterfaceNameHandler) Description() string {
	return "Display zebra's view of a single interface by name; honors `.` in interface names via the path encoding."
}
