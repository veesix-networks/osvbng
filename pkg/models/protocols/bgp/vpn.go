// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import "encoding/json"

type VPNRoutes struct {
	AddressFamily string `json:"-" metric:"label"`

	VrfId         uint32          `json:"vrfId,omitempty"`
	VrfName       string          `json:"vrfName,omitempty" metric:"label"`
	TableVersion  uint64          `json:"tableVersion" metric:"name=protocols.bgp.vpn.table_version,type=gauge,help=BGP VPN table version."`
	RouterId      string          `json:"routerId,omitempty"`
	DefaultLocPrf uint32          `json:"defaultLocPrf,omitempty"`
	LocalAS       uint32          `json:"localAS,omitempty"`
	Routes        json.RawMessage `json:"routes,omitempty"`
	TotalRoutes   uint64          `json:"totalRoutes" metric:"name=protocols.bgp.vpn.routes,type=gauge,help=Total BGP VPN routes in this AF."`
	TotalPaths    uint64          `json:"totalPaths" metric:"name=protocols.bgp.vpn.paths,type=gauge,help=Total BGP VPN paths in this AF."`
}

type VPNSummary struct {
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

type VPNPeer struct {
	NeighborAddr               string `json:"-" metric:"label,map_key"`
	Hostname                   string `json:"hostname,omitempty"`
	SoftwareVersion            string `json:"softwareVersion,omitempty"`
	RemoteAs                   uint32 `json:"remoteAs,omitempty"`
	LocalAs                    uint32 `json:"localAs,omitempty"`
	Version                    int    `json:"version,omitempty"`
	State                      string `json:"state" metric:"label"`
	PeerState                  string `json:"peerState,omitempty"`
	MsgRcvd                    uint64 `json:"msgRcvd" metric:"name=protocols.bgp.vpn.peer.messages_recv,type=counter,help=BGP messages received from this VPN peer."`
	MsgSent                    uint64 `json:"msgSent" metric:"name=protocols.bgp.vpn.peer.messages_sent,type=counter,help=BGP messages sent to this VPN peer."`
	PfxRcd                     uint64 `json:"pfxRcd" metric:"name=protocols.bgp.vpn.peer.prefixes_recv,type=gauge,help=BGP prefixes received from this VPN peer."`
	PfxSnt                     uint64 `json:"pfxSnt" metric:"name=protocols.bgp.vpn.peer.prefixes_sent,type=gauge,help=BGP prefixes sent to this VPN peer."`
	PeerUptimeMsec             uint64 `json:"peerUptimeMsec" metric:"name=protocols.bgp.vpn.peer.uptime_ms,type=gauge,help=BGP VPN peer uptime in milliseconds."`
	PeerUptimeEstablishedEpoch int64  `json:"peerUptimeEstablishedEpoch,omitempty"`
}
