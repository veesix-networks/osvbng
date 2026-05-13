// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import "testing"

func TestTunnelFSMInitiatorHappyPath(t *testing.T) {
	f := NewTunnelFSM(RoleInitiator)
	if err := f.SendSCCRQ(); err != nil {
		t.Fatal(err)
	}
	if f.State() != TunnelWaitCtlReply {
		t.Fatalf("state after SCCRQ: %v", f.State())
	}
	if err := f.RecvSCCRP(); err != nil {
		t.Fatal(err)
	}
	if !f.CanForwardSession() {
		t.Fatal("should be Established after SCCRP")
	}
}

func TestTunnelFSMResponderHappyPath(t *testing.T) {
	f := NewTunnelFSM(RoleResponder)
	if err := f.RecvSCCRQ(); err != nil {
		t.Fatal(err)
	}
	if f.State() != TunnelWaitCtlConn {
		t.Fatalf("state after RecvSCCRQ: %v", f.State())
	}
	if err := f.RecvSCCCN(); err != nil {
		t.Fatal(err)
	}
	if f.State() != TunnelEstablished {
		t.Fatalf("state after RecvSCCCN: %v", f.State())
	}
}

func TestTunnelFSMInvalidTransitions(t *testing.T) {
	f := NewTunnelFSM(RoleInitiator)
	if err := f.RecvSCCRP(); err != ErrBadTunnelTransition {
		t.Fatalf("RecvSCCRP from Idle should be invalid, got %v", err)
	}

	r := NewTunnelFSM(RoleResponder)
	if err := r.SendSCCRQ(); err != ErrBadTunnelTransition {
		t.Fatalf("Responder cannot SendSCCRQ, got %v", err)
	}
}

func TestTunnelFSMStop(t *testing.T) {
	f := NewTunnelFSM(RoleInitiator)
	_ = f.SendSCCRQ()
	f.Stop()
	if f.State() != TunnelCleanup {
		t.Fatalf("Stop should move to Cleanup, got %v", f.State())
	}
	if f.CanForwardSession() {
		t.Fatal("Cleanup must not allow data forwarding")
	}
}
