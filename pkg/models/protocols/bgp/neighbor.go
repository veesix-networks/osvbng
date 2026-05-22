// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

type Neighbor struct {
	NeighborAddr    string `json:"neighborAddr" metric:"label=neighbor_addr,map_key"`
	RemoteAs        uint32 `json:"remoteAs" metric:"name=protocols.bgp.neighbor.remote_as,type=gauge,help=BGP remote AS number."`
	LocalAs         uint32 `json:"localAs" metric:"name=protocols.bgp.neighbor.local_as,type=gauge,help=BGP local AS number."`
	NbrInternalLink bool   `json:"nbrInternalLink,omitempty"`
	Hostname        string `json:"hostname,omitempty"`
	BgpState        string `json:"bgpState" metric:"label=state"`
	BgpVersion      int    `json:"bgpVersion,omitempty"`
	RemoteRouterId  string `json:"remoteRouterId,omitempty"`
	LocalRouterId   string `json:"localRouterId,omitempty"`

	BgpTimerUpMsec                 uint64 `json:"bgpTimerUpMsec" metric:"name=protocols.bgp.neighbor.uptime_ms,type=gauge,help=Time since the BGP session was established, in milliseconds."`
	BgpTimerUpString               string `json:"bgpTimerUpString,omitempty"`
	BgpTimerUpEstablishedEpoch     int64  `json:"bgpTimerUpEstablishedEpoch,omitempty"`
	BgpTimerLastRead               uint64 `json:"bgpTimerLastRead" metric:"name=protocols.bgp.neighbor.last_read_ms,type=gauge,help=Milliseconds since the last BGP message was read."`
	BgpTimerLastWrite              uint64 `json:"bgpTimerLastWrite" metric:"name=protocols.bgp.neighbor.last_write_ms,type=gauge,help=Milliseconds since the last BGP message was written."`
	BgpTimerHoldTimeMsecs          uint64 `json:"bgpTimerHoldTimeMsecs" metric:"name=protocols.bgp.neighbor.hold_time_ms,type=gauge,help=Negotiated BGP hold time in milliseconds."`
	BgpTimerKeepAliveIntervalMsecs uint64 `json:"bgpTimerKeepAliveIntervalMsecs" metric:"name=protocols.bgp.neighbor.keepalive_interval_ms,type=gauge,help=Negotiated BGP keepalive interval in milliseconds."`

	MessageStats           NeighborMessageStats `json:"messageStats" metric:"flatten"`
	ConnectionsEstablished uint64               `json:"connectionsEstablished" metric:"name=protocols.bgp.neighbor.connections_established,type=counter,help=Number of times the BGP session has reached Established state."`
	ConnectionsDropped     uint64               `json:"connectionsDropped" metric:"name=protocols.bgp.neighbor.connections_dropped,type=counter,help=Number of times the BGP session has dropped."`
}

type NeighborMessageStats struct {
	OpensSent         uint64 `json:"opensSent" metric:"name=protocols.bgp.neighbor.opens_sent,type=counter,help=BGP OPEN messages sent."`
	OpensRecv         uint64 `json:"opensRecv" metric:"name=protocols.bgp.neighbor.opens_recv,type=counter,help=BGP OPEN messages received."`
	UpdatesSent       uint64 `json:"updatesSent" metric:"name=protocols.bgp.neighbor.updates_sent,type=counter,help=BGP UPDATE messages sent."`
	UpdatesRecv       uint64 `json:"updatesRecv" metric:"name=protocols.bgp.neighbor.updates_recv,type=counter,help=BGP UPDATE messages received."`
	KeepalivesSent    uint64 `json:"keepalivesSent" metric:"name=protocols.bgp.neighbor.keepalives_sent,type=counter,help=BGP KEEPALIVE messages sent."`
	KeepalivesRecv    uint64 `json:"keepalivesRecv" metric:"name=protocols.bgp.neighbor.keepalives_recv,type=counter,help=BGP KEEPALIVE messages received."`
	NotificationsSent uint64 `json:"notificationsSent" metric:"name=protocols.bgp.neighbor.notifications_sent,type=counter,help=BGP NOTIFICATION messages sent."`
	NotificationsRecv uint64 `json:"notificationsRecv" metric:"name=protocols.bgp.neighbor.notifications_recv,type=counter,help=BGP NOTIFICATION messages received."`
	RouteRefreshSent  uint64 `json:"routeRefreshSent,omitempty" metric:"name=protocols.bgp.neighbor.route_refresh_sent,type=counter,help=BGP ROUTE-REFRESH messages sent."`
	RouteRefreshRecv  uint64 `json:"routeRefreshRecv,omitempty" metric:"name=protocols.bgp.neighbor.route_refresh_recv,type=counter,help=BGP ROUTE-REFRESH messages received."`
	CapabilitySent    uint64 `json:"capabilitySent,omitempty" metric:"name=protocols.bgp.neighbor.capability_sent,type=counter,help=BGP CAPABILITY messages sent."`
	CapabilityRecv    uint64 `json:"capabilityRecv,omitempty" metric:"name=protocols.bgp.neighbor.capability_recv,type=counter,help=BGP CAPABILITY messages received."`
	TotalSent         uint64 `json:"totalSent" metric:"name=protocols.bgp.neighbor.total_sent,type=counter,help=Total BGP messages sent."`
	TotalRecv         uint64 `json:"totalRecv" metric:"name=protocols.bgp.neighbor.total_recv,type=counter,help=Total BGP messages received."`
}
