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
	show.RegisterFactory(NewOSPFDatabaseLSAHandler)
}

type OSPFDatabaseLSAHandler struct {
	routing *routing.Component
}

func NewOSPFDatabaseLSAHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPFDatabaseLSAHandler{routing: deps.Routing}
}

func (h *OSPFDatabaseLSAHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	wildcards, err := paths.Extract(req.Path, paths.ProtocolsOSPFDatabaseLSA)
	if err != nil {
		return nil, fmt.Errorf("extract LSA type: %w", err)
	}
	if len(wildcards) != 1 {
		return nil, fmt.Errorf("invalid path: expected 1 wildcard, got %d", len(wildcards))
	}
	opts := ospf.DatabaseOpts{
		LinkStateID: req.Options["link_state_id"],
		AdvRouter:   req.Options["adv_router"],
	}
	opts.SelfOriginate, _ = strconv.ParseBool(req.Options["self_originate"])
	return h.routing.GetOSPFDatabase(req.Options["vrf"], wildcards[0], opts)
}

func (h *OSPFDatabaseLSAHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPFDatabaseLSA
}

func (h *OSPFDatabaseLSAHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPFDatabaseLSAHandler) Summary() string {
	return "Show OSPFv2 LSDB entries for one LSA type"
}

func (h *OSPFDatabaseLSAHandler) Description() string {
	return "Display LSDB entries for one LSA type (router, network, summary, asbr-summary, external, nssa-external, opaque-link, opaque-area, opaque-as). Response is forwarded raw from FRR."
}

type OSPFDatabaseLSAOptions struct {
	VRF           string `query:"vrf" description:"VRF name; empty means the default routing table, 'all' returns every VRF"`
	SelfOriginate bool   `query:"self_originate" description:"Restrict to self-originated LSAs"`
	LinkStateID   string `query:"link_state_id" description:"Filter by link-state ID (IPv4)"`
	AdvRouter     string `query:"adv_router" description:"Filter by advertising router (IPv4)"`
}

func (h *OSPFDatabaseLSAHandler) OptionsType() interface{} {
	return &OSPFDatabaseLSAOptions{}
}
