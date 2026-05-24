// Copyright 2025 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package events

import (
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/session"
)

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
	Key           *session.TupleKey
}

// RestoreCause identifies which recovery scenario produced a
// TopicSessionRestored emission. Lets consumers that care to differentiate
// (e.g. emit a "mappings restored from opdb" counter distinct from
// "mappings installed fresh") branch on it; consumers that do not care
// treat all causes identically.
type RestoreCause string

const (
	// RestoreCauseOsvbngdRestart marks restoration after osvbngd
	// respawned while the dataplane (VPP plugin state, ifMgr) remained
	// intact. setupSession's idempotent path is the only work needed
	// per-session in this case.
	RestoreCauseOsvbngdRestart RestoreCause = "osvbngd_restart"

	// RestoreCauseVPPRecovery marks restoration after VPP itself
	// restarted while osvbngd continued running. Plugin pools are empty
	// and every session is replayed through the plugin Add path.
	RestoreCauseVPPRecovery RestoreCause = "vpp_recovery"

	// RestoreCauseColdBoot marks restoration at full system bring-up
	// where both osvbngd and VPP are starting fresh from opdb-on-disk
	// state.
	RestoreCauseColdBoot RestoreCause = "cold_boot"

	// RestoreCauseHAFailover marks restoration on the just-promoted
	// node of an HA pair. Reserved for the HA-side setupSession
	// adoption (osvbng-context#94); not emitted by the in-this-spec
	// recovery paths.
	RestoreCauseHAFailover RestoreCause = "ha_failover"
)

// SessionRestoredEvent is the TopicSessionRestored payload. Carries the
// same SubscriberSession shape as SessionLifecycleEvent so consumer
// handlers can share code; the RestoreCause is supplied so a consumer
// that needs to distinguish opdb-restore from fresh-install can.
type SessionRestoredEvent struct {
	AccessType   models.AccessType
	Protocol     models.Protocol
	SessionID    string
	Session      models.SubscriberSession
	RestoreCause RestoreCause
}

// ComponentReadyEvent is the TopicComponentReady payload. Component is the
// component's Name() (e.g. "ipoe", "pppoe", "cgnat"); State is the lowercase
// readiness state string (always "ready" at publish time today, included so
// future state transitions can reuse the topic).
type ComponentReadyEvent struct {
	Component string
	State     string
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
