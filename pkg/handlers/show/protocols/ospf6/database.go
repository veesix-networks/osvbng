// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf6

import (
	"context"
	"fmt"
	"strconv"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/models/protocols/ospf6"
)

func init() {
	show.RegisterFactory(NewOSPF6DatabaseHandler)
}

type OSPF6DatabaseHandler struct {
	routing *routing.Component
}

func NewOSPF6DatabaseHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPF6DatabaseHandler{routing: deps.Routing}
}

func (h *OSPF6DatabaseHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	opts := ospf6.DatabaseOpts{
		AdvRouter:   req.Options["adv_router"],
		LinkStateID: req.Options["link_state_id"],
	}
	opts.Detail, _ = strconv.ParseBool(req.Options["detail"])
	opts.Dump, _ = strconv.ParseBool(req.Options["dump"])
	opts.Internal, _ = strconv.ParseBool(req.Options["internal"])
	opts.SelfOriginated, _ = strconv.ParseBool(req.Options["self_originated"])
	return h.routing.GetOSPF6Database(req.Options["vrf"], "", opts)
}

func (h *OSPF6DatabaseHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPF6Database
}

func (h *OSPF6DatabaseHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPF6DatabaseHandler) Summary() string {
	return "Show OSPFv3 link-state database summary"
}

func (h *OSPF6DatabaseHandler) Description() string {
	return "Display the OSPFv3 LSDB across all LSA types; response forwarded as-is from FRR."
}

type OSPF6DatabaseOptions struct {
	VRF            string `query:"vrf" description:"VRF name; empty means the default routing table"`
	Detail         bool   `query:"detail" description:"Return full LSA bodies"`
	Dump           bool   `query:"dump" description:"Return the raw LSDB dump"`
	Internal       bool   `query:"internal" description:"Return internal LSA representation"`
	SelfOriginated bool   `query:"self_originated" description:"Restrict to self-originated LSAs"`
	AdvRouter      string `query:"adv_router" description:"Filter by advertising router (IPv4)"`
	LinkStateID    string `query:"link_state_id" description:"Filter by link-state ID (IPv4)"`
}

func (h *OSPF6DatabaseHandler) OptionsType() interface{} {
	return &OSPF6DatabaseOptions{}
}
