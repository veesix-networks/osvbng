// Copyright 2025 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper/paths"
)

func init() {
	oper.RegisterFactory(NewSwitchoverHandler)
}

type SwitchoverHandler struct {
	deps *deps.OperDeps
}

type SwitchoverRequest struct {
	SRGNames []string `json:"srg_names"`
}

type SwitchoverResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func NewSwitchoverHandler(deps *deps.OperDeps) oper.OperHandler {
	return &SwitchoverHandler{deps: deps}
}

func (h *SwitchoverHandler) Execute(ctx context.Context, req *oper.Request) (interface{}, error) {
	if h.deps.HAManager == nil {
		return nil, fmt.Errorf("HA not enabled")
	}

	var body SwitchoverRequest
	if err := json.Unmarshal(req.Body, &body); err != nil {
		return nil, fmt.Errorf("invalid request body: %w", err)
	}

	if len(body.SRGNames) == 0 {
		for name := range h.deps.HAManager.GetSRGs() {
			body.SRGNames = append(body.SRGNames, name)
		}
	}

	if err := h.deps.HAManager.RequestSwitchover(ctx, body.SRGNames); err != nil {
		return &SwitchoverResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	return &SwitchoverResponse{
		Success: true,
		Message: fmt.Sprintf("switchover requested for SRGs: %v", body.SRGNames),
	}, nil
}

func (h *SwitchoverHandler) PathPattern() paths.Path {
	return paths.HASwitchover
}

func (h *SwitchoverHandler) Dependencies() []paths.Path {
	return nil
}
