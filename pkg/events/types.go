// Copyright 2025 Veesix Networks Ltd
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
