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
)

func init() {
	show.RegisterFactory(NewFIBIPv6TableHandler)
}

type FIBIPv6TableHandler struct {
	southbound southbound.Southbound
}

func NewFIBIPv6TableHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &FIBIPv6TableHandler{southbound: deps.Southbound}
}

func (h *FIBIPv6TableHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.southbound == nil {
		return nil, fmt.Errorf("southbound not available")
	}
	tableID, err := extractTableID(req.Path, paths.ProtocolsFIBIPv6Table)
	if err != nil {
		return nil, err
	}
	return h.southbound.GetIPv6FIB(tableID)
}

func (h *FIBIPv6TableHandler) PathPattern() paths.Path {
	return paths.ProtocolsFIBIPv6Table
}

func (h *FIBIPv6TableHandler) Dependencies() []paths.Path {
	return nil
}

func (h *FIBIPv6TableHandler) Summary() string {
	return "Show VPP IPv6 FIB for one table"
}

func (h *FIBIPv6TableHandler) Description() string {
	return "Display the IPv6 forwarding table entries for the requested VPP table ID."
}
