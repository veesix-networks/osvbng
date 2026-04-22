// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"net"
	"sync"
)

type BlacklistManager struct {
	mu      sync.RWMutex
	entries map[string]map[string]bool
}

func NewBlacklistManager() *BlacklistManager {
	return &BlacklistManager{
		entries: make(map[string]map[string]bool),
	}
}

func (bm *BlacklistManager) Exclude(poolName string, ip net.IP) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if bm.entries[poolName] == nil {
		bm.entries[poolName] = make(map[string]bool)
	}
	bm.entries[poolName][ip.To4().String()] = true
}

func (bm *BlacklistManager) Include(poolName string, ip net.IP) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if bm.entries[poolName] != nil {
		delete(bm.entries[poolName], ip.To4().String())
	}
}

func (bm *BlacklistManager) IsExcluded(poolName string, ip net.IP) bool {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	if bm.entries[poolName] == nil {
		return false
	}
	return bm.entries[poolName][ip.To4().String()]
}

func (bm *BlacklistManager) GetExcluded(poolName string) []net.IP {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	var ips []net.IP
	for ipStr := range bm.entries[poolName] {
		ips = append(ips, net.ParseIP(ipStr).To4())
	}
	return ips
}
