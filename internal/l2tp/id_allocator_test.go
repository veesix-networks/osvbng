// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import "testing"

func TestAllocateMonotonic(t *testing.T) {
	a := NewIDAllocator()
	first, err := a.Allocate()
	if err != nil || first != 1 {
		t.Fatalf("first allocation: id=%d err=%v", first, err)
	}
	second, _ := a.Allocate()
	if second != 2 {
		t.Fatalf("second should be 2, got %d", second)
	}
}

func TestAllocateNeverZero(t *testing.T) {
	a := NewIDAllocator()
	for i := 0; i < 100; i++ {
		id, err := a.Allocate()
		if err != nil {
			t.Fatal(err)
		}
		if id == 0 {
			t.Fatalf("allocator returned 0 on iteration %d", i)
		}
	}
}

func TestReleaseFrees(t *testing.T) {
	a := NewIDAllocator()
	id, _ := a.Allocate()
	if a.Count() != 1 {
		t.Fatal("count should be 1")
	}
	a.Release(id)
	if a.Count() != 0 {
		t.Fatal("count should be 0 after release")
	}
}

func TestReserveLocksRestoredID(t *testing.T) {
	a := NewIDAllocator()
	if !a.Reserve(100) {
		t.Fatal("Reserve should succeed for fresh ID")
	}
	if a.Reserve(100) {
		t.Fatal("second Reserve of same ID should fail")
	}
	// Counter should have advanced past 100.
	next, _ := a.Allocate()
	if next != 101 {
		t.Fatalf("next allocation should be 101 after Reserve(100), got %d", next)
	}
}

func TestWraparoundReusesFree(t *testing.T) {
	a := NewIDAllocator()
	// Allocate two, release the first, force wraparound.
	first, _ := a.Allocate()
	_, _ = a.Allocate()
	a.Release(first)
	// Force the counter to wrap.
	a.mu.Lock()
	a.next = 0xfffe
	a.mu.Unlock()
	_, _ = a.Allocate() // 0xfffe
	_, _ = a.Allocate() // 0xffff
	// Next call wraps.
	id, err := a.Allocate()
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("post-wrap id must be non-zero")
	}
	if id != first {
		// We expect the freed id to be reclaimed once we scan.
		t.Fatalf("post-wrap should reclaim freed id %d, got %d", first, id)
	}
}

func TestExhaustionReturnsError(t *testing.T) {
	a := NewIDAllocator()
	// Mark every id as in-use to simulate exhaustion.
	a.mu.Lock()
	for id := uint16(1); id != 0; id++ {
		a.inUse[id] = struct{}{}
	}
	a.wrapped = true
	a.mu.Unlock()
	if _, err := a.Allocate(); err != ErrIDExhausted {
		t.Fatalf("want ErrIDExhausted, got %v", err)
	}
}
