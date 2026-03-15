// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(func(d *deps.ShowDeps) show.ShowHandler {
		return &LookupHandler{deps: d}
	})
}

type LookupHandler struct {
	deps *deps.ShowDeps
}

func (h *LookupHandler) Collect(_ context.Context, req *show.Request) (interface{}, error) {
	if h.deps.CGNAT == nil {
		return nil, fmt.Errorf("CGNAT not configured")
	}

	ipStr := req.Options["ip"]
	portStr := req.Options["port"]

	if ipStr == "" || portStr == "" {
		return nil, fmt.Errorf("ip and port parameters required")
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP: %s", ipStr)
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid port: %s", portStr)
	}

	mapping := h.deps.CGNAT.GetReverseIndex().Lookup(ip, uint16(port))
	if mapping == nil {
		return nil, fmt.Errorf("no mapping found for %s:%d", ip, port)
	}

	return mapping, nil
}

func (h *LookupHandler) PathPattern() paths.Path {
	return paths.CGNATLookup
}

func (h *LookupHandler) Dependencies() []paths.Path {
	return nil
}
