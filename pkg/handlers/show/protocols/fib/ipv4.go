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
	show.RegisterFactory(NewFIBIPv4Handler)
}

type FIBIPv4Handler struct {
	southbound southbound.Southbound
}

func NewFIBIPv4Handler(deps *deps.ShowDeps) show.ShowHandler {
	return &FIBIPv4Handler{southbound: deps.Southbound}
}

func (h *FIBIPv4Handler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.southbound == nil {
		return nil, fmt.Errorf("southbound not available")
	}
	return h.southbound.GetIPv4FIB(0)
}

func (h *FIBIPv4Handler) PathPattern() paths.Path {
	return paths.ProtocolsFIBIPv4
}

func (h *FIBIPv4Handler) Dependencies() []paths.Path {
	return nil
}

func (h *FIBIPv4Handler) Summary() string {
	return "Show VPP IPv4 FIB (default table)"
}

func (h *FIBIPv4Handler) Description() string {
	return "Display the IPv4 forwarding table entries in the default VRF as held by VPP."
}
