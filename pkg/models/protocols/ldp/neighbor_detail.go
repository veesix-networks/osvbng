// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ldp

import "encoding/json"

// NeighborDetail mirrors `show mpls ldp neighbor <A.B.C.D> detail json`.
// CLI / API only; SentMessages, ReceivedMessages, and AddressList carry
// per-message counters and peer-address lists that vary across releases.
type NeighborDetail struct {
	PeerId             string          `json:"peerId"`
	TCPLocalAddress    string          `json:"tcpLocalAddress,omitempty"`
	TCPLocalPort       uint32          `json:"tcpLocalPort,omitempty"`
	TCPRemoteAddress   string          `json:"tcpRemoteAddress,omitempty"`
	TCPRemotePort      uint32          `json:"tcpRemotePort,omitempty"`
	Authentication     string          `json:"authentication,omitempty"`
	SessionHoldtime    uint32          `json:"sessionHoldtime,omitempty"`
	KeepAliveInterval  uint32          `json:"keepAliveInterval,omitempty"`
	State              string          `json:"state,omitempty"`
	UpTime             string          `json:"upTime,omitempty"`
	UpTimeSecs         uint64          `json:"-"`
	SentMessages       json.RawMessage `json:"sentMessages,omitempty"`
	ReceivedMessages   json.RawMessage `json:"receivedMessages,omitempty"`
	AddressList        json.RawMessage `json:"addressList,omitempty"`
}
