// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"errors"
	"sync"
)

// IDAllocator hands out non-zero u16 identifiers from a monotonic
// counter, wrapping at 65535. On wraparound it scans for a free slot,
// falling back to ErrIDExhausted only when all 65,535 IDs are held.
// Safe for concurrent use.
type IDAllocator struct {
	mu       sync.Mutex
	next     uint16
	inUse    map[uint16]struct{}
	wrapped  bool
}

var ErrIDExhausted = errors.New("l2tp: id allocator exhausted")

func NewIDAllocator() *IDAllocator {
	return &IDAllocator{
		next:  1,
		inUse: make(map[uint16]struct{}),
	}
}

func (a *IDAllocator) Allocate() (uint16, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.wrapped {
		id := a.next
		a.next++
		if a.next == 0 {
			a.next = 1
			a.wrapped = true
		}
		if _, taken := a.inUse[id]; !taken {
			a.inUse[id] = struct{}{}
			return id, nil
		}
	}

	for i := 0; i < 0xffff; i++ {
		id := a.next
		a.next++
		if a.next == 0 {
			a.next = 1
		}
		if _, taken := a.inUse[id]; !taken {
			a.inUse[id] = struct{}{}
			return id, nil
		}
	}
	return 0, ErrIDExhausted
}

func (a *IDAllocator) Release(id uint16) {
	if id == 0 {
		return
	}
	a.mu.Lock()
	delete(a.inUse, id)
	a.mu.Unlock()
}

// Reserve marks an ID as in-use without bumping the counter past it.
// Used after HA restore to lock the IDs of restored sessions before
// allocating fresh IDs for new sessions.
func (a *IDAllocator) Reserve(id uint16) bool {
	if id == 0 {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, taken := a.inUse[id]; taken {
		return false
	}
	a.inUse[id] = struct{}{}
	if !a.wrapped && id >= a.next {
		a.next = id + 1
		if a.next == 0 {
			a.next = 1
			a.wrapped = true
		}
	}
	return true
}

// Count returns the number of IDs currently held.
func (a *IDAllocator) Count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.inUse)
}
