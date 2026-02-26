// Copyright 2025 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(func(d *deps.ShowDeps) show.ShowHandler {
		return &SRGHandler{deps: d}
	})
}

type SRGHandler struct {
	deps *deps.ShowDeps
}

type SRGDetail struct {
	Name           string `json:"name"`
	State          string `json:"state"`
	Priority       uint32 `json:"priority"`
	Preempt        bool   `json:"preempt"`
	VirtualMAC     string `json:"virtual_mac,omitempty"`
	LastTransition string `json:"last_transition"`
}

func (h *SRGHandler) Collect(_ context.Context, _ *show.Request) (interface{}, error) {
	if h.deps.HAManager == nil {
		return []SRGDetail{}, nil
	}

	var result []SRGDetail
	for name, sm := range h.deps.HAManager.GetSRGs() {
		detail := SRGDetail{
			Name:           name,
			State:          string(sm.State()),
			Priority:       sm.Priority(),
			Preempt:        sm.Preempt(),
			LastTransition: sm.LastTransition().Format("2006-01-02T15:04:05Z07:00"),
		}
		if vmac := sm.VirtualMAC(); vmac != nil {
			detail.VirtualMAC = vmac.String()
		}
		result = append(result, detail)
	}

	return result, nil
}

func (h *SRGHandler) PathPattern() paths.Path {
	return paths.HASRGs
}

func (h *SRGHandler) Dependencies() []paths.Path {
	return nil
}
