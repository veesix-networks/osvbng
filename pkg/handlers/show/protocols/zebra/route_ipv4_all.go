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
	show.RegisterFactory(NewZebraRouteIPv4AllHandler)
}

type ZebraRouteIPv4AllHandler struct {
	routing *routing.Component
}

func NewZebraRouteIPv4AllHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &ZebraRouteIPv4AllHandler{routing: deps.Routing}
}

func (h *ZebraRouteIPv4AllHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetZebraRouteIPv4All()
}

func (h *ZebraRouteIPv4AllHandler) PathPattern() paths.Path    { return paths.ProtocolsZebraRouteIPv4All }
func (h *ZebraRouteIPv4AllHandler) Dependencies() []paths.Path { return nil }
func (h *ZebraRouteIPv4AllHandler) Summary() string {
	return "Show zebra IPv4 route table across all VRFs"
}
func (h *ZebraRouteIPv4AllHandler) Description() string {
	return "Display zebra's IPv4 RIB across every VRF. Each VRF is fetched independently because vtysh's `vrf all` form returns concatenated JSON objects; the wrapper merges into a single prefix-keyed map."
}
