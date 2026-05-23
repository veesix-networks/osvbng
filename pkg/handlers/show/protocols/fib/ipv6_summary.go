// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package fib

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

func init() {
	show.RegisterFactory(NewFIBIPv6SummaryHandler)
	telemetry.RegisterMetric[southbound.IPFIBSummaryAll](paths.ProtocolsFIBIPv6Summary)
}

type FIBIPv6SummaryHandler struct {
	southbound southbound.Southbound
}

func NewFIBIPv6SummaryHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &FIBIPv6SummaryHandler{southbound: deps.Southbound}
}

func (h *FIBIPv6SummaryHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.southbound == nil {
		return nil, fmt.Errorf("southbound not available")
	}
	return h.southbound.GetIPv6FIBSummary()
}

func (h *FIBIPv6SummaryHandler) PathPattern() paths.Path {
	return paths.ProtocolsFIBIPv6Summary
}

func (h *FIBIPv6SummaryHandler) Dependencies() []paths.Path {
	return nil
}

func (h *FIBIPv6SummaryHandler) Summary() string {
	return "Show per-table IPv6 FIB entry counts"
}

func (h *FIBIPv6SummaryHandler) Description() string {
	return "Display the number of IPv6 FIB entries per VPP table; backs Prometheus per-table FIB-size metrics."
}
