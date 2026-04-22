// Copyright 2025 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"testing"

	hapb "github.com/veesix-networks/osvbng/api/proto/ha"
	"github.com/veesix-networks/osvbng/pkg/allocator"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/opdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type memStore struct {
	data map[string]map[string][]byte
	mu   sync.Mutex
}

func newMemStore() *memStore {
	return &memStore{data: make(map[string]map[string][]byte)}
}

func (m *memStore) Put(_ context.Context, ns, key string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data[ns] == nil {
		m.data[ns] = make(map[string][]byte)
	}
	m.data[ns][key] = value
	return nil
}

func (m *memStore) Delete(_ context.Context, ns, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data[ns], key)
	return nil
}

func (m *memStore) Load(_ context.Context, ns string, fn opdb.LoadFunc) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, v := range m.data[ns] {
		if err := fn(k, v); err != nil {
			return err
		}
	}
	return nil
}

func (m *memStore) Count(_ context.Context, ns string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.data[ns]), nil
}

func (m *memStore) Clear(_ context.Context, ns string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, ns)
	return nil
}

func (m *memStore) Stats() opdb.Stats { return opdb.Stats{} }
func (m *memStore) Close() error      { return nil }

func (m *memStore) has(ns, key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.data[ns][key]
	return ok
}

func newTestRegistry() *allocator.Registry {
	return allocator.NewTestRegistry(
		netip.MustParseAddr("10.0.0.1"),
		netip.MustParseAddr("10.0.0.254"),
	)
}

func TestSyncReceiver_CreateStoresAndReserves(t *testing.T) {
	store := newMemStore()
	reg := newTestRegistry()
	recv := NewSyncReceiver(store, reg, logger.NewTest())
	ctx := context.Background()

	req := &hapb.SyncSessionRequest{
		SrgName:  "srg1",
		Sequence: 1,
		Action:   hapb.SyncAction_SYNC_ACTION_CREATE,
		Session: &hapb.SessionCheckpoint{
			SessionId:   "sess-1",
			SrgName:     "srg1",
			AccessType:  "ipoe",
			Ipv4Address: net.ParseIP("10.0.0.5").To4(),
		},
	}

	resp, err := recv.HandleSyncSession(ctx, req)
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Equal(t, uint64(1), resp.LastSyncSeq)

	assert.True(t, store.has(opdb.NamespaceHASyncedIPoE, "sess-1"))

	err = reg.ReserveIP(net.ParseIP("10.0.0.5"), "other")
	assert.Error(t, err, "IP should already be reserved")
}

func TestSyncReceiver_DeleteReleasesIP(t *testing.T) {
	store := newMemStore()
	reg := newTestRegistry()
	recv := NewSyncReceiver(store, reg, logger.NewTest())
	ctx := context.Background()

	createReq := &hapb.SyncSessionRequest{
		SrgName:  "srg1",
		Sequence: 1,
		Action:   hapb.SyncAction_SYNC_ACTION_CREATE,
		Session: &hapb.SessionCheckpoint{
			SessionId:   "sess-1",
			AccessType:  "ipoe",
			Ipv4Address: net.ParseIP("10.0.0.5").To4(),
		},
	}
	recv.HandleSyncSession(ctx, createReq)

	deleteReq := &hapb.SyncSessionRequest{
		SrgName:  "srg1",
		Sequence: 2,
		Action:   hapb.SyncAction_SYNC_ACTION_DELETE,
		Session: &hapb.SessionCheckpoint{
			SessionId:   "sess-1",
			AccessType:  "ipoe",
			Ipv4Address: net.ParseIP("10.0.0.5").To4(),
		},
	}

	resp, err := recv.HandleSyncSession(ctx, deleteReq)
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Equal(t, uint64(2), resp.LastSyncSeq)

	assert.False(t, store.has(opdb.NamespaceHASyncedIPoE, "sess-1"))

	err = reg.ReserveIP(net.ParseIP("10.0.0.5"), "new-session")
	assert.NoError(t, err, "IP should be free after delete")
}

func TestSyncReceiver_PPPoENamespace(t *testing.T) {
	store := newMemStore()
	recv := NewSyncReceiver(store, nil, logger.NewTest())
	ctx := context.Background()

	req := &hapb.SyncSessionRequest{
		SrgName:  "srg1",
		Sequence: 1,
		Action:   hapb.SyncAction_SYNC_ACTION_CREATE,
		Session: &hapb.SessionCheckpoint{
			SessionId:  "ppp-1",
			AccessType: "pppoe",
		},
	}

	resp, err := recv.HandleSyncSession(ctx, req)
	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.True(t, store.has(opdb.NamespaceHASyncedPPPoE, "ppp-1"))
	assert.False(t, store.has(opdb.NamespaceHASyncedIPoE, "ppp-1"))
}

func TestSyncReceiver_SequenceTracking(t *testing.T) {
	store := newMemStore()
	recv := NewSyncReceiver(store, nil, logger.NewTest())
	ctx := context.Background()

	for seq := uint64(1); seq <= 5; seq++ {
		req := &hapb.SyncSessionRequest{
			SrgName:  "srg1",
			Sequence: seq,
			Action:   hapb.SyncAction_SYNC_ACTION_CREATE,
			Session: &hapb.SessionCheckpoint{
				SessionId:  "sess",
				AccessType: "ipoe",
			},
		}
		recv.HandleSyncSession(ctx, req)
	}

	assert.Equal(t, uint64(5), recv.GetLastSeq("srg1"))
	assert.Equal(t, uint64(0), recv.GetLastSeq("srg2"))
}

func TestSyncReceiver_BulkSyncPage(t *testing.T) {
	store := newMemStore()
	reg := newTestRegistry()
	recv := NewSyncReceiver(store, reg, logger.NewTest())
	ctx := context.Background()

	page := &hapb.BulkSyncResponse{
		SrgName: "srg1",
		Sessions: []*hapb.SessionCheckpoint{
			{SessionId: "s1", AccessType: "ipoe", Ipv4Address: net.ParseIP("10.0.0.1").To4()},
			{SessionId: "s2", AccessType: "ipoe", Ipv4Address: net.ParseIP("10.0.0.2").To4()},
			{SessionId: "s3", AccessType: "pppoe"},
		},
		Sequence: 100,
		LastPage: true,
	}

	err := recv.HandleBulkSyncPage(ctx, page)
	require.NoError(t, err)

	assert.True(t, store.has(opdb.NamespaceHASyncedIPoE, "s1"))
	assert.True(t, store.has(opdb.NamespaceHASyncedIPoE, "s2"))
	assert.True(t, store.has(opdb.NamespaceHASyncedPPPoE, "s3"))
	assert.Equal(t, uint64(100), recv.GetLastSeq("srg1"))
}

func TestSyncReceiver_ClearSyncedNamespace(t *testing.T) {
	store := newMemStore()
	recv := NewSyncReceiver(store, nil, logger.NewTest())
	ctx := context.Background()

	store.Put(ctx, opdb.NamespaceHASyncedIPoE, "s1", []byte("data"))
	store.Put(ctx, opdb.NamespaceHASyncedPPPoE, "p1", []byte("data"))

	err := recv.ClearSyncedNamespace(ctx, "srg1")
	require.NoError(t, err)

	assert.False(t, store.has(opdb.NamespaceHASyncedIPoE, "s1"))
	assert.False(t, store.has(opdb.NamespaceHASyncedPPPoE, "p1"))
}
