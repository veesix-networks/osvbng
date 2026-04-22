// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/models"
)

func init() {
	show.RegisterFactory(func(d *deps.ShowDeps) show.ShowHandler {
		return &SessionsHandler{deps: d}
	})
}

type SessionsHandler struct {
	deps *deps.ShowDeps
}

func (h *SessionsHandler) Collect(_ context.Context, _ *show.Request) (interface{}, error) {
	if h.deps.CGNAT == nil {
		return []models.CGNATMapping{}, nil
	}

	return h.deps.CGNAT.GetPoolManager().GetAllMappings(), nil
}

func (h *SessionsHandler) PathPattern() paths.Path {
	return paths.CGNATSessions
}

func (h *SessionsHandler) Dependencies() []paths.Path {
	return nil
}

func (h *SessionsHandler) Summary() string {
	return "List active CGNAT mappings"
}

func (h *SessionsHandler) Description() string {
	return "Return every active CGNAT mapping including inside and outside address and port-block pairs."
}
