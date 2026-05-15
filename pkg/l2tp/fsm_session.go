// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import "errors"

// SessionState is the L2TPv2 incoming-call FSM per RFC 2661 §7.4.1
// (LAC) and §7.4.2 (LNS). Outgoing-call (OCRQ/OCRP/OCCN) is not used
// in v1 — out-of-scope per the implementation spec — and is therefore
// not represented here.
type SessionState int

const (
	SessionIdle SessionState = iota

	// LAC side: ICRQ sent, awaiting ICRP.
	SessionWaitReply

	// LAC side: ICRP received, ICCN sent. Bring-up complete.
	// LNS side: ICCN received. Bring-up complete.
	SessionEstablished

	// CDN received or sent; pending teardown of session state.
	SessionCleanup
)

func (s SessionState) String() string {
	switch s {
	case SessionIdle:
		return "Idle"
	case SessionWaitReply:
		return "WaitReply"
	case SessionEstablished:
		return "Established"
	case SessionCleanup:
		return "Cleanup"
	}
	return "Unknown"
}

// SessionRole mirrors TunnelRole for sessions. The LAC initiates
// incoming-call sessions; the LNS responds. For v1 we only handle
// incoming calls.
type SessionRole int

const (
	SessionRoleLAC SessionRole = iota // initiator of the incoming-call exchange
	SessionRoleLNS                    // responder
)

var ErrBadSessionTransition = errors.New("l2tp: invalid session FSM transition")

type SessionFSM struct {
	state SessionState
	role  SessionRole
}

func NewSessionFSM(role SessionRole) *SessionFSM {
	return &SessionFSM{state: SessionIdle, role: role}
}

func (f *SessionFSM) State() SessionState { return f.state }
func (f *SessionFSM) Role() SessionRole   { return f.role }

// SendICRQ transitions Idle → WaitReply (LAC only).
func (f *SessionFSM) SendICRQ() error {
	if f.state != SessionIdle || f.role != SessionRoleLAC {
		return ErrBadSessionTransition
	}
	f.state = SessionWaitReply
	return nil
}

// RecvICRQ transitions Idle → WaitReply (LNS side; the LNS will reply
// with ICRP). We model the LNS's "received ICRQ, about to send ICRP"
// as the same WaitReply state from the FSM's perspective; the role
// disambiguates the meaning.
func (f *SessionFSM) RecvICRQ() error {
	if f.state != SessionIdle || f.role != SessionRoleLNS {
		return ErrBadSessionTransition
	}
	f.state = SessionWaitReply
	return nil
}

// RecvICRP transitions WaitReply → Established (LAC side; the LAC has
// received the ICRP and is about to send ICCN).
func (f *SessionFSM) RecvICRP() error {
	if f.state != SessionWaitReply || f.role != SessionRoleLAC {
		return ErrBadSessionTransition
	}
	f.state = SessionEstablished
	return nil
}

// RecvICCN transitions WaitReply → Established (LNS side).
func (f *SessionFSM) RecvICCN() error {
	if f.state != SessionWaitReply || f.role != SessionRoleLNS {
		return ErrBadSessionTransition
	}
	f.state = SessionEstablished
	return nil
}

// Disconnect transitions any state to Cleanup. Idempotent. Driven by
// either receiving a CDN or sending one locally.
func (f *SessionFSM) Disconnect() {
	f.state = SessionCleanup
}

// CanForwardData returns true iff data packets may flow through the
// session. False until ICCN exchange completes.
func (f *SessionFSM) CanForwardData() bool {
	return f.state == SessionEstablished
}
