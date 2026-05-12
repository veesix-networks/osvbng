// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package dhcp

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/dhcp4"
	"github.com/veesix-networks/osvbng/pkg/dhcp6"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

func init() {
	show.RegisterFactory(NewProxyHandler)
	telemetry.RegisterMetric[ProxyInfo](paths.DHCPProxy)
}

type ProxyHandler struct {
	dhcp4Providers map[string]dhcp4.DHCPProvider
	dhcp6Providers map[string]dhcp6.DHCPProvider
}

func NewProxyHandler(d *deps.ShowDeps) show.ShowHandler {
	return &ProxyHandler{
		dhcp4Providers: d.DHCPv4Providers,
		dhcp6Providers: d.DHCPv6Providers,
	}
}

type ProxyInfo struct {
	V4Bindings int `json:"v4Bindings" metric:"name=dhcp.proxy.bindings_v4,type=gauge,help=Active DHCPv4 proxy bindings."`
	V6Bindings int `json:"v6Bindings" metric:"name=dhcp.proxy.bindings_v6,type=gauge,help=Active DHCPv6 proxy bindings."`
}

func (h *ProxyHandler) Collect(_ context.Context, _ *show.Request) (interface{}, error) {
	info := &ProxyInfo{}

	for _, p := range h.dhcp4Providers {
		if bc, ok := p.(dhcp4.BindingCounter); ok {
			info.V4Bindings += bc.BindingCount()
		}
	}

	for _, p := range h.dhcp6Providers {
		if bc, ok := p.(dhcp6.BindingCounter); ok {
			info.V6Bindings += bc.BindingCount()
		}
	}

	return info, nil
}

func (h *ProxyHandler) PathPattern() paths.Path {
	return paths.DHCPProxy
}

func (h *ProxyHandler) Dependencies() []paths.Path {
	return nil
}

func (h *ProxyHandler) Summary() string {
	return "Show DHCP proxy binding counts"
}

func (h *ProxyHandler) Description() string {
	return "Display the number of active DHCPv4 and DHCPv6 proxy bindings."
}
