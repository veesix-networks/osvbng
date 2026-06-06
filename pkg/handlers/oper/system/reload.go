// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package system

import (
	"context"
	"fmt"
	"time"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper/paths"
)

func init() {
	oper.RegisterFactory(NewSystemReloadHandler)
}

type SystemReloadHandler struct {
	deps *deps.OperDeps
}

type SystemReloadRequest struct{}

type SystemReloadResponse struct {
	DurationMS        int64    `json:"duration_ms"`
	ComponentsTouched []string `json:"components_touched"`
	Errors            []string `json:"errors,omitempty"`
}

func NewSystemReloadHandler(deps *deps.OperDeps) oper.OperHandler {
	return &SystemReloadHandler{deps: deps}
}

func (h *SystemReloadHandler) Execute(ctx context.Context, req *oper.Request) (interface{}, error) {
	if h.deps.ConfigReloader == nil {
		return nil, fmt.Errorf("config reloader not wired into OperDeps")
	}
	start := time.Now()
	resp := &SystemReloadResponse{}

	if err := h.deps.ConfigReloader.ReloadFRR(); err != nil {
		resp.Errors = append(resp.Errors, fmt.Sprintf("frr reload: %v", err))
	} else {
		resp.ComponentsTouched = append(resp.ComponentsTouched, "frr")
	}
	resp.ComponentsTouched = append(resp.ComponentsTouched, "dataplane:not-handled")

	resp.DurationMS = time.Since(start).Milliseconds()
	if len(resp.Errors) > 0 {
		return resp, fmt.Errorf("reload had errors: %v", resp.Errors)
	}
	return resp, nil
}

func (h *SystemReloadHandler) PathPattern() paths.Path           { return paths.SystemReload }
func (h *SystemReloadHandler) Dependencies() []paths.Path        { return nil }
func (h *SystemReloadHandler) InputType() interface{}            { return &SystemReloadRequest{} }
func (h *SystemReloadHandler) OutputType() interface{}           { return &SystemReloadResponse{} }
func (h *SystemReloadHandler) Summary() string                   { return "Re-render config from current templates and reload" }
func (h *SystemReloadHandler) Description() string {
	return "Re-reads templates from /usr/share/osvbng/templates/, re-renders FRR configuration, and applies it via frr-reload."
}
