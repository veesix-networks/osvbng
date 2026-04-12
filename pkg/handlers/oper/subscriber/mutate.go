// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package subscriber

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper/paths"

	subcomp "github.com/veesix-networks/osvbng/internal/subscriber"
)

func init() {
	oper.RegisterFactory(NewMutateSessionHandler)
}

type MutateSessionHandler struct {
	deps *deps.OperDeps
}

type MutateSessionRequest struct {
	Targets    []subcomp.Target  `json:"targets"`
	Attributes map[string]string `json:"attributes"`
}

type MutateSessionResponse struct {
	Mutated int                    `json:"mutated"`
	Failed  int                    `json:"failed"`
	Results []subcomp.TargetResult `json:"results"`
}

func NewMutateSessionHandler(deps *deps.OperDeps) oper.OperHandler {
	return &MutateSessionHandler{deps: deps}
}

func (h *MutateSessionHandler) Execute(ctx context.Context, req *oper.Request) (interface{}, error) {
	if h.deps.Subscriber == nil {
		return nil, fmt.Errorf("subscriber component not available")
	}

	var body MutateSessionRequest
	if err := json.Unmarshal(req.Body, &body); err != nil {
		return nil, fmt.Errorf("invalid request body: %w", err)
	}

	if len(body.Targets) == 0 {
		return nil, fmt.Errorf("at least one target must be specified")
	}
	if len(body.Attributes) == 0 {
		return nil, fmt.Errorf("at least one attribute must be specified")
	}

	result, err := h.deps.Subscriber.MutateSubscribers(ctx, body.Targets, body.Attributes)
	if err != nil {
		return nil, fmt.Errorf("mutate subscribers: %w", err)
	}

	return &MutateSessionResponse{
		Mutated: result.Mutated,
		Failed:  result.Failed,
		Results: result.Results,
	}, nil
}

func (h *MutateSessionHandler) PathPattern() paths.Path {
	return paths.SubscriberSessionMutate
}

func (h *MutateSessionHandler) Dependencies() []paths.Path {
	return nil
}

func (h *MutateSessionHandler) Summary() string {
	return "Mutate subscriber session attributes"
}

func (h *MutateSessionHandler) Description() string {
	return "Change per-subscriber AAA attributes (QoS, ACL, timers) on live sessions without teardown."
}

func (h *MutateSessionHandler) InputType() interface{} {
	return &MutateSessionRequest{}
}

func (h *MutateSessionHandler) OutputType() interface{} {
	return &MutateSessionResponse{}
}
