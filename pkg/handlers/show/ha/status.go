// Copyright 2025 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/state"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	show.RegisterFactory(func(d *deps.ShowDeps) show.ShowHandler {
		return &StatusHandler{deps: d}
	})

	state.RegisterMetric(statepaths.HAStatus, paths.HAStatus)
}

type StatusHandler struct {
	deps *deps.ShowDeps
}

type HAStatusInfo struct {
	Enabled  bool           `json:"enabled"`
	NodeID   string         `json:"node_id,omitempty"`
	Peer     *PeerInfo      `json:"peer,omitempty"`
	SRGs     []SRGSummary   `json:"srgs,omitempty"`
}

type PeerInfo struct {
	Address   string `json:"address"`
	Connected bool   `json:"connected"`
	NodeID    string `json:"node_id,omitempty"`
}

type SRGSummary struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

func (h *StatusHandler) Collect(_ context.Context, _ *show.Request) (interface{}, error) {
	if h.deps.HAManager == nil {
		return &HAStatusInfo{Enabled: false}, nil
	}

	mgr := h.deps.HAManager
	ps := mgr.GetPeerState()

	info := &HAStatusInfo{
		Enabled: true,
		NodeID:  mgr.GetNodeID(),
		Peer: &PeerInfo{
			Connected: ps.Connected,
			NodeID:    ps.NodeID,
		},
	}

	for name, sm := range mgr.GetSRGs() {
		info.SRGs = append(info.SRGs, SRGSummary{
			Name:  name,
			State: string(sm.State()),
		})
	}

	return info, nil
}

func (h *StatusHandler) PathPattern() paths.Path {
	return paths.HAStatus
}

func (h *StatusHandler) Dependencies() []paths.Path {
	return nil
}

func (h *StatusHandler) Summary() string {
	return "Show HA subsystem overview"
}

func (h *StatusHandler) Description() string {
	return "Display a high-level overview of the HA subsystem including node ID, peer connectivity, and SRG states."
}
