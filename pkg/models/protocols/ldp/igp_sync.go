// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ldp

// IGPSync mirrors one entry in `show mpls ldp igp-sync json`.
// The outer JSON shape is `{"<iface>": IGPSync}`; Interface is populated
// from the map key via the telemetry walker's map_key handling.
type IGPSync struct {
	Interface         string `json:"-" metric:"label,map_key"`
	State             string `json:"state" metric:"label"`
	WaitTime          uint32 `json:"waitTime" metric:"name=protocols.ldp.igp_sync.wait_time_seconds,type=gauge,help=Configured LDP IGP-sync wait time in seconds."`
	WaitTimeRemaining uint32 `json:"waitTimeRemaining" metric:"name=protocols.ldp.igp_sync.wait_time_remaining_seconds,type=gauge,help=Remaining LDP IGP-sync wait time in seconds."`
	TimerRunning      bool   `json:"timerRunning,omitempty"`
	PeerLdpId         string `json:"peerLdpId,omitempty"`
}
