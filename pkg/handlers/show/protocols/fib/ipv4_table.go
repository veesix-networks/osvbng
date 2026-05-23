// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package fib

import (
	"context"
	"fmt"
	"strconv"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	show.RegisterFactory(NewFIBIPv4TableHandler)
}

type FIBIPv4TableHandler struct {
	southbound southbound.Southbound
}

func NewFIBIPv4TableHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &FIBIPv4TableHandler{southbound: deps.Southbound}
}

func (h *FIBIPv4TableHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.southbound == nil {
		return nil, fmt.Errorf("southbound not available")
	}
	tableID, err := extractTableID(req.Path, paths.ProtocolsFIBIPv4Table)
	if err != nil {
		return nil, err
	}
	return h.southbound.GetIPv4FIB(tableID)
}

func (h *FIBIPv4TableHandler) PathPattern() paths.Path {
	return paths.ProtocolsFIBIPv4Table
}

func (h *FIBIPv4TableHandler) Dependencies() []paths.Path {
	return nil
}

func (h *FIBIPv4TableHandler) Summary() string {
	return "Show VPP IPv4 FIB for one table"
}

func (h *FIBIPv4TableHandler) Description() string {
	return "Display the IPv4 forwarding table entries for the requested VPP table ID."
}

func extractTableID(path string, pattern paths.Path) (uint32, error) {
	wildcards, err := paths.Extract(path, pattern)
	if err != nil {
		return 0, fmt.Errorf("extract FIB table: %w", err)
	}
	if len(wildcards) < 1 {
		return 0, fmt.Errorf("invalid path: expected table wildcard, got 0")
	}
	id, err := strconv.ParseUint(wildcards[0], 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid FIB table ID %q: %w", wildcards[0], err)
	}
	return uint32(id), nil
}
