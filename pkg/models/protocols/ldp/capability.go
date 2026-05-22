// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ldp

// CapabilityTLV mirrors one entry in `show mpls ldp capabilities json` or
// either side of `show mpls ldp neighbor <addr> capabilities json`.
type CapabilityTLV struct {
	Description string `json:"description"`
	TLVType     string `json:"tlvType"`
}

// NeighborCapabilities mirrors one entry in
// `show mpls ldp neighbor [<addr>] capabilities json` — sent/received TLV lists.
type NeighborCapabilities struct {
	SentCapabilities     []CapabilityTLV `json:"sentCapabilities,omitempty"`
	ReceivedCapabilities []CapabilityTLV `json:"receivedCapabilities,omitempty"`
}
