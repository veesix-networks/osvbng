// Copyright 2025 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"sync"

	hapb "github.com/veesix-networks/osvbng/api/proto/ha"
)

type SyncBacklog struct {
	entries  []*hapb.SyncSessionRequest
	head     int
	size     int
	capacity int
	mu       sync.Mutex
}

func NewSyncBacklog(capacity int) *SyncBacklog {
	if capacity <= 0 {
		capacity = 10000
	}
	return &SyncBacklog{
		entries:  make([]*hapb.SyncSessionRequest, capacity),
		capacity: capacity,
	}
}

func (b *SyncBacklog) Push(req *hapb.SyncSessionRequest) {
	b.mu.Lock()
	b.entries[b.head] = req
	b.head = (b.head + 1) % b.capacity
	if b.size < b.capacity {
		b.size++
	}
	b.mu.Unlock()
}

func (b *SyncBacklog) OldestSeq() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.size == 0 {
		return 0
	}
	idx := (b.head - b.size + b.capacity) % b.capacity
	return b.entries[idx].Sequence
}

func (b *SyncBacklog) NewestSeq() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.size == 0 {
		return 0
	}
	idx := (b.head - 1 + b.capacity) % b.capacity
	return b.entries[idx].Sequence
}

func (b *SyncBacklog) Size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.size
}

func (b *SyncBacklog) Range(fromSeq, toSeq uint64) []*hapb.SyncSessionRequest {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.size == 0 {
		return nil
	}

	oldest := (b.head - b.size + b.capacity) % b.capacity
	oldestSeq := b.entries[oldest].Sequence

	if fromSeq < oldestSeq {
		fromSeq = oldestSeq
	}

	startOffset := int(fromSeq - oldestSeq)
	count := int(toSeq-fromSeq) + 1
	if startOffset+count > b.size {
		count = b.size - startOffset
	}
	if count <= 0 {
		return nil
	}

	result := make([]*hapb.SyncSessionRequest, count)
	for i := 0; i < count; i++ {
		result[i] = b.entries[(oldest+startOffset+i)%b.capacity]
	}
	return result
}
