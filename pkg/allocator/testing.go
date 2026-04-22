// Copyright 2025 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package allocator

import "net/netip"

func NewTestRegistry(rangeStart, rangeEnd netip.Addr) *Registry {
	r := &Registry{
		allocators:       make(map[string]*PoolAllocator),
		profilePools:     make(map[string][]string),
		poolVRFs:         make(map[string]string),
		ianaAllocators:   make(map[string]*PoolAllocator),
		profileIANAPools: make(map[string][]string),
		pdAllocators:     make(map[string]*PrefixAllocator),
		profilePDPools:   make(map[string][]string),
	}
	r.allocators["test/pool"] = NewPoolAllocator(rangeStart, rangeEnd, nil)
	r.profilePools["test"] = []string{"test/pool"}
	return r
}
