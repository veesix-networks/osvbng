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

const numShards = 16

type shard struct {
	mu    sync.RWMutex
	owned map[TupleKey]Owner
}

type Registry struct {
	shards [numShards]*shard
}

func NewRegistry() *Registry {
	r := &Registry{}
	for i := range r.shards {
		r.shards[i] = &shard{owned: make(map[TupleKey]Owner)}
	}
	return r
}

func (r *Registry) shardFor(k TupleKey) *shard {
	h := uint32(k.SVLAN)<<16 | uint32(k.CVLAN)
	h ^= uint32(k.MAC[0])<<24 | uint32(k.MAC[1])<<16 | uint32(k.MAC[2])<<8 | uint32(k.MAC[3])
	h ^= uint32(k.MAC[4])<<8 | uint32(k.MAC[5])
	return r.shards[h&(numShards-1)]
}

func (r *Registry) Claim(key TupleKey, owner Owner) *Owner {
	s := r.shardFor(key)
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, exists := s.owned[key]
	s.owned[key] = owner
	if !exists {
		return nil
	}
	if prev.Protocol == owner.Protocol && prev.SessionID == owner.SessionID {
		return nil
	}
	return &prev
}

func (r *Registry) Release(key TupleKey, owner Owner) {
	s := r.shardFor(key)
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, exists := s.owned[key]
	if !exists {
		return
	}
	if cur.Protocol != owner.Protocol || cur.SessionID != owner.SessionID {
		return
	}
	delete(s.owned, key)
}

func (r *Registry) IsOwner(key TupleKey, owner Owner) bool {
	s := r.shardFor(key)
	s.mu.RLock()
	defer s.mu.RUnlock()
	cur, exists := s.owned[key]
	if !exists {
		return false
	}
	return cur.Protocol == owner.Protocol && cur.SessionID == owner.SessionID
}

func (r *Registry) Lookup(key TupleKey) *Owner {
	s := r.shardFor(key)
	s.mu.RLock()
	defer s.mu.RUnlock()
	cur, exists := s.owned[key]
	if !exists {
		return nil
	}
	return &cur
}
