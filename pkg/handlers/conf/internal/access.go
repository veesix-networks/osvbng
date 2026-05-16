// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package internal

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/operations"
	pkgpaths "github.com/veesix-networks/osvbng/pkg/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	conf.RegisterFactory(NewIPoEInputHandler)
	conf.RegisterFactory(NewPromiscuousHandler)
}

type IPoEInputHandler struct {
	southbound southbound.Southbound
}

func NewIPoEInputHandler(d *deps.ConfDeps) conf.Handler {
	return &IPoEInputHandler{southbound: d.Southbound}
}

func (h *IPoEInputHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	if _, ok := hctx.NewValue.(*operations.AccessConfig); !ok {
		return fmt.Errorf("expected *operations.AccessConfig, got %T", hctx.NewValue)
	}
	return nil
}

func (h *IPoEInputHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg := hctx.NewValue.(*operations.AccessConfig)
	if !cfg.Enabled {
		return nil
	}
	values, err := paths.InternalAccessIPoEInput.ExtractWildcards(hctx.Path, 1)
	if err != nil {
		return fmt.Errorf("extract interface from path: %w", err)
	}
	ifName := pkgpaths.DecodeInterfaceName(values[0])
	return h.southbound.IPoEEnableInput(ifName)
}

func (h *IPoEInputHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPoEInputHandler) PathPattern() paths.Path    { return paths.InternalAccessIPoEInput }
func (h *IPoEInputHandler) Dependencies() []paths.Path { return []paths.Path{paths.InterfaceSubinterface} }
func (h *IPoEInputHandler) Callbacks() *conf.Callbacks { return nil }
func (h *IPoEInputHandler) Summary() string            { return "IPoE input enablement" }
func (h *IPoEInputHandler) Description() string {
	return "Enable IPoE classification on a sub-interface."
}
func (h *IPoEInputHandler) ValueType() interface{} { return &operations.AccessConfig{} }

type PromiscuousHandler struct {
	southbound southbound.Southbound
}

func NewPromiscuousHandler(d *deps.ConfDeps) conf.Handler {
	return &PromiscuousHandler{southbound: d.Southbound}
}

func (h *PromiscuousHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	if _, ok := hctx.NewValue.(*operations.AccessConfig); !ok {
		return fmt.Errorf("expected *operations.AccessConfig, got %T", hctx.NewValue)
	}
	return nil
}

func (h *PromiscuousHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg := hctx.NewValue.(*operations.AccessConfig)
	values, err := paths.InternalAccessPromisc.ExtractWildcards(hctx.Path, 1)
	if err != nil {
		return fmt.Errorf("extract interface from path: %w", err)
	}
	ifName := pkgpaths.DecodeInterfaceName(values[0])
	return h.southbound.SetInterfacePromiscuous(ifName, cfg.Enabled)
}

func (h *PromiscuousHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *PromiscuousHandler) PathPattern() paths.Path    { return paths.InternalAccessPromisc }
func (h *PromiscuousHandler) Dependencies() []paths.Path { return []paths.Path{paths.Interface} }
func (h *PromiscuousHandler) Callbacks() *conf.Callbacks { return nil }
func (h *PromiscuousHandler) Summary() string            { return "Parent-interface promiscuous mode" }
func (h *PromiscuousHandler) Description() string {
	return "Toggle promiscuous mode on a parent interface for PPPoE/LAC subscriber access."
}
func (h *PromiscuousHandler) ValueType() interface{} { return &operations.AccessConfig{} }
