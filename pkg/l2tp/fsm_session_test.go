// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import "testing"

func TestSessionFSMLACHappyPath(t *testing.T) {
	f := NewSessionFSM(SessionRoleLAC)
	if err := f.SendICRQ(); err != nil {
		t.Fatal(err)
	}
	if f.State() != SessionWaitReply {
		t.Fatalf("state after SendICRQ: %v", f.State())
	}
	if err := f.RecvICRP(); err != nil {
		t.Fatal(err)
	}
	if !f.CanForwardData() {
		t.Fatal("should be Established after RecvICRP")
	}
}

func TestSessionFSMLNSHappyPath(t *testing.T) {
	f := NewSessionFSM(SessionRoleLNS)
	if err := f.RecvICRQ(); err != nil {
		t.Fatal(err)
	}
	if err := f.RecvICCN(); err != nil {
		t.Fatal(err)
	}
	if f.State() != SessionEstablished {
		t.Fatalf("LNS state after RecvICCN: %v", f.State())
	}
}

func TestSessionFSMInvalidTransitions(t *testing.T) {
	lac := NewSessionFSM(SessionRoleLAC)
	if err := lac.RecvICRQ(); err != ErrBadSessionTransition {
		t.Fatal("LAC cannot RecvICRQ from Idle")
	}

	lns := NewSessionFSM(SessionRoleLNS)
	if err := lns.SendICRQ(); err != ErrBadSessionTransition {
		t.Fatal("LNS cannot SendICRQ")
	}
}

func TestSessionFSMDisconnect(t *testing.T) {
	f := NewSessionFSM(SessionRoleLAC)
	_ = f.SendICRQ()
	_ = f.RecvICRP()
	f.Disconnect()
	if f.State() != SessionCleanup {
		t.Fatalf("Disconnect should move to Cleanup, got %v", f.State())
	}
}
