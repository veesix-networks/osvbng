// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package zebra

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewZebraRouteIPv4Handler)
}

type ZebraRouteIPv4Handler struct {
	routing *routing.Component
}

func NewZebraRouteIPv4Handler(deps *deps.ShowDeps) show.ShowHandler {
	return &ZebraRouteIPv4Handler{routing: deps.Routing}
}

func (h *ZebraRouteIPv4Handler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetZebraRouteIPv4(req.Options["vrf"])
}

func (h *ZebraRouteIPv4Handler) PathPattern() paths.Path    { return paths.ProtocolsZebraRouteIPv4 }
func (h *ZebraRouteIPv4Handler) Dependencies() []paths.Path { return nil }
func (h *ZebraRouteIPv4Handler) Summary() string            { return "Show zebra IPv4 route table" }
func (h *ZebraRouteIPv4Handler) Description() string {
	return "Display zebra's IPv4 RIB; honors req.Options[\"vrf\"]. Response forwarded as-is from FRR."
}

type ZebraRouteIPv4Options struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *ZebraRouteIPv4Handler) OptionsType() interface{} { return &ZebraRouteIPv4Options{} }
