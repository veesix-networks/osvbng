// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"context"
	"strings"
	"sync"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

func init() {
	show.RegisterFactory(func(d *deps.ShowDeps) show.ShowHandler {
		return &SRGStateHandler{deps: d}
	})
	telemetry.RegisterMetricMulti[SRGStateInfo](paths.HASRGState)
}

type SRGStateInfo struct {
	SRG    string `json:"srg"    metric:"label"`
	State  string `json:"state"  metric:"label"`
	Active uint64 `json:"active" metric:"name=ha.srg.state,type=gauge,help=1 if the SRG is currently in this state on this peer."`
}

type SRGStateHandler struct {
	deps *deps.ShowDeps

	knownMu sync.Mutex
	known   map[string]struct{}
}

// Stale-state retention: when an SRG transitions A→B, the (srg,A) tuple
// must continue to emit Active=0 so the dashboard sees the transition
// instead of a stuck "1" left behind by the last poll. Without this the
// SDK keeps the last-seen value forever (no scrape staleness here).
func (h *SRGStateHandler) Collect(_ context.Context, _ *show.Request) (interface{}, error) {
	if h.deps.HAManager == nil {
		return []SRGStateInfo{}, nil
	}

	h.knownMu.Lock()
	defer h.knownMu.Unlock()
	if h.known == nil {
		h.known = make(map[string]struct{})
	}

	current := make(map[string]string)
	for name, sm := range h.deps.HAManager.GetSRGs() {
		current[name] = string(sm.State())
	}

	out := make([]SRGStateInfo, 0, len(current)+len(h.known))
	for name, state := range current {
		out = append(out, SRGStateInfo{SRG: name, State: state, Active: 1})
		h.known[name+"|"+state] = struct{}{}
	}
	for key := range h.known {
		sep := strings.Index(key, "|")
		if sep < 0 {
			continue
		}
		name, state := key[:sep], key[sep+1:]
		if current[name] == state {
			continue
		}
		out = append(out, SRGStateInfo{SRG: name, State: state, Active: 0})
	}
	return out, nil
}

func (h *SRGStateHandler) PathPattern() paths.Path {
	return paths.HASRGState
}

func (h *SRGStateHandler) Dependencies() []paths.Path {
	return nil
}

func (h *SRGStateHandler) Summary() string {
	return "Show HA SRG state per (srg, state) tuple"
}

func (h *SRGStateHandler) Description() string {
	return "Per-SRG, per-state gauge with stale-state retention for telemetry consumers."
}
