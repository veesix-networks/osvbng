// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

type SummaryAFI struct {
	VRF          string                 `json:"-"                          metric:"label=vrf,map_key"`
	RouterID     string                 `json:"routerId"`
	AS           uint32                 `json:"as"                         metric:"name=protocols.bgp.summary.local_as,type=gauge,help=BGP local AS number."`
	VRFID        uint32                 `json:"vrfId,omitempty"`
	VRFName      string                 `json:"vrfName,omitempty"`
	TableVersion uint64                 `json:"tableVersion"               metric:"name=protocols.bgp.summary.table_version,type=gauge,help=BGP table version."`
	RIBCount     uint64                 `json:"ribCount"                   metric:"name=protocols.bgp.summary.rib_entries,type=gauge,help=BGP RIB entries."`
	RIBMemory    uint64                 `json:"ribMemory"                  metric:"name=protocols.bgp.summary.rib_memory_bytes,type=gauge,help=BGP RIB memory in bytes."`
	PeerCount    uint64                 `json:"peerCount"                  metric:"name=protocols.bgp.summary.peers,type=gauge,help=BGP peer count."`
	PeerMemory   uint64                 `json:"peerMemory"                 metric:"name=protocols.bgp.summary.peer_memory_bytes,type=gauge,help=BGP peer memory in bytes."`
	Peers        map[string]SummaryPeer `json:"peers,omitempty"            metric:"flatten"`
	FailedPeers  uint64                 `json:"failedPeers,omitempty"      metric:"name=protocols.bgp.summary.failed_peers,type=gauge,help=BGP peers in non-established state."`
	TotalPeers   uint64                 `json:"totalPeers,omitempty"       metric:"name=protocols.bgp.summary.total_peers,type=gauge,help=BGP total peers configured."`
	DynamicPeers uint64                 `json:"dynamicPeers,omitempty"     metric:"name=protocols.bgp.summary.dynamic_peers,type=gauge,help=BGP dynamic peers."`
}

type SummaryPeer struct {
	NeighborAddr           string `json:"-"                              metric:"label=neighbor_addr,map_key"`
	Hostname               string `json:"hostname,omitempty"`
	RemoteAS               uint32 `json:"remoteAs"                       metric:"name=protocols.bgp.summary.peer.remote_as,type=gauge,help=BGP peer remote AS."`
	LocalAS                uint32 `json:"localAs"                        metric:"name=protocols.bgp.summary.peer.local_as,type=gauge,help=BGP peer local AS."`
	Version                int    `json:"version"`
	State                  string `json:"state"                          metric:"label=state"`
	PeerState              string `json:"peerState,omitempty"`
	MsgRcvd                uint64 `json:"msgRcvd"                        metric:"name=protocols.bgp.summary.peer.messages_recv,type=counter,help=BGP messages received from this peer."`
	MsgSent                uint64 `json:"msgSent"                        metric:"name=protocols.bgp.summary.peer.messages_sent,type=counter,help=BGP messages sent to this peer."`
	PfxRcd                 uint64 `json:"pfxRcd"                         metric:"name=protocols.bgp.summary.peer.prefixes_recv,type=gauge,help=BGP prefixes received from this peer."`
	PfxSnt                 uint64 `json:"pfxSnt"                         metric:"name=protocols.bgp.summary.peer.prefixes_sent,type=gauge,help=BGP prefixes sent to this peer."`
	PeerUptimeMsec         uint64 `json:"peerUptimeMsec,omitempty"       metric:"name=protocols.bgp.summary.peer.uptime_ms,type=gauge,help=BGP peer uptime in milliseconds."`
	ConnectionsEstablished uint64 `json:"connectionsEstablished"         metric:"name=protocols.bgp.summary.peer.connections_established,type=counter,help=BGP connections established with this peer."`
	ConnectionsDropped     uint64 `json:"connectionsDropped"             metric:"name=protocols.bgp.summary.peer.connections_dropped,type=counter,help=BGP connections dropped with this peer."`
}
