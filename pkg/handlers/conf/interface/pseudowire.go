// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package iface

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	conf.RegisterFactory(NewInterfacePseudowireHandler)
}

type PseudowireHandler struct {
	southbound southbound.Southbound
}

func NewInterfacePseudowireHandler(d *deps.ConfDeps) conf.Handler {
	return &PseudowireHandler{
		southbound: d.Southbound,
	}
}

func (h *PseudowireHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg, ok := hctx.NewValue.(*interfaces.PseudowireConfig)
	if !ok {
		return fmt.Errorf("expected *interfaces.PseudowireConfig, got %T", hctx.NewValue)
	}
	return cfg.Validate()
}

func (h *PseudowireHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg := hctx.NewValue.(*interfaces.PseudowireConfig)

	values, err := paths.InterfacePseudowire.ExtractWildcards(hctx.Path, 1)
	if err != nil {
		return fmt.Errorf("extract interface name: %w", err)
	}

	// An evpn-signaled transport may not exist in VPP yet; the EVPN
	// manager binds the headend when it programs the tunnel.
	if isEVPNTunnel(hctx, cfg.Transport) {
		if _, err := h.southbound.GetInterfaceIndex(cfg.Transport); err != nil {
			return nil
		}
	}

	return h.southbound.BindPseudowire(values[0], cfg.Transport)
}

func (h *PseudowireHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *PseudowireHandler) PathPattern() paths.Path {
	return paths.InterfacePseudowire
}

func (h *PseudowireHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.Interface}
}

func (h *PseudowireHandler) Callbacks() *conf.Callbacks {
	return nil
}

func (h *PseudowireHandler) Summary() string {
	return "Pseudowire headend binding"
}

func (h *PseudowireHandler) Description() string {
	return "Bind a pseudowire headend interface to its transport tunnel."
}

func (h *PseudowireHandler) ValueType() interface{} {
	return &interfaces.PseudowireConfig{}
}
