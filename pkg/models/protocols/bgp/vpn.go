// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import "encoding/json"

// VPNRoutes matches `show bgp ipv4 vpn json` and `show bgp ipv6 vpn json`.
// Top-level scalar fields drive aggregate metrics; the deeply nested
// `routes` payload (routeDistinguishers -> RD -> prefix -> [paths]) is
// preserved as RawMessage for CLI/API passthrough but not modelled per
// route (per-RD per-prefix per-path cardinality is unbounded; spec #59
// Phase 1 Option 2 picked totalRoutes/totalPaths aggregates only).
type VPNRoutes struct {
	// AddressFamily is populated by the routing layer ("ipv4" or "ipv6")
	// so IPv4 and IPv6 VPN routes emit as distinct metric series.
	AddressFamily string `json:"-" metric:"label"`

	VrfId         uint32          `json:"vrfId,omitempty"`
	VrfName       string          `json:"vrfName,omitempty" metric:"label"`
	TableVersion  uint64          `json:"tableVersion" metric:"name=protocols.bgp.vpn.table_version,type=gauge,help=BGP VPN table version."`
	RouterId      string          `json:"routerId,omitempty"`        // show-output only
	DefaultLocPrf uint32          `json:"defaultLocPrf,omitempty"`   // show-output only
	LocalAS       uint32          `json:"localAS,omitempty"`         // show-output only
	Routes        json.RawMessage `json:"routes,omitempty"`          // show-output passthrough (Option 2)
	TotalRoutes   uint64          `json:"totalRoutes" metric:"name=protocols.bgp.vpn.routes,type=gauge,help=Total BGP VPN routes in this AF."`
	TotalPaths    uint64          `json:"totalPaths" metric:"name=protocols.bgp.vpn.paths,type=gauge,help=Total BGP VPN paths in this AF."`
}

// VPNSummary matches `show bgp ipv4 vpn summary json` and `show bgp ipv6
// vpn summary json`. The top-level scalars are gauges; the `peers` map
// (keyed by neighbor IP) is iterated and emitted per peer.
type VPNSummary struct {
	// AddressFamily is populated by the routing layer ("ipv4" or "ipv6")
	// so IPv4 and IPv6 VPN summary emit as distinct metric series.
	AddressFamily string `json:"-" metric:"label"`

	RouterId     string             `json:"routerId,omitempty"`
	As           uint32             `json:"as" metric:"name=protocols.bgp.vpn.summary.local_as,type=gauge,help=Local BGP AS for the VPN AF."`
	VrfId        uint32             `json:"vrfId,omitempty"`
	VrfName      string             `json:"vrfName,omitempty" metric:"label"`
	TableVersion uint64             `json:"tableVersion" metric:"name=protocols.bgp.vpn.summary.table_version,type=gauge,help=BGP VPN summary table version."`
	RibCount     uint64             `json:"ribCount" metric:"name=protocols.bgp.vpn.summary.rib_entries,type=gauge,help=BGP VPN RIB entries."`
	RibMemory    uint64             `json:"ribMemory" metric:"name=protocols.bgp.vpn.summary.rib_memory_bytes,type=gauge,help=BGP VPN RIB memory in bytes."`
	PeerCount    uint64             `json:"peerCount" metric:"name=protocols.bgp.vpn.summary.peers,type=gauge,help=BGP VPN peer count."`
	PeerMemory   uint64             `json:"peerMemory" metric:"name=protocols.bgp.vpn.summary.peer_memory_bytes,type=gauge,help=BGP VPN peer memory in bytes."`
	Peers        map[string]VPNPeer `json:"peers,omitempty" metric:"flatten"`
}

// VPNPeer is one entry from the `peers` map in VPNSummary. The map key
// (neighbor IP) projects onto NeighborAddr via metric:"map_key".
type VPNPeer struct {
	NeighborAddr               string `json:"-" metric:"label,map_key"`
	Hostname                   string `json:"hostname,omitempty" metric:"label"`
	SoftwareVersion            string `json:"softwareVersion,omitempty"`     // show-output only
	RemoteAs                   uint32 `json:"remoteAs,omitempty"`            // show-output only
	LocalAs                    uint32 `json:"localAs,omitempty"`             // show-output only
	Version                    int    `json:"version,omitempty"`             // show-output only
	State                      string `json:"state" metric:"label"`
	PeerState                  string `json:"peerState,omitempty"`           // show-output only
	MsgRcvd                    uint64 `json:"msgRcvd" metric:"name=protocols.bgp.vpn.peer.messages_recv,type=counter,help=BGP messages received from this VPN peer."`
	MsgSent                    uint64 `json:"msgSent" metric:"name=protocols.bgp.vpn.peer.messages_sent,type=counter,help=BGP messages sent to this VPN peer."`
	PfxRcd                     uint64 `json:"pfxRcd" metric:"name=protocols.bgp.vpn.peer.prefixes_recv,type=gauge,help=BGP prefixes received from this VPN peer."`
	PfxSnt                     uint64 `json:"pfxSnt" metric:"name=protocols.bgp.vpn.peer.prefixes_sent,type=gauge,help=BGP prefixes sent to this VPN peer."`
	PeerUptimeMsec             uint64 `json:"peerUptimeMsec" metric:"name=protocols.bgp.vpn.peer.uptime_ms,type=gauge,help=BGP VPN peer uptime in milliseconds."`
	PeerUptimeEstablishedEpoch int64  `json:"peerUptimeEstablishedEpoch,omitempty"` // show-output only
}
