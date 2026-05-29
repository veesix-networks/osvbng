// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"context"
	"net"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/models"
)

func TestRecoverDataplane_DoesNotDoubleAppendPools(t *testing.T) {
	store := newFakeOpDB()
	mapping := &models.CGNATMapping{
		SessionID:      "s1",
		PoolName:       "p1",
		PoolID:         1,
		InsideIP:       net.ParseIP("10.0.0.1").To4(),
		OutsideIP:      net.ParseIP("100.64.0.1").To4(),
		PortBlockStart: 1024,
		PortBlockEnd:   1087,
	}
	putMapping(t, store, mapping)

	sp := &fakeProvider{sessions: map[string]models.SubscriberSession{
		"s1": &models.IPoESession{SessionID: "s1", AccessType: string(models.AccessTypeIPoE), IfIndex: 42, IPv4Address: mapping.InsideIP},
	}}
	dp := &fakeDP{}
	c := newRestoreComponent(t, dp, store, sp, pbaConfig())

	// Cold restore primes the local state (PoolManager, sessionPoolMap)
	// the same way Start() would on first boot.
	if err := c.restoreFromOpDB(context.Background()); err != nil {
		t.Fatalf("cold restore: %v", err)
	}
	if got := len(c.pools.GetMappings("p1", mapping.InsideIP, 0)); got != 1 {
		t.Fatalf("after cold restore: expected 1 local block, got %d", got)
	}

	// VPP crashes; watchdog reconnects and invokes RecoverDataplane.
	dp.bulkCalls = nil
	if err := c.RecoverDataplane(context.Background()); err != nil {
		t.Fatalf("recover dataplane: %v", err)
	}

	if got := len(c.pools.GetMappings("p1", mapping.InsideIP, 0)); got != 1 {
		t.Fatalf("RecoverDataplane double-appended local block; expected 1, got %d", got)
	}
	if len(dp.bulkCalls) != 1 {
		t.Fatalf("RecoverDataplane should reprogram every mapping; got %d bulk calls", len(dp.bulkCalls))
	}
	if dp.bulkCalls[0].mappings[0].SwIfIndex != 42 {
		t.Fatalf("RecoverDataplane should use live sw_if_index 42, got %d", dp.bulkCalls[0].mappings[0].SwIfIndex)
	}
}

func TestRecoverDataplane_AbsentSessionIsSkippedNotDeleted(t *testing.T) {
	store := newFakeOpDB()
	mapping := &models.CGNATMapping{
		SessionID:      "absent-1",
		PoolName:       "p1",
		PoolID:         1,
		InsideIP:       net.ParseIP("10.0.0.5").To4(),
		OutsideIP:      net.ParseIP("100.64.0.1").To4(),
		PortBlockStart: 1024,
		PortBlockEnd:   1087,
	}
	putMapping(t, store, mapping)

	sp := &fakeProvider{sessions: map[string]models.SubscriberSession{}}
	dp := &fakeDP{}
	c := newRestoreComponent(t, dp, store, sp, pbaConfig())

	if err := c.RecoverDataplane(context.Background()); err != nil {
		t.Fatalf("recover: %v", err)
	}

	count, _ := store.Count(context.Background(), "cgnat_mappings")
	if count != 1 {
		t.Fatalf("watchdog recovery must NOT delete absent-session mappings (that's the cold path's job); got %d entries", count)
	}
	if len(dp.bulkCalls) != 0 {
		t.Fatalf("absent session must not be programmed; got %d bulk calls", len(dp.bulkCalls))
	}
}
