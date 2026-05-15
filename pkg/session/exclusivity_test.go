// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package session

import (
	"fmt"
	"net"
	"sync"
	"testing"
)

func mac(s string) net.HardwareAddr {
	hw, err := net.ParseMAC(s)
	if err != nil {
		panic(err)
	}
	return hw
}

func TestClaim_EmptyTupleNoPrevious(t *testing.T) {
	r := NewRegistry()
	k := MakeTupleKey(100, 200, mac("aa:bb:cc:00:00:01"))
	owner := Owner{Protocol: ProtoIPoE, SessionID: "s1", Key: k}
	if prev := r.Claim(k, owner); prev != nil {
		t.Fatalf("expected nil previous, got %+v", prev)
	}
	if !r.IsOwner(k, owner) {
		t.Fatal("expected current owner to match")
	}
}

func TestClaim_SameProtocolSameSessionIsIdempotent(t *testing.T) {
	r := NewRegistry()
	k := MakeTupleKey(100, 200, mac("aa:bb:cc:00:00:01"))
	owner := Owner{Protocol: ProtoIPoE, SessionID: "s1", Key: k}
	r.Claim(k, owner)
	if prev := r.Claim(k, owner); prev != nil {
		t.Fatalf("re-claim by same owner should return nil, got %+v", prev)
	}
}

func TestClaim_CrossProtocolReturnsPreviousAndOverwrites(t *testing.T) {
	r := NewRegistry()
	k := MakeTupleKey(100, 200, mac("aa:bb:cc:00:00:01"))
	first := Owner{Protocol: ProtoIPoE, SessionID: "ipoe-1", Key: k}
	second := Owner{Protocol: ProtoPPPoE, SessionID: "pppoe-1", Key: k}

	r.Claim(k, first)
	prev := r.Claim(k, second)
	if prev == nil {
		t.Fatal("cross-protocol claim should return previous owner")
	}
	if prev.Protocol != ProtoIPoE || prev.SessionID != "ipoe-1" {
		t.Fatalf("previous owner wrong: %+v", prev)
	}
	if !r.IsOwner(k, second) {
		t.Fatal("second owner should now own the tuple")
	}
	if r.IsOwner(k, first) {
		t.Fatal("first owner should no longer own the tuple")
	}
}

func TestRelease_OwnedTupleFreed(t *testing.T) {
	r := NewRegistry()
	k := MakeTupleKey(100, 200, mac("aa:bb:cc:00:00:01"))
	owner := Owner{Protocol: ProtoIPoE, SessionID: "s1", Key: k}
	r.Claim(k, owner)
	r.Release(k, owner)
	if r.Lookup(k) != nil {
		t.Fatal("Release should have freed the tuple")
	}
}

func TestRelease_OtherOwnerIsNoOp(t *testing.T) {
	r := NewRegistry()
	k := MakeTupleKey(100, 200, mac("aa:bb:cc:00:00:01"))
	first := Owner{Protocol: ProtoIPoE, SessionID: "ipoe-1", Key: k}
	second := Owner{Protocol: ProtoPPPoE, SessionID: "pppoe-1", Key: k}

	r.Claim(k, first)
	r.Claim(k, second)
	r.Release(k, first)
	if !r.IsOwner(k, second) {
		t.Fatal("Release by stale owner must not affect current owner")
	}
}

func TestLookup_Unknown(t *testing.T) {
	r := NewRegistry()
	if got := r.Lookup(MakeTupleKey(1, 2, mac("aa:bb:cc:00:00:01"))); got != nil {
		t.Fatalf("expected nil lookup, got %+v", got)
	}
}

func TestTupleIdentity_DistinctCVLANsDoNotCollide(t *testing.T) {
	r := NewRegistry()
	hw := mac("aa:bb:cc:00:00:01")
	k1 := MakeTupleKey(100, 42, hw)
	k2 := MakeTupleKey(100, 99, hw)
	owner1 := Owner{Protocol: ProtoIPoE, SessionID: "s1", Key: k1}
	owner2 := Owner{Protocol: ProtoIPoE, SessionID: "s2", Key: k2}

	if prev := r.Claim(k1, owner1); prev != nil {
		t.Fatalf("k1 claim should return nil, got %+v", prev)
	}
	if prev := r.Claim(k2, owner2); prev != nil {
		t.Fatalf("k2 claim with different CVLAN must not collide with k1, got %+v", prev)
	}
	if !r.IsOwner(k1, owner1) || !r.IsOwner(k2, owner2) {
		t.Fatal("both tuples should retain their owners")
	}
}

func TestConcurrentClaimRelease_NoRaces(t *testing.T) {
	r := NewRegistry()
	const N = 64
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			k := MakeTupleKey(uint16(100+i), uint16(1), mac("aa:bb:cc:00:00:01"))
			owner := Owner{Protocol: ProtoIPoE, SessionID: fmt.Sprintf("s-%d", i), Key: k}
			for j := 0; j < 100; j++ {
				r.Claim(k, owner)
				r.IsOwner(k, owner)
				r.Lookup(k)
				r.Release(k, owner)
			}
		}()
	}
	wg.Wait()
}
