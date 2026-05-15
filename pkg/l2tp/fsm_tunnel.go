// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import "errors"

// TunnelState is the L2TPv2 control-connection establishment FSM per
// RFC 2661 §7.2. The initiator and responder roles share the state
// space; only the transitions taken differ.
type TunnelState int

const (
	TunnelIdle TunnelState = iota

	// Initiator side: SCCRQ sent, awaiting SCCRP.
	TunnelWaitCtlReply

	// Responder side: SCCRP sent, awaiting SCCCN.
	TunnelWaitCtlConn

	// SCCCN exchanged in both directions; data may flow.
	TunnelEstablished

	// StopCCN received or sent; pending cleanup of session state and
	// dataplane teardown.
	TunnelCleanup
)

func (s TunnelState) String() string {
	switch s {
	case TunnelIdle:
		return "Idle"
	case TunnelWaitCtlReply:
		return "WaitCtlReply"
	case TunnelWaitCtlConn:
		return "WaitCtlConn"
	case TunnelEstablished:
		return "Established"
	case TunnelCleanup:
		return "Cleanup"
	}
	return "Unknown"
}

// TunnelRole indicates who initiated the tunnel. Set when the local
// side either sends or receives the SCCRQ.
type TunnelRole int

const (
	RoleInitiator TunnelRole = iota
	RoleResponder
)

var ErrBadTunnelTransition = errors.New("l2tp: invalid tunnel FSM transition")

// TunnelFSM is the per-tunnel control-plane state machine. It does
// not own the control channel or send packets; it consumes events
// from the channel and returns the action the caller should take.
type TunnelFSM struct {
	state TunnelState
	role  TunnelRole
}

func NewTunnelFSM(role TunnelRole) *TunnelFSM {
	return &TunnelFSM{state: TunnelIdle, role: role}
}

func (f *TunnelFSM) State() TunnelState { return f.state }
func (f *TunnelFSM) Role() TunnelRole   { return f.role }

// SendSCCRQ transitions Idle → WaitCtlReply (initiator only).
func (f *TunnelFSM) SendSCCRQ() error {
	if f.state != TunnelIdle || f.role != RoleInitiator {
		return ErrBadTunnelTransition
	}
	f.state = TunnelWaitCtlReply
	return nil
}

// RecvSCCRQ transitions Idle → WaitCtlConn (responder side).
func (f *TunnelFSM) RecvSCCRQ() error {
	if f.state != TunnelIdle || f.role != RoleResponder {
		return ErrBadTunnelTransition
	}
	f.state = TunnelWaitCtlConn
	return nil
}

// RecvSCCRP transitions WaitCtlReply → Established (initiator), after
// which the caller sends SCCCN.
func (f *TunnelFSM) RecvSCCRP() error {
	if f.state != TunnelWaitCtlReply {
		return ErrBadTunnelTransition
	}
	f.state = TunnelEstablished
	return nil
}

// RecvSCCCN transitions WaitCtlConn → Established (responder).
func (f *TunnelFSM) RecvSCCCN() error {
	if f.state != TunnelWaitCtlConn {
		return ErrBadTunnelTransition
	}
	f.state = TunnelEstablished
	return nil
}

// Stop transitions any state to Cleanup. Idempotent.
func (f *TunnelFSM) Stop() {
	f.state = TunnelCleanup
}

// CanForwardSession returns true iff data-plane sessions may be
// programmed for this tunnel. Conservatively false until the tunnel
// is Established.
func (f *TunnelFSM) CanForwardSession() bool {
	return f.state == TunnelEstablished
}
