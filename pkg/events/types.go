// Copyright 2025 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package events

import "github.com/veesix-networks/osvbng/pkg/models"

type SessionLifecycleEvent struct {
	AccessType models.AccessType
	Protocol   models.Protocol
	SessionID  string
	State      models.SessionState
	Session    any
}

type AAARequestEvent struct {
	AccessType models.AccessType
	Protocol   models.Protocol
	SessionID  string
	Request    models.AAARequest
}

type AAAResponseEvent struct {
	AccessType models.AccessType
	SessionID  string
	Response   models.AAAResponse
}

type EgressEvent struct {
	Protocol models.Protocol
	Packet   models.EgressPacketPayload
}

type HAStateChangeEvent struct {
	SRGName  string
	OldState string
	NewState string
}

type InterfaceStateEvent struct {
	SwIfIndex uint32
	Name      string
	AdminUp   bool
	LinkUp    bool
	Deleted   bool
}

type CGNATMappingEvent struct {
	SRGName   string
	SessionID string
	Mapping   *models.CGNATMapping
	IsAdd     bool
}

type SubscriberMutationEvent struct {
	RequestID      string
	SessionID      string
	AcctSessionID  string
	Username       string
	FramedIPv4     string
	FramedIPv6     string
	AttributeDelta map[string]string
}

type SubscriberMutationResultEvent struct {
	RequestID  string
	SessionID  string
	Ok         bool
	Error      string
	ErrorCause int
	Session    models.SubscriberSession
}

type SubscriberTerminateEvent struct {
	SessionID     string
	AcctSessionID string
	Username      string
	FramedIPv4    string
	FramedIPv6    string
	Reason        string
}

// L2TPLACDecisionEvent communicates the LAC bring-up outcome back to
// the PPPoE component. Published on TopicL2TPLACDecision once the L2TP
// component has either established the tunneled session or exhausted
// its candidate list.
//
// `PPPoESessionID` keys the PPPoE-side session lookup. On success,
// `LocalIP / PeerIP / LocalTunnelID / PeerSessionID / LACL2TPSessionIndex`
// describe the dataplane binding the PPPoE plugin needs to set the
// `is_lac_tunneled` flag and the L2TP opaque on the PPPoE session
// struct. On failure, `Error` carries the reason and the PPPoE side
// must respond to the subscriber with PAP-Nak / CHAP-Failure.
type L2TPLACDecisionEvent struct {
	PPPoESessionID       uint16
	Success              bool
	Error                string
	LocalIP              string
	PeerIP               string
	LocalTunnelID        uint16
	PeerTunnelID         uint16
	LocalSessionID       uint16
	PeerSessionID        uint16
	LACL2TPSessionIndex  uint32
}
