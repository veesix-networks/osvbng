// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewBGPIPv6UnicastSummaryHandler)
}

type BGPIPv6UnicastSummaryHandler struct {
	routing *routing.Component
}

func NewBGPIPv6UnicastSummaryHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPIPv6UnicastSummaryHandler{routing: deps.Routing}
}

func (h *BGPIPv6UnicastSummaryHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPAFISummary(req.Options["vrf"], "ipv6")
}

func (h *BGPIPv6UnicastSummaryHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPIPv6UnicastSummary
}

func (h *BGPIPv6UnicastSummaryHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPIPv6UnicastSummaryHandler) Summary() string {
	return "Show BGP IPv6 unicast summary"
}

func (h *BGPIPv6UnicastSummaryHandler) Description() string {
	return "Display BGP IPv6 unicast summary including peer table, table version, RIB and peer memory counters."
}

type BGPIPv6UnicastSummaryOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *BGPIPv6UnicastSummaryHandler) OptionsType() interface{} {
	return &BGPIPv6UnicastSummaryOptions{}
}
