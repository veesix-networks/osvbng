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
	show.RegisterFactory(NewBGPIPv4UnicastSummaryHandler)
}

type BGPIPv4UnicastSummaryHandler struct {
	routing *routing.Component
}

func NewBGPIPv4UnicastSummaryHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPIPv4UnicastSummaryHandler{routing: deps.Routing}
}

func (h *BGPIPv4UnicastSummaryHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPAFISummary(req.Options["vrf"], "ipv4")
}

func (h *BGPIPv4UnicastSummaryHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPIPv4UnicastSummary
}

func (h *BGPIPv4UnicastSummaryHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPIPv4UnicastSummaryHandler) Summary() string {
	return "Show BGP IPv4 unicast summary"
}

func (h *BGPIPv4UnicastSummaryHandler) Description() string {
	return "Display BGP IPv4 unicast summary including peer table, table version, RIB and peer memory counters."
}

type BGPIPv4UnicastSummaryOptions struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *BGPIPv4UnicastSummaryHandler) OptionsType() interface{} {
	return &BGPIPv4UnicastSummaryOptions{}
}
