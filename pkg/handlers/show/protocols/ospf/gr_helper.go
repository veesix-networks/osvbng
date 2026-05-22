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
	show.RegisterFactory(NewOSPFGRHelperHandler)
}

type OSPFGRHelperHandler struct {
	routing *routing.Component
}

func NewOSPFGRHelperHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPFGRHelperHandler{routing: deps.Routing}
}

func (h *OSPFGRHelperHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	detail, _ := strconv.ParseBool(req.Options["detail"])
	return h.routing.GetOSPFGRHelper(req.Options["vrf"], detail)
}

func (h *OSPFGRHelperHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPFGRHelper
}

func (h *OSPFGRHelperHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPFGRHelperHandler) Summary() string {
	return "Show OSPFv2 graceful-restart helper state"
}

func (h *OSPFGRHelperHandler) Description() string {
	return "Display OSPFv2 graceful-restart helper configuration and the per-neighbor helper status when detail is set."
}

type OSPFGRHelperOptions struct {
	VRF    string `query:"vrf" description:"VRF name; empty means the default routing table, 'all' returns every VRF"`
	Detail bool   `query:"detail" description:"Include the per-neighbor helper state map"`
}

func (h *OSPFGRHelperHandler) OptionsType() interface{} {
	return &OSPFGRHelperOptions{}
}
