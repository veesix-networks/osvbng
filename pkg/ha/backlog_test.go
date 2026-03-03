// Copyright 2025 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"testing"

	hapb "github.com/veesix-networks/osvbng/api/proto/ha"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeReq(seq uint64) *hapb.SyncSessionRequest {
	return &hapb.SyncSessionRequest{
		SrgName:  "srg1",
		Sequence: seq,
		Action:   hapb.SyncAction_SYNC_ACTION_CREATE,
	}
}

func TestSyncBacklog_Empty(t *testing.T) {
	b := NewSyncBacklog(10)
	assert.Equal(t, uint64(0), b.OldestSeq())
	assert.Equal(t, uint64(0), b.NewestSeq())
	assert.Equal(t, 0, b.Size())
	assert.Nil(t, b.Range(1, 10))
}

func TestSyncBacklog_PushAndRange(t *testing.T) {
	b := NewSyncBacklog(10)

	for i := uint64(1); i <= 5; i++ {
		b.Push(makeReq(i))
	}

	assert.Equal(t, 5, b.Size())
	assert.Equal(t, uint64(1), b.OldestSeq())
	assert.Equal(t, uint64(5), b.NewestSeq())

	result := b.Range(2, 4)
	require.Len(t, result, 3)
	assert.Equal(t, uint64(2), result[0].Sequence)
	assert.Equal(t, uint64(3), result[1].Sequence)
	assert.Equal(t, uint64(4), result[2].Sequence)
}

func TestSyncBacklog_WrapAround(t *testing.T) {
	b := NewSyncBacklog(5)

	for i := uint64(1); i <= 8; i++ {
		b.Push(makeReq(i))
	}

	assert.Equal(t, 5, b.Size())
	assert.Equal(t, uint64(4), b.OldestSeq())
	assert.Equal(t, uint64(8), b.NewestSeq())

	result := b.Range(4, 8)
	require.Len(t, result, 5)
	assert.Equal(t, uint64(4), result[0].Sequence)
	assert.Equal(t, uint64(8), result[4].Sequence)
}

func TestSyncBacklog_RangeClampedToOldest(t *testing.T) {
	b := NewSyncBacklog(5)

	for i := uint64(1); i <= 8; i++ {
		b.Push(makeReq(i))
	}

	result := b.Range(1, 6)
	require.Len(t, result, 3)
	assert.Equal(t, uint64(4), result[0].Sequence)
	assert.Equal(t, uint64(6), result[2].Sequence)
}

func TestSyncBacklog_RangeBeyondNewest(t *testing.T) {
	b := NewSyncBacklog(10)

	for i := uint64(1); i <= 5; i++ {
		b.Push(makeReq(i))
	}

	result := b.Range(3, 100)
	require.Len(t, result, 3)
	assert.Equal(t, uint64(3), result[0].Sequence)
	assert.Equal(t, uint64(5), result[2].Sequence)
}

func TestSyncBacklog_GapDetection(t *testing.T) {
	b := NewSyncBacklog(5)

	for i := uint64(1); i <= 5; i++ {
		b.Push(makeReq(i))
	}

	oldest := b.OldestSeq()
	newest := b.NewestSeq()

	peerLastSeq := uint64(0)
	needsBulkSync := peerLastSeq == 0 || peerLastSeq+1 < oldest
	assert.True(t, needsBulkSync)

	peerLastSeq = 3
	needsBulkSync = peerLastSeq == 0 || peerLastSeq+1 < oldest
	assert.False(t, needsBulkSync)
	replay := b.Range(peerLastSeq+1, newest)
	require.Len(t, replay, 2)
	assert.Equal(t, uint64(4), replay[0].Sequence)
	assert.Equal(t, uint64(5), replay[1].Sequence)

	for i := uint64(6); i <= 15; i++ {
		b.Push(makeReq(i))
	}
	oldest = b.OldestSeq()
	peerLastSeq = 5
	needsBulkSync = peerLastSeq+1 < oldest
	assert.True(t, needsBulkSync, "peer at seq 5 but oldest in backlog is %d", oldest)
}

func TestSyncBacklog_DefaultCapacity(t *testing.T) {
	b := NewSyncBacklog(0)
	assert.Equal(t, 10000, b.capacity)

	b2 := NewSyncBacklog(-1)
	assert.Equal(t, 10000, b2.capacity)
}
