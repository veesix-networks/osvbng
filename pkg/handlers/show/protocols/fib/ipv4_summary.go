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
	show.RegisterFactory(NewFIBIPv4SummaryHandler)
	telemetry.RegisterMetric[southbound.IPFIBSummaryAll](paths.ProtocolsFIBIPv4Summary)
}

type FIBIPv4SummaryHandler struct {
	southbound southbound.Southbound
}

func NewFIBIPv4SummaryHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &FIBIPv4SummaryHandler{southbound: deps.Southbound}
}

func (h *FIBIPv4SummaryHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.southbound == nil {
		return nil, fmt.Errorf("southbound not available")
	}
	return h.southbound.GetIPv4FIBSummary()
}

func (h *FIBIPv4SummaryHandler) PathPattern() paths.Path {
	return paths.ProtocolsFIBIPv4Summary
}

func (h *FIBIPv4SummaryHandler) Dependencies() []paths.Path {
	return nil
}

func (h *FIBIPv4SummaryHandler) Summary() string {
	return "Show per-table IPv4 FIB entry counts"
}

func (h *FIBIPv4SummaryHandler) Description() string {
	return "Display the number of IPv4 FIB entries per VPP table; backs Prometheus per-table FIB-size metrics."
}
