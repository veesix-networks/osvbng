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
	show.RegisterFactory(NewFIBIPv6PrefixHandler)
}

type FIBIPv6PrefixHandler struct {
	southbound southbound.Southbound
}

func NewFIBIPv6PrefixHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &FIBIPv6PrefixHandler{southbound: deps.Southbound}
}

func (h *FIBIPv6PrefixHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.southbound == nil {
		return nil, fmt.Errorf("southbound not available")
	}
	tableID, prefix, err := extractTableAndPrefix(req.Path, paths.ProtocolsFIBIPv6Prefix)
	if err != nil {
		return nil, err
	}
	if prefix.Addr().Is4() {
		return nil, fmt.Errorf("expected IPv6 prefix, got %q", prefix)
	}
	return h.southbound.LookupIPv6FIB(tableID, prefix)
}

func (h *FIBIPv6PrefixHandler) PathPattern() paths.Path {
	return paths.ProtocolsFIBIPv6Prefix
}

func (h *FIBIPv6PrefixHandler) Dependencies() []paths.Path {
	return nil
}

func (h *FIBIPv6PrefixHandler) Summary() string {
	return "Look up an IPv6 prefix in a VPP FIB table"
}

func (h *FIBIPv6PrefixHandler) Description() string {
	return "Return the matching FIB entry for an IPv6 prefix in the requested VPP table; the lookup dumps the table and returns the entry whose prefix matches."
}
