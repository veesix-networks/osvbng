// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package dataplane

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	show.RegisterFactory(NewLACPHandler)
}

type LACPHandler struct {
	southbound southbound.Southbound
}

func NewLACPHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &LACPHandler{southbound: deps.Southbound}
}

func (h *LACPHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	return h.southbound.DumpLACPInterfaces()
}

func (h *LACPHandler) PathPattern() paths.Path {
	return paths.SystemDataplaneLACP
}

func (h *LACPHandler) Dependencies() []paths.Path {
	return nil
}
