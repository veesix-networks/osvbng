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
	show.RegisterFactory(NewOSPF6DatabaseLSAHandler)
}

type OSPF6DatabaseLSAHandler struct {
	routing *routing.Component
}

func NewOSPF6DatabaseLSAHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPF6DatabaseLSAHandler{routing: deps.Routing}
}

func (h *OSPF6DatabaseLSAHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	wildcards, err := paths.Extract(req.Path, paths.ProtocolsOSPF6DatabaseLSA)
	if err != nil {
		return nil, fmt.Errorf("extract LSA type: %w", err)
	}
	if len(wildcards) != 1 {
		return nil, fmt.Errorf("invalid path: expected 1 wildcard, got %d", len(wildcards))
	}
	opts := ospf6.DatabaseOpts{
		AdvRouter:   req.Options["adv_router"],
		LinkStateID: req.Options["link_state_id"],
	}
	opts.SelfOriginated, _ = strconv.ParseBool(req.Options["self_originated"])
	return h.routing.GetOSPF6Database(req.Options["vrf"], wildcards[0], opts)
}

func (h *OSPF6DatabaseLSAHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPF6DatabaseLSA
}

func (h *OSPF6DatabaseLSAHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPF6DatabaseLSAHandler) Summary() string {
	return "Show OSPFv3 LSDB entries for one LSA type"
}

func (h *OSPF6DatabaseLSAHandler) Description() string {
	return "Display LSDB entries for one LSA type (router, network, inter-prefix, inter-router, as-external, group-membership, type-7, link, intra-prefix). Response forwarded as-is from FRR."
}

type OSPF6DatabaseLSAOptions struct {
	VRF            string `query:"vrf" description:"VRF name; empty means the default routing table"`
	SelfOriginated bool   `query:"self_originated" description:"Restrict to self-originated LSAs"`
	AdvRouter      string `query:"adv_router" description:"Filter by advertising router (IPv4)"`
	LinkStateID    string `query:"link_state_id" description:"Filter by link-state ID (IPv4)"`
}

func (h *OSPF6DatabaseLSAHandler) OptionsType() interface{} {
	return &OSPF6DatabaseLSAOptions{}
}
