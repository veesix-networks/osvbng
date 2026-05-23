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
	show.RegisterFactory(NewFIBIPv6Handler)
}

type FIBIPv6Handler struct {
	southbound southbound.Southbound
}

func NewFIBIPv6Handler(deps *deps.ShowDeps) show.ShowHandler {
	return &FIBIPv6Handler{southbound: deps.Southbound}
}

func (h *FIBIPv6Handler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.southbound == nil {
		return nil, fmt.Errorf("southbound not available")
	}
	return h.southbound.GetIPv6FIB(0)
}

func (h *FIBIPv6Handler) PathPattern() paths.Path {
	return paths.ProtocolsFIBIPv6
}

func (h *FIBIPv6Handler) Dependencies() []paths.Path {
	return nil
}

func (h *FIBIPv6Handler) Summary() string {
	return "Show VPP IPv6 FIB (default table)"
}

func (h *FIBIPv6Handler) Description() string {
	return "Display the IPv6 forwarding table entries in the default VRF as held by VPP."
}
