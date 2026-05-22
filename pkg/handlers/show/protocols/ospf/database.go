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
	show.RegisterFactory(NewOSPFDatabaseHandler)
}

type OSPFDatabaseHandler struct {
	routing *routing.Component
}

func NewOSPFDatabaseHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPFDatabaseHandler{routing: deps.Routing}
}

func (h *OSPFDatabaseHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	opts := ospf.DatabaseOpts{
		LinkStateID: req.Options["link_state_id"],
		AdvRouter:   req.Options["adv_router"],
	}
	opts.Detail, _ = strconv.ParseBool(req.Options["detail"])
	opts.SelfOriginate, _ = strconv.ParseBool(req.Options["self_originate"])
	opts.MaxAge, _ = strconv.ParseBool(req.Options["max_age"])
	return h.routing.GetOSPFDatabase(req.Options["vrf"], "", opts)
}

func (h *OSPFDatabaseHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPFDatabase
}

func (h *OSPFDatabaseHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPFDatabaseHandler) Summary() string {
	return "Show OSPFv2 link-state database summary"
}

func (h *OSPFDatabaseHandler) Description() string {
	return "Display the OSPFv2 LSDB summary across all LSA types. Response is forwarded raw from FRR; shape varies between basic and detail variants."
}

type OSPFDatabaseOptions struct {
	VRF           string `query:"vrf" description:"VRF name; empty means the default routing table, 'all' returns every VRF"`
	Detail        bool   `query:"detail" description:"Return full LSA bodies instead of the summary"`
	SelfOriginate bool   `query:"self_originate" description:"Restrict to self-originated LSAs"`
	MaxAge        bool   `query:"max_age" description:"Show only MaxAge LSAs"`
	LinkStateID   string `query:"link_state_id" description:"Filter by link-state ID (IPv4)"`
	AdvRouter     string `query:"adv_router" description:"Filter by advertising router (IPv4)"`
}

func (h *OSPFDatabaseHandler) OptionsType() interface{} {
	return &OSPFDatabaseOptions{}
}
