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
