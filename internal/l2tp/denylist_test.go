// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"net"
	"testing"
	"time"
)

func TestDenylistAddAndIsDenied(t *testing.T) {
	d := NewPeerDenylist()
	peer := net.IPv4(192, 0, 2, 10)

	if d.IsDenied(peer) {
		t.Fatal("fresh denylist should not deny anything")
	}
	d.Add(peer, "transport-failure", time.Minute)
	if !d.IsDenied(peer) {
		t.Fatal("peer should be denied within TTL")
	}
	if got := d.Reason(peer); got != "transport-failure" {
		t.Fatalf("Reason = %q, want transport-failure", got)
	}
}

func TestDenylistTTLExpiresToProbeEligible(t *testing.T) {
	d := NewPeerDenylist()
	peer := net.IPv4(192, 0, 2, 11)

	d.Add(peer, "stopccn", 1*time.Millisecond)
	time.Sleep(5 * time.Millisecond)

	if d.IsDenied(peer) {
		t.Fatal("expired entry must not deny")
	}
	if !d.ProbeEligible(peer) {
		t.Fatal("expired entry must be probe-eligible")
	}
}

func TestDenylistRemoveClears(t *testing.T) {
	d := NewPeerDenylist()
	peer := net.IPv4(192, 0, 2, 12)

	d.Add(peer, "auth-failure", time.Minute)
	d.Remove(peer)
	if d.IsDenied(peer) {
		t.Fatal("Remove must clear the entry")
	}
	if d.ProbeEligible(peer) {
		t.Fatal("removed peer is no longer in the list, not probe-eligible")
	}
}

func TestDenylistAddRefreshesTTL(t *testing.T) {
	d := NewPeerDenylist()
	peer := net.IPv4(192, 0, 2, 13)

	d.Add(peer, "first", 1*time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	d.Add(peer, "second", time.Minute)

	if !d.IsDenied(peer) {
		t.Fatal("re-Add must reset the TTL into the future")
	}
	if got := d.Reason(peer); got != "second" {
		t.Fatalf("Reason = %q after re-Add, want 'second'", got)
	}
}

func TestDenylistIgnoresZeroTTLOrNilIP(t *testing.T) {
	d := NewPeerDenylist()
	d.Add(nil, "x", time.Minute)
	d.Add(net.IPv4(1, 1, 1, 1), "x", 0)
	if d.Len() != 0 {
		t.Fatalf("invalid Add calls populated the map (len=%d)", d.Len())
	}
}
