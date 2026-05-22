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
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

func init() {
	show.RegisterFactory(NewOSPF6GRHelperHandler)
	telemetry.RegisterMetric[ospf6.GRHelper](paths.ProtocolsOSPF6GRHelper)
}

type OSPF6GRHelperHandler struct {
	routing *routing.Component
}

func NewOSPF6GRHelperHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPF6GRHelperHandler{routing: deps.Routing}
}

func (h *OSPF6GRHelperHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	detail, _ := strconv.ParseBool(req.Options["detail"])
	return h.routing.GetOSPF6GRHelper(req.Options["vrf"], detail)
}

func (h *OSPF6GRHelperHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPF6GRHelper
}

func (h *OSPF6GRHelperHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPF6GRHelperHandler) Summary() string {
	return "Show OSPFv3 graceful-restart helper state"
}

func (h *OSPF6GRHelperHandler) Description() string {
	return "Display OSPFv3 graceful-restart helper configuration; per-neighbor helper state when detail is set."
}

type OSPF6GRHelperOptions struct {
	VRF    string `query:"vrf" description:"VRF name; empty means the default routing table"`
	Detail bool   `query:"detail" description:"Include the per-neighbor helper state map"`
}

func (h *OSPF6GRHelperHandler) OptionsType() interface{} {
	return &OSPF6GRHelperOptions{}
}
