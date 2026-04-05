// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package iface

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	conf.RegisterFactory(NewSubinterfaceLCPHandler)
}

type LCPHandler struct {
	southbound   southbound.Southbound
	pathPattern  paths.Path
	dependencies []paths.Path
}

func NewSubinterfaceLCPHandler(d *deps.ConfDeps) conf.Handler {
	return &LCPHandler{
		southbound:   d.Southbound,
		pathPattern:  paths.InterfaceSubinterfaceLCP,
		dependencies: []paths.Path{paths.InterfaceSubinterface},
	}
}

func (h *LCPHandler) extractInterfaceName(path string) (string, error) {
	values, err := h.pathPattern.ExtractWildcards(path, 2)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.%s", values[0], values[1]), nil
}

func (h *LCPHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	_, ok := hctx.NewValue.(bool)
	if !ok {
		return fmt.Errorf("lcp must be a boolean")
	}
	return nil
}

func (h *LCPHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	enabled := hctx.NewValue.(bool)
	if !enabled {
		return nil
	}

	ifName, err := h.extractInterfaceName(hctx.Path)
	if err != nil {
		return fmt.Errorf("extract interface name: %w", err)
	}

	if h.southbound.HasLCPPair(ifName) {
		return nil
	}

	return h.southbound.CreateLCPPair(ifName)
}

func (h *LCPHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *LCPHandler) PathPattern() paths.Path {
	return h.pathPattern
}

func (h *LCPHandler) Dependencies() []paths.Path {
	return h.dependencies
}

func (h *LCPHandler) Callbacks() *conf.Callbacks {
	return nil
}
