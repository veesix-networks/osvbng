// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"net"
	"sync"

	"github.com/veesix-networks/osvbng/pkg/models"
)

type bypassRule struct {
	Network *net.IPNet
	VRFID   uint32
}

type BypassManager struct {
	mu    sync.RWMutex
	rules []bypassRule
}

func NewBypassManager() *BypassManager {
	return &BypassManager{}
}

func (bm *BypassManager) AddPrefix(prefix *net.IPNet, vrfID uint32) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	for _, r := range bm.rules {
		if r.Network.String() == prefix.String() && r.VRFID == vrfID {
			return
		}
	}

	bm.rules = append(bm.rules, bypassRule{
		Network: prefix,
		VRFID:   vrfID,
	})
}

func (bm *BypassManager) AddIP(ip net.IP, vrfID uint32) {
	ip4 := ip.To4()
	if ip4 == nil {
		return
	}
	prefix := &net.IPNet{
		IP:   ip4,
		Mask: net.CIDRMask(32, 32),
	}
	bm.AddPrefix(prefix, vrfID)
}

func (bm *BypassManager) RemovePrefix(prefix *net.IPNet, vrfID uint32) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	for i, r := range bm.rules {
		if r.Network.String() == prefix.String() && r.VRFID == vrfID {
			bm.rules = append(bm.rules[:i], bm.rules[i+1:]...)
			return
		}
	}
}

func (bm *BypassManager) IsBypassed(ip net.IP, vrfID uint32) bool {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}

	for _, r := range bm.rules {
		if r.VRFID != vrfID {
			continue
		}
		if r.Network.Contains(ip4) {
			return true
		}
	}
	return false
}

func (bm *BypassManager) GetAll() []models.CGNATBypassEntry {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	entries := make([]models.CGNATBypassEntry, 0, len(bm.rules))
	for _, r := range bm.rules {
		entries = append(entries, models.CGNATBypassEntry{
			Prefix:      r.Network.String(),
			InsideVRFID: r.VRFID,
		})
	}
	return entries
}
