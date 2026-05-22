// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"encoding/json"
	"fmt"
	"net/netip"
)

// NeighborsAll wraps the output of `show bgp vrf all neighbors json`. FRR
// returns a polymorphic outer map keyed by VRF name; each VRF block mixes
// fixed metadata (vrfId, vrfName) with arbitrary IP-keyed neighbor entries.
// The wrapper splits these into a typed two-level map for the telemetry walker
// (outer vrf, inner neighbor_addr).
//
// Metric names live under `protocols.bgp.neighbor.all.*` rather than reusing
// the single-VRF `protocols.bgp.neighbor.*` namespace, because the existing
// path registers Neighbor with labels [neighbor_addr,state] and this path
// adds a vrf dimension; same metric name with different label sets would
// fail registration validation (registry.ErrSchemaMismatch).
type NeighborsAll struct {
	VRFs map[string]VRFNeighborSummary `json:"-" metric:"flatten"`
}

// VRFNeighborSummary is one entry in NeighborsAll.VRFs. The outer map key
// projects into the `vrf` label via map_key; Neighbors is a flatten map of
// per-peer summary metrics keyed by neighbor address.
type VRFNeighborSummary struct {
	VRF       string                     `json:"-" metric:"label=vrf,map_key"`
	Neighbors map[string]NeighborSummary `json:"-" metric:"flatten"`
}

// NeighborSummary is the per-VRF, per-peer view emitted on
// `protocols.bgp.neighbors.all`. Fields are populated by the wrapper from the
// full Neighbor parse; this struct intentionally exposes a smaller, dashboard-
// oriented subset of fields rather than the full Neighbor surface.
type NeighborSummary struct {
	NeighborAddr string `json:"-" metric:"label=neighbor_addr,map_key"`
	State        string `json:"-" metric:"label=state"`

	Up                     uint64 `json:"-" metric:"name=protocols.bgp.neighbor.all.up,type=gauge,help=1 if the BGP session is Established, 0 otherwise."`
	UptimeMS               uint64 `json:"-" metric:"name=protocols.bgp.neighbor.all.uptime_ms,type=gauge,help=BGP session uptime in milliseconds."`
	ConnectionsEstablished uint64 `json:"-" metric:"name=protocols.bgp.neighbor.all.connections_established_total,type=counter,help=Number of times the BGP session reached Established state."`
	ConnectionsDropped     uint64 `json:"-" metric:"name=protocols.bgp.neighbor.all.connections_dropped_total,type=counter,help=Number of times the BGP session dropped."`
	MessagesSent           uint64 `json:"-" metric:"name=protocols.bgp.neighbor.all.messages_sent_total,type=counter,help=Total BGP messages sent on this session."`
	MessagesRecv           uint64 `json:"-" metric:"name=protocols.bgp.neighbor.all.messages_recv_total,type=counter,help=Total BGP messages received on this session."`
}

// UnmarshalJSON splits FRR's polymorphic per-VRF block into VRF metadata
// (silently dropped — surfaced separately if needed) and per-neighbor entries
// keyed by IP. A key is treated as a neighbor IP iff `netip.ParseAddr`
// accepts it; otherwise it's treated as VRF metadata (vrfId, vrfName, etc.)
// and skipped for summary purposes.
func (g *VRFNeighborSummary) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse VRF neighbor group: %w", err)
	}
	g.Neighbors = make(map[string]NeighborSummary, len(raw))
	for k, v := range raw {
		if _, err := netip.ParseAddr(k); err != nil {
			continue
		}
		var n Neighbor
		if err := json.Unmarshal(v, &n); err != nil {
			return fmt.Errorf("parse neighbor %s: %w", k, err)
		}
		var up uint64
		if n.BgpState == "Established" {
			up = 1
		}
		g.Neighbors[k] = NeighborSummary{
			NeighborAddr:           k,
			State:                  n.BgpState,
			Up:                     up,
			UptimeMS:               n.BgpTimerUpMsec,
			ConnectionsEstablished: n.ConnectionsEstablished,
			ConnectionsDropped:     n.ConnectionsDropped,
			MessagesSent:           n.MessageStats.TotalSent,
			MessagesRecv:           n.MessageStats.TotalRecv,
		}
	}
	return nil
}
