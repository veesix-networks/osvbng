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
	show.RegisterFactory(NewZebraRouteIPv6Handler)
}

type ZebraRouteIPv6Handler struct {
	routing *routing.Component
}

func NewZebraRouteIPv6Handler(deps *deps.ShowDeps) show.ShowHandler {
	return &ZebraRouteIPv6Handler{routing: deps.Routing}
}

func (h *ZebraRouteIPv6Handler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetZebraRouteIPv6(req.Options["vrf"])
}

func (h *ZebraRouteIPv6Handler) PathPattern() paths.Path    { return paths.ProtocolsZebraRouteIPv6 }
func (h *ZebraRouteIPv6Handler) Dependencies() []paths.Path { return nil }
func (h *ZebraRouteIPv6Handler) Summary() string            { return "Show zebra IPv6 route table" }
func (h *ZebraRouteIPv6Handler) Description() string {
	return "Display zebra's IPv6 RIB; honors req.Options[\"vrf\"]. Response forwarded as-is from FRR."
}

type ZebraRouteIPv6Options struct {
	VRF string `query:"vrf" description:"VRF name; empty means the default routing table"`
}

func (h *ZebraRouteIPv6Handler) OptionsType() interface{} { return &ZebraRouteIPv6Options{} }
