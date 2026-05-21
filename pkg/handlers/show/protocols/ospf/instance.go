// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewOSPFInstanceHandler)
}

type OSPFInstanceHandler struct {
	routing *routing.Component
}

func NewOSPFInstanceHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPFInstanceHandler{routing: deps.Routing}
}

func (h *OSPFInstanceHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetOSPFInstance(req.Options["vrf"])
}

func (h *OSPFInstanceHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPF
}

func (h *OSPFInstanceHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPFInstanceHandler) Summary() string {
	return "Show OSPFv2 instance state"
}

func (h *OSPFInstanceHandler) Description() string {
	return "Display OSPFv2 router state, SPF timing, LSA counters, and per-area LSDB summary."
}

type OSPFInstanceOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table, 'all' returns every VRF"`
}

func (h *OSPFInstanceHandler) OptionsType() interface{} {
	return &OSPFInstanceOptions{}
}
