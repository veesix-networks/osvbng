// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf

import (
	"context"
	"fmt"
	"strconv"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/models/protocols/ospf"
)

func init() {
	show.RegisterFactory(NewOSPFMPLSTEDatabaseHandler)
}

type OSPFMPLSTEDatabaseHandler struct {
	routing *routing.Component
}

func NewOSPFMPLSTEDatabaseHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPFMPLSTEDatabaseHandler{routing: deps.Routing}
}

func (h *OSPFMPLSTEDatabaseHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	opts := ospf.MPLSTEDatabaseOpts{
		Scope:     req.Options["scope"],
		LSID:      req.Options["link_state_id"],
		AdvRouter: req.Options["adv_router"],
	}
	opts.Verbose, _ = strconv.ParseBool(req.Options["verbose"])
	return h.routing.GetOSPFMPLSTEDatabase(opts)
}

func (h *OSPFMPLSTEDatabaseHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPFMPLSTEDatabase
}

func (h *OSPFMPLSTEDatabaseHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPFMPLSTEDatabaseHandler) Summary() string {
	return "Show OSPFv2 MPLS-TE database"
}

func (h *OSPFMPLSTEDatabaseHandler) Description() string {
	return "Display the MPLS-TE database (vertex, edge, or subnet subviews via the scope option). FRR returns plain text when MPLS-TE is disabled and JSON otherwise; the response is forwarded as-is."
}

type OSPFMPLSTEDatabaseOptions struct {
	Scope       string `query:"scope" description:"Restrict to one of vertex, edge, subnet"`
	LinkStateID string `query:"link_state_id" description:"Filter by link-state ID (edge: A.B.C.D, subnet: A.B.C.D/M)"`
	AdvRouter   string `query:"adv_router" description:"Filter by advertising router (IPv4); vertex scope only"`
	Verbose     bool   `query:"verbose" description:"Return verbose output"`
}

func (h *OSPFMPLSTEDatabaseHandler) OptionsType() interface{} {
	return &OSPFMPLSTEDatabaseOptions{}
}
