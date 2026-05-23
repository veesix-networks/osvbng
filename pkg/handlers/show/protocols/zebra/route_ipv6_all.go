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
	show.RegisterFactory(NewZebraRouteIPv6AllHandler)
}

type ZebraRouteIPv6AllHandler struct {
	routing *routing.Component
}

func NewZebraRouteIPv6AllHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &ZebraRouteIPv6AllHandler{routing: deps.Routing}
}

func (h *ZebraRouteIPv6AllHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetZebraRouteIPv6All()
}

func (h *ZebraRouteIPv6AllHandler) PathPattern() paths.Path    { return paths.ProtocolsZebraRouteIPv6All }
func (h *ZebraRouteIPv6AllHandler) Dependencies() []paths.Path { return nil }
func (h *ZebraRouteIPv6AllHandler) Summary() string {
	return "Show zebra IPv6 route table across all VRFs"
}
func (h *ZebraRouteIPv6AllHandler) Description() string {
	return "Display zebra's IPv6 RIB across every VRF. The wrapper iterates per-VRF to work around FRR's concatenated-JSON `vrf all` output."
}
