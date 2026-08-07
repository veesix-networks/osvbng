// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2gw

import (
	"sort"
	"time"

	"github.com/veesix-networks/osvbng/pkg/southbound"
)

// CircuitSummary is the show/API projection of one circuit. The metric
// tags export the per-circuit counters through the telemetry SDK as
// dataplane.vpp.l2gw.* with the stable access/handoff identity as
// labels.
type CircuitSummary struct {
	SessionID       string    `json:"session_id,omitempty"`
	MAC             string    `json:"mac,omitempty"`
	Username        string    `json:"username,omitempty"`
	AccessInterface string    `json:"access_interface"        metric:"label"`
	AccessSVLAN     uint16    `json:"access_svlan"            metric:"label"`
	AccessCVLAN     uint16    `json:"access_cvlan,omitempty"  metric:"label"`
	AccessCVLANAny  bool      `json:"access_cvlan_any,omitempty"`
	HandoffGroup    string    `json:"handoff_group"           metric:"label"`
	HandoffSVLAN    uint16    `json:"handoff_svlan,omitempty"`
	HandoffCVLAN    uint16    `json:"handoff_cvlan,omitempty"`
	Transparent     bool      `json:"transparent,omitempty"`
	Static          bool      `json:"static,omitempty"`
	Protocol        string    `json:"protocol,omitempty"`
	State           string    `json:"state"`
	CircuitID       uint32    `json:"circuit_id"`
	CreatedAt       time.Time `json:"created_at,omitempty"`

	UpstreamPackets   uint64 `json:"upstream_packets"   metric:"name=dataplane.vpp.l2gw.upstream_packets,type=counter,help=L2GW circuit access-to-handoff packets."`
	UpstreamBytes     uint64 `json:"upstream_bytes"     metric:"name=dataplane.vpp.l2gw.upstream_bytes,type=counter,help=L2GW circuit access-to-handoff bytes."`
	DownstreamPackets uint64 `json:"downstream_packets" metric:"name=dataplane.vpp.l2gw.downstream_packets,type=counter,help=L2GW circuit handoff-to-access packets."`
	DownstreamBytes   uint64 `json:"downstream_bytes"   metric:"name=dataplane.vpp.l2gw.downstream_bytes,type=counter,help=L2GW circuit handoff-to-access bytes."`
}

// SnapshotCircuits returns all circuits sorted by access tuple, with
// per-direction counters from the /osvbng/l2gw stats segment.
func (c *Component) SnapshotCircuits() []CircuitSummary {
	var stats map[uint32]southbound.L2GWEntryStats
	if c.vpp != nil {
		if s, err := c.vpp.GetL2GWStats(); err == nil {
			stats = s
		}
	}

	var out []CircuitSummary
	c.circuits.Range(func(_, v any) bool {
		ct := v.(*Circuit)
		ct.mu.Lock()
		s := CircuitSummary{
			SessionID:       ct.SessionID,
			MAC:             ct.MAC,
			Username:        ct.Username,
			AccessInterface: ct.AccessInterface,
			AccessSVLAN:     ct.AccessSVLAN,
			HandoffGroup:    ct.HandoffGroup,
			HandoffSVLAN:    ct.HandoffSVLAN,
			HandoffCVLAN:    ct.HandoffCVLAN,
			Transparent:     ct.Transparent,
			Static:          ct.Static,
			Protocol:        ct.Protocol,
			State:           ct.State,
			CircuitID:       ct.CircuitID,
			CreatedAt:       ct.CreatedAt,
		}
		if ct.AccessCVLAN == 0xFFFF {
			s.AccessCVLANAny = true
		} else {
			s.AccessCVLAN = ct.AccessCVLAN
		}
		if up, ok := stats[ct.AccessEntryIndex]; ok {
			s.UpstreamPackets = up.Packets
			s.UpstreamBytes = up.Bytes
		}
		if down, ok := stats[ct.HandoffEntryIndex]; ok {
			s.DownstreamPackets = down.Packets
			s.DownstreamBytes = down.Bytes
		}
		ct.mu.Unlock()
		out = append(out, s)
		return true
	})
	sort.Slice(out, func(i, j int) bool {
		if out[i].AccessInterface != out[j].AccessInterface {
			return out[i].AccessInterface < out[j].AccessInterface
		}
		if out[i].AccessSVLAN != out[j].AccessSVLAN {
			return out[i].AccessSVLAN < out[j].AccessSVLAN
		}
		return out[i].AccessCVLAN < out[j].AccessCVLAN
	})
	return out
}
