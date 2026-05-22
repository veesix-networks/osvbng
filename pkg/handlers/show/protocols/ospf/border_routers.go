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
	show.RegisterFactory(NewOSPFBorderRoutersHandler)
}

type OSPFBorderRoutersHandler struct {
	routing *routing.Component
}

func NewOSPFBorderRoutersHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPFBorderRoutersHandler{routing: deps.Routing}
}

func (h *OSPFBorderRoutersHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetOSPFBorderRouters(req.Options["vrf"])
}

func (h *OSPFBorderRoutersHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPFBorderRouters
}

func (h *OSPFBorderRoutersHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPFBorderRoutersHandler) Summary() string {
	return "Show OSPFv2 ABR and ASBR border routers"
}

func (h *OSPFBorderRoutersHandler) Description() string {
	return "Display the list of ABR and ASBR border routers learnt via Type-3 (Summary) and Type-4 (ASBR-Summary) LSAs."
}

type OSPFBorderRoutersOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table, 'all' returns every VRF"`
}

func (h *OSPFBorderRoutersHandler) OptionsType() interface{} {
	return &OSPFBorderRoutersOptions{}
}
