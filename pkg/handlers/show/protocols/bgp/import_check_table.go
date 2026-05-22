// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

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
	show.RegisterFactory(NewBGPImportCheckTableHandler)
}

type BGPImportCheckTableHandler struct {
	routing *routing.Component
}

func NewBGPImportCheckTableHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPImportCheckTableHandler{routing: deps.Routing}
}

func (h *BGPImportCheckTableHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	detail, _ := strconv.ParseBool(req.Options["detail"])
	return h.routing.GetBGPImportCheckTable(req.Options["vrf"], detail)
}

func (h *BGPImportCheckTableHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPImportCheckTable
}

func (h *BGPImportCheckTableHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPImportCheckTableHandler) Summary() string {
	return "Show BGP import-check table"
}

func (h *BGPImportCheckTableHandler) Description() string {
	return "Display BGP import-check table entries; response forwarded as-is from FRR."
}

type BGPImportCheckTableOptions struct {
	VRF    string `query:"vrf" description:"VRF name; empty means the default routing table"`
	Detail bool   `query:"detail" description:"Return detailed import-check entries"`
}

func (h *BGPImportCheckTableHandler) OptionsType() interface{} {
	return &BGPImportCheckTableOptions{}
}
