// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package qos

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/qos"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper/paths"
)

func init() {
	oper.RegisterFactory(NewSchedulerSetHandler)
}

type SchedulerSetHandler struct {
	deps *deps.OperDeps
}

type SchedulerSetRequest struct {
	SwIfIndex uint32 `json:"sw_if_index"`
	RateKbps  uint32 `json:"rate_kbps"`
	TinMode   string `json:"tin_mode,omitempty"`
	Disable   bool   `json:"disable,omitempty"`
}

type SchedulerSetResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

func NewSchedulerSetHandler(deps *deps.OperDeps) oper.OperHandler {
	return &SchedulerSetHandler{deps: deps}
}

func (h *SchedulerSetHandler) Execute(_ context.Context, req *oper.Request) (interface{}, error) {
	if h.deps.Southbound == nil {
		return nil, fmt.Errorf("southbound not available")
	}

	var body SchedulerSetRequest
	if err := json.Unmarshal(req.Body, &body); err != nil {
		return nil, fmt.Errorf("invalid request body: %w", err)
	}

	if body.SwIfIndex == 0 {
		return nil, fmt.Errorf("sw_if_index is required")
	}

	if body.Disable {
		if err := h.deps.Southbound.RemoveScheduler(body.SwIfIndex); err != nil {
			return nil, fmt.Errorf("remove scheduler: %w", err)
		}
		return &SchedulerSetResponse{
			Success: true,
			Message: fmt.Sprintf("scheduler disabled on sw_if_index %d", body.SwIfIndex),
		}, nil
	}

	if body.RateKbps == 0 {
		return nil, fmt.Errorf("rate_kbps is required when not disabling")
	}

	if err := h.deps.Southbound.RemoveScheduler(body.SwIfIndex); err != nil {
		return nil, fmt.Errorf("remove existing scheduler: %w", err)
	}

	cfg := &qos.SchedulerConfig{TinMode: body.TinMode}
	if err := h.deps.Southbound.ApplyScheduler(body.SwIfIndex, body.RateKbps, cfg); err != nil {
		return nil, fmt.Errorf("apply scheduler: %w", err)
	}

	return &SchedulerSetResponse{
		Success: true,
		Message: fmt.Sprintf("scheduler set on sw_if_index %d at %d kbps", body.SwIfIndex, body.RateKbps),
	}, nil
}

func (h *SchedulerSetHandler) PathPattern() paths.Path {
	return paths.QoSSchedulerSet
}

func (h *SchedulerSetHandler) Dependencies() []paths.Path {
	return nil
}

func (h *SchedulerSetHandler) Summary() string {
	return "Set QoS scheduler on an interface"
}

func (h *SchedulerSetHandler) Description() string {
	return "Apply or disable a QoS scheduling policy on a dataplane interface."
}

func (h *SchedulerSetHandler) InputType() interface{} {
	return &SchedulerSetRequest{}
}

func (h *SchedulerSetHandler) OutputType() interface{} {
	return &SchedulerSetResponse{}
}
