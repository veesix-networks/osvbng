// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package evpn

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewL2VPNEVPNHandler)
}

type L2VPNEVPNHandler struct {
	callbacks *conf.Callbacks
}

func NewL2VPNEVPNHandler(deps *deps.ConfDeps) conf.Handler {
	return &L2VPNEVPNHandler{
		callbacks: &conf.Callbacks{
			OnAfterApply: func(hctx *conf.HandlerContext, err error) {
				if err == nil {
					hctx.MarkFRRReloadNeeded()
				}
			},
		},
	}
}

func (h *L2VPNEVPNHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *L2VPNEVPNHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *L2VPNEVPNHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *L2VPNEVPNHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPL2VPNEVPN
}

func (h *L2VPNEVPNHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ProtocolsBGPInstance}
}

func (h *L2VPNEVPNHandler) Callbacks() *conf.Callbacks {
	return h.callbacks
}

func (h *L2VPNEVPNHandler) Summary() string {
	return "BGP L2VPN EVPN address family"
}

func (h *L2VPNEVPNHandler) Description() string {
	return "Enable the BGP L2VPN EVPN address family."
}

func (h *L2VPNEVPNHandler) ValueType() interface{} {
	return false
}
