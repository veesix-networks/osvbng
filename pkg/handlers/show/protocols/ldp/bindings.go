// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ldp

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/models/protocols/ldp"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

func init() {
	show.RegisterFactory(NewLDPBindingsHandler)
	telemetry.RegisterMetric[ldp.Binding](paths.ProtocolsLDPBindings)
}

type LDPBindingsHandler struct {
	routing *routing.Component
}

func NewLDPBindingsHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &LDPBindingsHandler{routing: deps.Routing}
}

func (h *LDPBindingsHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetLDPBindings(req.Options["afi"], routing.BindingFilter{
		Neighbor:    req.Options["neighbor"],
		LocalLabel:  req.Options["local_label"],
		RemoteLabel: req.Options["remote_label"],
	})
}

func (h *LDPBindingsHandler) PathPattern() paths.Path {
	return paths.ProtocolsLDPBindings
}

func (h *LDPBindingsHandler) Dependencies() []paths.Path {
	return nil
}

func (h *LDPBindingsHandler) Summary() string {
	return "Show LDP label bindings"
}

func (h *LDPBindingsHandler) Description() string {
	return "Display LDP label-to-prefix bindings from FRR."
}

type LDPBindingsOptions struct {
	AFI         string `query:"afi" description:"Address family: ipv4 or ipv6; empty means both"`
	Neighbor    string `query:"neighbor" description:"Filter by LDP neighbor LSR-ID"`
	LocalLabel  string `query:"local_label" description:"Filter by local label (numeric or imp-null)"`
	RemoteLabel string `query:"remote_label" description:"Filter by remote label (numeric or imp-null)"`
}

func (h *LDPBindingsHandler) OptionsType() interface{} {
	return &LDPBindingsOptions{}
}
