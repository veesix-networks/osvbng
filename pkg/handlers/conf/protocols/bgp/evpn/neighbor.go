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
	conf.RegisterFactory(NewL2VPNEVPNNeighborHandler)
}

type L2VPNEVPNNeighborHandler struct {
	callbacks *conf.Callbacks
}

func NewL2VPNEVPNNeighborHandler(deps *deps.ConfDeps) conf.Handler {
	return &L2VPNEVPNNeighborHandler{
		callbacks: &conf.Callbacks{
			OnAfterApply: func(hctx *conf.HandlerContext, err error) {
				if err == nil {
					hctx.MarkFRRReloadNeeded()
				}
			},
		},
	}
}

func (h *L2VPNEVPNNeighborHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *L2VPNEVPNNeighborHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *L2VPNEVPNNeighborHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *L2VPNEVPNNeighborHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPL2VPNEVPNNeighbor
}

func (h *L2VPNEVPNNeighborHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ProtocolsBGPL2VPNEVPN}
}

func (h *L2VPNEVPNNeighborHandler) Callbacks() *conf.Callbacks {
	return h.callbacks
}

func (h *L2VPNEVPNNeighborHandler) Summary() string {
	return "BGP L2VPN EVPN neighbor"
}

func (h *L2VPNEVPNNeighborHandler) Description() string {
	return "Configure a BGP L2VPN EVPN neighbor."
}

func (h *L2VPNEVPNNeighborHandler) ValueType() interface{} {
	return false
}
