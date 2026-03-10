// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package proxy

import (
	"sync"
	"time"
)

const numShards = 16

type Binding struct {
	ServerDUID          []byte
	ServerPreferred     uint32
	ServerValid         uint32
	ClientPreferred     uint32
	ClientValid         uint32
	ServerBoundAt       int64
	LastClientRenew     int64
}

func (b *Binding) UpstreamT1Elapsed() bool {
	elapsed := time.Now().Unix() - b.ServerBoundAt
	return elapsed >= int64(b.ServerPreferred/2)
}

type bindingShard struct {
	mu       sync.RWMutex
	bindings map[string]Binding
}

type Bindings struct {
	shards [numShards]bindingShard
}

func NewBindings() *Bindings {
	b := &Bindings{}
	for i := range b.shards {
		b.shards[i].bindings = make(map[string]Binding)
	}
	return b
}

func (b *Bindings) shard(key string) *bindingShard {
	h := uint32(0)
	for i := 0; i < len(key); i++ {
		h = h*31 + uint32(key[i])
	}
	return &b.shards[h%numShards]
}

func (b *Bindings) Get(duidKey string) (Binding, bool) {
	s := b.shard(duidKey)
	s.mu.RLock()
	binding, ok := s.bindings[duidKey]
	s.mu.RUnlock()
	return binding, ok
}

func (b *Bindings) Set(duidKey string, binding Binding) {
	s := b.shard(duidKey)
	s.mu.Lock()
	s.bindings[duidKey] = binding
	s.mu.Unlock()
}

func (b *Bindings) Delete(duidKey string) {
	s := b.shard(duidKey)
	s.mu.Lock()
	delete(s.bindings, duidKey)
	s.mu.Unlock()
}

func (b *Bindings) Count() int {
	total := 0
	for i := range b.shards {
		b.shards[i].mu.RLock()
		total += len(b.shards[i].bindings)
		b.shards[i].mu.RUnlock()
	}
	return total
}

func (b *Bindings) UpdateLastRenew(duidKey string) {
	s := b.shard(duidKey)
	s.mu.Lock()
	if binding, ok := s.bindings[duidKey]; ok {
		binding.LastClientRenew = time.Now().Unix()
		s.bindings[duidKey] = binding
	}
	s.mu.Unlock()
}
