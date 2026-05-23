// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package fib

import (
	"context"
	"fmt"
	"net/netip"
	"strconv"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	show.RegisterFactory(NewFIBIPv4PrefixHandler)
}

type FIBIPv4PrefixHandler struct {
	southbound southbound.Southbound
}

func NewFIBIPv4PrefixHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &FIBIPv4PrefixHandler{southbound: deps.Southbound}
}

func (h *FIBIPv4PrefixHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.southbound == nil {
		return nil, fmt.Errorf("southbound not available")
	}
	tableID, prefix, err := extractTableAndPrefix(req.Path, paths.ProtocolsFIBIPv4Prefix)
	if err != nil {
		return nil, err
	}
	if !prefix.Addr().Is4() {
		return nil, fmt.Errorf("expected IPv4 prefix, got %q", prefix)
	}
	return h.southbound.LookupIPv4FIB(tableID, prefix)
}

func (h *FIBIPv4PrefixHandler) PathPattern() paths.Path {
	return paths.ProtocolsFIBIPv4Prefix
}

func (h *FIBIPv4PrefixHandler) Dependencies() []paths.Path {
	return nil
}

func (h *FIBIPv4PrefixHandler) Summary() string {
	return "Look up an IPv4 prefix in a VPP FIB table"
}

func (h *FIBIPv4PrefixHandler) Description() string {
	return "Return the matching FIB entry for an IPv4 prefix in the requested VPP table; the lookup dumps the table and returns the entry whose prefix matches."
}

func extractTableAndPrefix(path string, pattern paths.Path) (uint32, netip.Prefix, error) {
	wildcards, err := paths.Extract(path, pattern)
	if err != nil {
		return 0, netip.Prefix{}, fmt.Errorf("extract FIB table+prefix: %w", err)
	}
	if len(wildcards) != 2 {
		return 0, netip.Prefix{}, fmt.Errorf("invalid path: expected 2 wildcards, got %d", len(wildcards))
	}
	id, err := strconv.ParseUint(wildcards[0], 10, 32)
	if err != nil {
		return 0, netip.Prefix{}, fmt.Errorf("invalid FIB table ID %q: %w", wildcards[0], err)
	}
	prefix, err := netip.ParsePrefix(wildcards[1])
	if err != nil {
		return 0, netip.Prefix{}, fmt.Errorf("invalid prefix %q: %w", wildcards[1], err)
	}
	return uint32(id), prefix, nil
}
