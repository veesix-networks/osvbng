// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package subscriber

import (
	"context"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
)

func newProgrammedTestComponent() *Component {
	c := &Component{
		Base:             component.NewBase("subscriber"),
		logger:           logger.Get(logger.Subscriber),
		cache:            newMemCache(),
		sessionByIfIndex: make(map[uint32]string),
		ifIndexBySession: make(map[string]uint32),
	}
	c.StartContext(context.Background())
	return c
}

// PPPoE publishes its Active lifecycle event before the async VPP add, so
// the cache first holds the punt interface as IfIndex and no AccessIfIndex.
// The programmed event must repair both without losing any other cached
// field, and the ifindex map must follow.
func TestHandleSessionProgrammed_RepairsInterfaceIndexes(t *testing.T) {
	c := newProgrammedTestComponent()

	stale := &models.PPPSession{
		SessionID:  "07d9b127-1048-4ed2-ab79-065d70efd72d",
		State:      models.SessionStateActive,
		AccessType: string(models.AccessTypePPPoE),
		Protocol:   string(models.ProtocolPPPoESession),
		IfIndex:    10, // punt interface at lifecycle time
		Username:   "user@example",
	}
	if err := c.persistSession(stale); err != nil {
		t.Fatal(err)
	}

	c.handleSessionProgrammed(events.Event{Data: &events.SessionLifecycleEvent{
		SessionID: stale.SessionID,
		Session: &models.PPPSession{
			SessionID:     stale.SessionID,
			State:         models.SessionStateActive,
			AccessType:    string(models.AccessTypePPPoE),
			IfIndex:       77, // real pppoe session interface
			AccessIfIndex: 12, // encap sub-interface
		},
	}})

	cached, ok := c.SessionSnapshot(context.Background(), stale.SessionID)
	if !ok {
		t.Fatal("session missing from cache after programmed event")
	}
	if cached.GetIfIndex() != 77 || cached.GetAccessIfIndex() != 12 {
		t.Fatalf("indexes not repaired: if=%d access=%d", cached.GetIfIndex(), cached.GetAccessIfIndex())
	}
	if cached.GetUsername() != "user@example" {
		t.Fatalf("cached fields lost on repair: username=%q", cached.GetUsername())
	}
	if id, ok := c.SessionIDByIfIndex(77); !ok || id != stale.SessionID {
		t.Fatalf("ifindex map not repointed: %q %v", id, ok)
	}
	if _, ok := c.SessionIDByIfIndex(10); ok {
		t.Fatal("stale punt-interface mapping survived")
	}
}

// A partial programmed payload (IPoE publishes a stripped model) must never
// replace the cached session - only the interface indexes move.
func TestHandleSessionProgrammed_MatchingIndexesOnlyReindex(t *testing.T) {
	c := newProgrammedTestComponent()

	full := &models.IPoESession{
		SessionID:     "ipoe:52:54:00:aa:bb:01:100",
		State:         models.SessionStateActive,
		AccessType:    string(models.AccessTypeIPoE),
		IfIndex:       21,
		AccessIfIndex: 5,
		Hostname:      "cpe-1",
	}
	if err := c.persistSession(full); err != nil {
		t.Fatal(err)
	}

	c.handleSessionProgrammed(events.Event{Data: &events.SessionLifecycleEvent{
		SessionID: full.SessionID,
		Session: &models.IPoESession{
			SessionID: full.SessionID,
			IfIndex:   21,
		},
	}})

	cached, _ := c.SessionSnapshot(context.Background(), full.SessionID)
	ipoe, ok := cached.(*models.IPoESession)
	if !ok || ipoe.Hostname != "cpe-1" || ipoe.AccessIfIndex != 5 {
		t.Fatalf("cached session damaged by partial programmed payload: %+v", cached)
	}
	if id, ok := c.SessionIDByIfIndex(21); !ok || id != full.SessionID {
		t.Fatal("ifindex map not populated on matching programmed event")
	}
}
