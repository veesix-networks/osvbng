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
	show.RegisterFactory(NewOSPFMPLSTEInterfaceHandler)
}

type OSPFMPLSTEInterfaceHandler struct {
	routing *routing.Component
}

func NewOSPFMPLSTEInterfaceHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPFMPLSTEInterfaceHandler{routing: deps.Routing}
}

func (h *OSPFMPLSTEInterfaceHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetOSPFMPLSTEInterface(req.Options["interface"])
}

func (h *OSPFMPLSTEInterfaceHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPFMPLSTEInterface
}

func (h *OSPFMPLSTEInterfaceHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPFMPLSTEInterfaceHandler) Summary() string {
	return "Show OSPFv2 MPLS-TE interface state"
}

func (h *OSPFMPLSTEInterfaceHandler) Description() string {
	return "Display MPLS-TE parameters for OSPF interfaces. FRR returns plain text; the response is forwarded as-is."
}

type OSPFMPLSTEInterfaceOptions struct {
	Interface string `query:"interface" description:"Restrict output to one interface name"`
}

func (h *OSPFMPLSTEInterfaceHandler) OptionsType() interface{} {
	return &OSPFMPLSTEInterfaceOptions{}
}
