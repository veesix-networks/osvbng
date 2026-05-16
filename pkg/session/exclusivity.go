// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package session

import (
	"net"
	"sync"
)

type Protocol string

const (
	ProtoIPoE  Protocol = "ipoe"
	ProtoPPPoE Protocol = "pppoe"
)

type TupleKey struct {
	SVLAN uint16
	CVLAN uint16
	MAC   [6]byte
}

func MakeTupleKey(svlan, cvlan uint16, mac net.HardwareAddr) TupleKey {
	var k TupleKey
	k.SVLAN = svlan
	k.CVLAN = cvlan
	copy(k.MAC[:], mac)
	return k
}

type Owner struct {
	Protocol  Protocol
	SessionID string
	Key       TupleKey
}

type ExclusivityRegistry interface {
	Claim(key TupleKey, owner Owner) *Owner
	Release(key TupleKey, owner Owner)
	IsOwner(key TupleKey, owner Owner) bool
	Lookup(key TupleKey) *Owner
}

type Registry struct {
	mu    sync.RWMutex
	owned map[TupleKey]Owner
}

func NewRegistry() *Registry {
	return &Registry{owned: make(map[TupleKey]Owner)}
}

func (r *Registry) Claim(key TupleKey, owner Owner) *Owner {
	r.mu.Lock()
	defer r.mu.Unlock()
	prev, exists := r.owned[key]
	r.owned[key] = owner
	if !exists {
		return nil
	}
	if prev.Protocol == owner.Protocol && prev.SessionID == owner.SessionID {
		return nil
	}
	return &prev
}

func (r *Registry) Release(key TupleKey, owner Owner) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, exists := r.owned[key]
	if !exists {
		return
	}
	if cur.Protocol != owner.Protocol || cur.SessionID != owner.SessionID {
		return
	}
	delete(r.owned, key)
}

func (r *Registry) IsOwner(key TupleKey, owner Owner) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cur, exists := r.owned[key]
	if !exists {
		return false
	}
	return cur.Protocol == owner.Protocol && cur.SessionID == owner.SessionID
}

func (r *Registry) Lookup(key TupleKey) *Owner {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cur, exists := r.owned[key]
	if !exists {
		return nil
	}
	return &cur
}
