// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package proxy

import (
	"sync"
	"time"
)

const numShards = 16

type Binding struct {
	ClientIP        [4]byte
	ServerIP        [4]byte
	ServerLease     uint32
	ClientLease     uint32
	ServerBoundAt   int64
	LastClientRenew int64
}

func (b *Binding) UpstreamT1Elapsed() bool {
	elapsed := time.Now().Unix() - b.ServerBoundAt
	return elapsed >= int64(b.ServerLease/2)
}

type bindingShard struct {
	mu       sync.RWMutex
	bindings map[string]Binding
}

type Bindings struct {
	shards [numShards]bindingShard
	stop   chan struct{}
}

func NewBindings() *Bindings {
	b := &Bindings{
		stop: make(chan struct{}),
	}
	for i := range b.shards {
		b.shards[i].bindings = make(map[string]Binding)
	}
	go b.sweepLoop()
	return b
}

func (b *Bindings) Close() {
	close(b.stop)
}

func (b *Bindings) sweepLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			b.sweep()
		case <-b.stop:
			return
		}
	}
}

func (b *Bindings) sweep() {
	now := time.Now().Unix()
	for i := range b.shards {
		s := &b.shards[i]
		s.mu.Lock()
		for key, binding := range s.bindings {
			if binding.ServerLease == 0 {
				continue
			}
			if now-binding.ServerBoundAt > int64(binding.ServerLease*2) {
				delete(s.bindings, key)
			}
		}
		s.mu.Unlock()
	}
}

func (b *Bindings) shard(mac string) *bindingShard {
	h := uint32(0)
	for i := 0; i < len(mac); i++ {
		h = h*31 + uint32(mac[i])
	}
	return &b.shards[h%numShards]
}

func (b *Bindings) Get(mac string) (Binding, bool) {
	s := b.shard(mac)
	s.mu.RLock()
	binding, ok := s.bindings[mac]
	s.mu.RUnlock()
	return binding, ok
}

func (b *Bindings) Set(mac string, binding Binding) {
	s := b.shard(mac)
	s.mu.Lock()
	s.bindings[mac] = binding
	s.mu.Unlock()
}

func (b *Bindings) Delete(mac string) {
	s := b.shard(mac)
	s.mu.Lock()
	delete(s.bindings, mac)
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

func (b *Bindings) UpdateLastRenew(mac string) {
	s := b.shard(mac)
	s.mu.Lock()
	if binding, ok := s.bindings[mac]; ok {
		binding.LastClientRenew = time.Now().Unix()
		s.bindings[mac] = binding
	}
	s.mu.Unlock()
}
