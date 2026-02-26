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
		return &PeerHandler{deps: d}
	})
}

type PeerHandler struct {
	deps *deps.ShowDeps
}

type PeerDetail struct {
	Connected     bool    `json:"connected"`
	NodeID        string  `json:"node_id,omitempty"`
	LastHeartbeat string  `json:"last_heartbeat,omitempty"`
	RTTMs         float64 `json:"rtt_ms"`
	ClockSkewMs   float64 `json:"clock_skew_ms"`
}

func (h *PeerHandler) Collect(_ context.Context, _ *show.Request) (interface{}, error) {
	if h.deps.HAManager == nil {
		return &PeerDetail{}, nil
	}

	ps := h.deps.HAManager.GetPeerState()

	detail := &PeerDetail{
		Connected:   ps.Connected,
		NodeID:      ps.NodeID,
		RTTMs:       float64(ps.RTT.Milliseconds()),
		ClockSkewMs: float64(ps.ClockSkew.Milliseconds()),
	}

	if !ps.LastHeartbeat.IsZero() {
		detail.LastHeartbeat = ps.LastHeartbeat.Format("2006-01-02T15:04:05Z07:00")
	}

	return detail, nil
}

func (h *PeerHandler) PathPattern() paths.Path {
	return paths.HAPeer
}

func (h *PeerHandler) Dependencies() []paths.Path {
	return nil
}
