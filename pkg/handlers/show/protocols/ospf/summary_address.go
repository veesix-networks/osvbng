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
)

func init() {
	show.RegisterFactory(NewOSPFSummaryHandler)
}

type OSPFSummaryHandler struct {
	routing *routing.Component
}

func NewOSPFSummaryHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPFSummaryHandler{routing: deps.Routing}
}

func (h *OSPFSummaryHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	detail, _ := strconv.ParseBool(req.Options["detail"])
	return h.routing.GetOSPFSummaryAddress(req.Options["vrf"], detail)
}

func (h *OSPFSummaryHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPFSummary
}

func (h *OSPFSummaryHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPFSummaryHandler) Summary() string {
	return "Show OSPFv2 external-route summary-address configuration"
}

func (h *OSPFSummaryHandler) Description() string {
	return "Display the configured external-route summarisation entries and the aggregation delay timer."
}

type OSPFSummaryOptions struct {
	VRF    string `query:"vrf" description:"VRF name; empty means the default routing table, 'all' returns every VRF"`
	Detail bool   `query:"detail" description:"Include matched external LSAs per summary"`
}

func (h *OSPFSummaryHandler) OptionsType() interface{} {
	return &OSPFSummaryOptions{}
}
