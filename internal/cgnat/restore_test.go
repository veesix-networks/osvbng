// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/cgnat"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/opdb"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

type fakeOpDB struct {
	mu sync.Mutex
	ns map[string]map[string][]byte
}

func newFakeOpDB() *fakeOpDB { return &fakeOpDB{ns: map[string]map[string][]byte{}} }

func (f *fakeOpDB) Put(ctx context.Context, namespace, key string, value []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ns[namespace] == nil {
		f.ns[namespace] = map[string][]byte{}
	}
	f.ns[namespace][key] = append([]byte(nil), value...)
	return nil
}
func (f *fakeOpDB) Delete(ctx context.Context, namespace, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ns[namespace] != nil {
		delete(f.ns[namespace], key)
	}
	return nil
}
func (f *fakeOpDB) Load(ctx context.Context, namespace string, fn opdb.LoadFunc) error {
	f.mu.Lock()
	keys := make([]string, 0, len(f.ns[namespace]))
	vals := make([][]byte, 0, len(f.ns[namespace]))
	for k, v := range f.ns[namespace] {
		keys = append(keys, k)
		vals = append(vals, v)
	}
	f.mu.Unlock()
	for i, k := range keys {
		if err := fn(k, vals[i]); err != nil {
			return err
		}
	}
	return nil
}
func (f *fakeOpDB) Count(ctx context.Context, namespace string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.ns[namespace]), nil
}
func (f *fakeOpDB) Clear(ctx context.Context, namespace string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.ns, namespace)
	return nil
}
func (f *fakeOpDB) Stats() opdb.Stats { return opdb.Stats{} }
func (f *fakeOpDB) Close() error      { return nil }

type bulkCall struct {
	poolID   uint32
	mappings []southbound.CGNATMapping
}

type fakeDP struct {
	stubDP
	bulkCalls []bulkCall
	bulkErr   func(poolID uint32, mappings []southbound.CGNATMapping) []error
}

func (f *fakeDP) CGNATAddSubscriberMappingBulk(poolID uint32, mappings []southbound.CGNATMapping) ([]error, error) {
	f.bulkCalls = append(f.bulkCalls, bulkCall{poolID: poolID, mappings: append([]southbound.CGNATMapping(nil), mappings...)})
	if f.bulkErr != nil {
		return f.bulkErr(poolID, mappings), nil
	}
	return make([]error, len(mappings)), nil
}

type fakeProvider struct {
	sessions map[string]models.SubscriberSession
}

func (p *fakeProvider) SessionSnapshot(_ context.Context, sessionID string) (models.SubscriberSession, bool) {
	s, ok := p.sessions[sessionID]
	return s, ok
}
func (p *fakeProvider) GetSessions(_ context.Context, _, _ string, _ uint32) ([]models.SubscriberSession, error) {
	out := make([]models.SubscriberSession, 0, len(p.sessions))
	for _, s := range p.sessions {
		out = append(out, s)
	}
	return out, nil
}

type fakeCfg struct{ cfg *config.Config }

func (f *fakeCfg) GetRunning() (*config.Config, error) { return f.cfg, nil }
func (f *fakeCfg) GetStartup() (*config.Config, error) { return f.cfg, nil }

func newRestoreComponent(t *testing.T, dp *fakeDP, opdbStore *fakeOpDB, sp SessionProvider, cfg *config.Config) *Component {
	t.Helper()
	pm := NewPoolManager()
	for name, p := range cfg.CGNAT.Pools {
		if err := pm.ConfigurePool(name, 1, p); err != nil {
			t.Fatalf("configure pool %s: %v", name, err)
		}
	}
	return &Component{
		Base:            component.NewBase("cgnat"),
		logger:          logger.Get("cgnat-test"),
		dataplane:       dp,
		opdb:            opdbStore,
		cfgMgr:          &fakeCfg{cfg: cfg},
		pools:           pm,
		reverse:         NewReverseIndex(),
		bypass:          NewBypassManager(),
		blacklist:       NewBlacklistManager(),
		poolIDMap:       map[string]uint32{"p1": 1},
		sessionPoolMap:  map[string]string{},
		sessionProvider: sp,
		activations:     map[string]struct{}{},
	}
}

func pbaConfig() *config.Config {
	return &config.Config{
		CGNAT: &cgnat.Config{
			Pools: map[string]*cgnat.Pool{
				"p1": {
					Mode:                   "pba",
					BlockSize:              64,
					MaxBlocksPerSubscriber: 4,
					PortRange:              "1024-65535",
					AddressPooling:         "paired",
					OutsideAddresses:       []string{"100.64.0.0/30"},
				},
			},
		},
	}
}

func putMapping(t *testing.T, store *fakeOpDB, m *models.CGNATMapping) {
	t.Helper()
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := store.Put(context.Background(), "cgnat_mappings", m.SessionID, data); err != nil {
		t.Fatalf("put: %v", err)
	}
}

func TestRestore_PresentSession_ReprogramsWithFreshSwIfIndex(t *testing.T) {
	store := newFakeOpDB()
	mapping := &models.CGNATMapping{
		SessionID:      "s1",
		PoolName:       "p1",
		PoolID:         1,
		InsideIP:       net.ParseIP("10.0.0.1").To4(),
		OutsideIP:      net.ParseIP("100.64.0.1").To4(),
		PortBlockStart: 1024,
		PortBlockEnd:   1087,
		SwIfIndex:      55, // stale
	}
	putMapping(t, store, mapping)

	sp := &fakeProvider{sessions: map[string]models.SubscriberSession{
		"s1": &models.IPoESession{SessionID: "s1", AccessType: string(models.AccessTypeIPoE), IfIndex: 99, IPv4Address: net.ParseIP("10.0.0.1").To4()},
	}}
	dp := &fakeDP{}
	c := newRestoreComponent(t, dp, store, sp, pbaConfig())

	if err := c.restoreFromOpDB(context.Background()); err != nil {
		t.Fatalf("restore: %v", err)
	}

	if len(dp.bulkCalls) != 1 {
		t.Fatalf("expected 1 bulk call, got %d", len(dp.bulkCalls))
	}
	if dp.bulkCalls[0].mappings[0].SwIfIndex != 99 {
		t.Fatalf("expected fresh sw_if_index 99 in bulk, got %d", dp.bulkCalls[0].mappings[0].SwIfIndex)
	}
	if dp.bulkCalls[0].mappings[0].PortBlockStart != 1024 || !dp.bulkCalls[0].mappings[0].OutsideIP.Equal(net.ParseIP("100.64.0.1").To4()) {
		t.Fatalf("outside binding changed during restore: %+v", dp.bulkCalls[0].mappings[0])
	}
	if got := c.sessionPoolMap["s1"]; got != "p1" {
		t.Fatalf("sessionPoolMap[s1] = %q, want p1", got)
	}
	if c.RestoreDegraded() {
		t.Fatalf("expected RestoreDegraded=false, got true")
	}
}

func TestRestore_AbsentSession_NoAccessOpdb_DeletesOrphan(t *testing.T) {
	store := newFakeOpDB()
	mapping := &models.CGNATMapping{
		SessionID:      "expired-1",
		PoolName:       "p1",
		PoolID:         1,
		InsideIP:       net.ParseIP("10.0.0.2").To4(),
		OutsideIP:      net.ParseIP("100.64.0.1").To4(),
		PortBlockStart: 1088,
		PortBlockEnd:   1151,
	}
	putMapping(t, store, mapping)

	sp := &fakeProvider{sessions: map[string]models.SubscriberSession{}}
	dp := &fakeDP{}
	c := newRestoreComponent(t, dp, store, sp, pbaConfig())

	if err := c.restoreFromOpDB(context.Background()); err != nil {
		t.Fatalf("restore: %v", err)
	}

	count, _ := store.Count(context.Background(), "cgnat_mappings")
	if count != 0 {
		t.Fatalf("expected orphan cgnat_mappings entry deleted, %d remain", count)
	}
	if len(dp.bulkCalls) != 0 {
		t.Fatalf("expected no VPP programming for orphan, got %d bulk calls", len(dp.bulkCalls))
	}
	if c.RestoreDegraded() {
		t.Fatalf("orphan cleanup should NOT mark degraded")
	}
}

func TestRestore_AbsentSession_AccessOpdbRetained_PreservesMapping(t *testing.T) {
	store := newFakeOpDB()
	mapping := &models.CGNATMapping{
		SessionID:      "pending-1",
		PoolName:       "p1",
		PoolID:         1,
		InsideIP:       net.ParseIP("10.0.0.3").To4(),
		OutsideIP:      net.ParseIP("100.64.0.1").To4(),
		PortBlockStart: 1152,
		PortBlockEnd:   1215,
	}
	putMapping(t, store, mapping)
	store.Put(context.Background(), opdb.NamespaceIPoESessions, "pending-1", []byte("{}"))

	sp := &fakeProvider{sessions: map[string]models.SubscriberSession{}}
	dp := &fakeDP{}
	c := newRestoreComponent(t, dp, store, sp, pbaConfig())

	if err := c.restoreFromOpDB(context.Background()); err != nil {
		t.Fatalf("restore: %v", err)
	}

	count, _ := store.Count(context.Background(), "cgnat_mappings")
	if count != 1 {
		t.Fatalf("retained-checkpoint mapping must NOT be deleted; got %d entries", count)
	}
	if len(dp.bulkCalls) != 0 {
		t.Fatalf("should not program VPP when session cache missing; got %d bulk calls", len(dp.bulkCalls))
	}
	if !c.RestoreDegraded() {
		t.Fatalf("expected RestoreDegraded=true after retained-checkpoint preservation")
	}
}

func TestRestore_PartialVPPFailure_PreservesEntryAndMarksDegraded(t *testing.T) {
	store := newFakeOpDB()
	m1 := &models.CGNATMapping{SessionID: "s1", PoolName: "p1", PoolID: 1, InsideIP: net.ParseIP("10.0.0.1").To4(), OutsideIP: net.ParseIP("100.64.0.1").To4(), PortBlockStart: 1024, PortBlockEnd: 1087}
	m2 := &models.CGNATMapping{SessionID: "s2", PoolName: "p1", PoolID: 1, InsideIP: net.ParseIP("10.0.0.2").To4(), OutsideIP: net.ParseIP("100.64.0.2").To4(), PortBlockStart: 1024, PortBlockEnd: 1087}
	putMapping(t, store, m1)
	putMapping(t, store, m2)

	sp := &fakeProvider{sessions: map[string]models.SubscriberSession{
		"s1": &models.IPoESession{SessionID: "s1", AccessType: string(models.AccessTypeIPoE), IfIndex: 11, IPv4Address: m1.InsideIP},
		"s2": &models.IPoESession{SessionID: "s2", AccessType: string(models.AccessTypeIPoE), IfIndex: 12, IPv4Address: m2.InsideIP},
	}}

	dp := &fakeDP{
		bulkErr: func(poolID uint32, mappings []southbound.CGNATMapping) []error {
			res := make([]error, len(mappings))
			for i, m := range mappings {
				if m.SwIfIndex == 12 {
					res[i] = &netError{"injected"}
				}
			}
			return res
		},
	}
	c := newRestoreComponent(t, dp, store, sp, pbaConfig())

	if err := c.restoreFromOpDB(context.Background()); err != nil {
		t.Fatalf("restore: %v", err)
	}

	if _, ok := c.sessionPoolMap["s1"]; !ok {
		t.Fatalf("s1 should be committed locally")
	}
	if _, ok := c.sessionPoolMap["s2"]; ok {
		t.Fatalf("s2 must NOT be committed locally on VPP failure")
	}
	if !c.RestoreDegraded() {
		t.Fatalf("expected RestoreDegraded=true after partial VPP failure")
	}
	count, _ := store.Count(context.Background(), "cgnat_mappings")
	if count != 2 {
		t.Fatalf("failed-mapping opdb entry must be preserved; got %d entries", count)
	}
}

func TestRestore_DeterministicViaNonPBAScan_EnablesOnSession(t *testing.T) {
	store := newFakeOpDB()
	cfg := &config.Config{
		CGNAT: &cgnat.Config{
			Pools: map[string]*cgnat.Pool{
				"p1": {
					Mode:             "deterministic",
					BlockSize:        64,
					PortRange:        "1024-65535",
					OutsideAddresses: []string{"100.64.0.0/30"},
					InsidePrefixes:   []cgnat.InsidePrefix{{Prefix: "10.0.0.0/24"}},
				},
			},
		},
	}
	sp := &fakeProvider{sessions: map[string]models.SubscriberSession{
		"det-1": &models.IPoESession{SessionID: "det-1", AccessType: string(models.AccessTypeIPoE), IfIndex: 7, IPv4Address: net.ParseIP("10.0.0.42").To4(), VRF: ""},
	}}
	dp := &fakeDP{}
	c := newRestoreComponent(t, dp, store, sp, cfg)

	if err := c.restoreFromOpDB(context.Background()); err != nil {
		t.Fatalf("restore: %v", err)
	}

	if len(dp.bulkCalls) != 0 {
		t.Fatalf("deterministic recovery should not use bulk PBA binapi; got %d calls", len(dp.bulkCalls))
	}
}

type netError struct{ msg string }

func (e *netError) Error() string { return e.msg }
