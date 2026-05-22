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
)

func init() {
	show.RegisterFactory(NewOSPF6SummaryHandler)
}

type OSPF6SummaryHandler struct {
	routing *routing.Component
}

func NewOSPF6SummaryHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPF6SummaryHandler{routing: deps.Routing}
}

func (h *OSPF6SummaryHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	detail, _ := strconv.ParseBool(req.Options["detail"])
	return h.routing.GetOSPF6SummaryAddress(detail)
}

func (h *OSPF6SummaryHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPF6Summary
}

func (h *OSPF6SummaryHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPF6SummaryHandler) Summary() string {
	return "Show OSPFv3 external-route summary-address configuration"
}

func (h *OSPF6SummaryHandler) Description() string {
	return "Display OSPFv3 summary-address entries and aggregation delay; response forwarded as-is from FRR."
}

type OSPF6SummaryOptions struct {
	Detail bool `query:"detail" description:"Include matched external LSAs per summary"`
}

func (h *OSPF6SummaryHandler) OptionsType() interface{} {
	return &OSPF6SummaryOptions{}
}
