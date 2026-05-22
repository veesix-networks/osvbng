// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf6

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewOSPF6ZebraHandler)
}

type OSPF6ZebraHandler struct {
	routing *routing.Component
}

func NewOSPF6ZebraHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPF6ZebraHandler{routing: deps.Routing}
}

func (h *OSPF6ZebraHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetOSPF6Zebra()
}

func (h *OSPF6ZebraHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPF6Zebra
}

func (h *OSPF6ZebraHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPF6ZebraHandler) Summary() string {
	return "Show OSPFv3 zebra interaction state"
}

func (h *OSPF6ZebraHandler) Description() string {
	return "Display OSPFv3 zebra-client state; response forwarded as-is from FRR."
}
