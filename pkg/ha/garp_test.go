// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"net"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/models"
)

type staticSessionIterator struct {
	sessions []models.SubscriberSession
}

func (s *staticSessionIterator) ForEachSession(fn func(models.SubscriberSession) bool) {
	for _, sess := range s.sessions {
		if !fn(sess) {
			return
		}
	}
}

// GARP entries must carry the access encap interface, never the
// per-session interface: session interfaces have no output node, so
// frames injected there via interface-output die on local0 (issue 417).
func TestCollectGarpEntriesUsesAccessIfIndex(t *testing.T) {
	m := newTestManager(t)

	m.RegisterSessionIterator(&staticSessionIterator{sessions: []models.SubscriberSession{
		&models.IPoESession{
			SessionID:     "s1",
			State:         models.SessionStateActive,
			SRGName:       "srg1",
			IfIndex:       17,
			AccessIfIndex: 3,
			OuterVLAN:     100,
			InnerVLAN:     7,
			IPv4Address:   net.ParseIP("100.64.0.1"),
		},
		&models.IPoESession{
			SessionID:     "s2",
			State:         models.SessionStateActive,
			SRGName:       "srg1",
			IfIndex:       18,
			AccessIfIndex: 3,
			IPv4Address:   net.ParseIP("100.64.0.2"),
			IPv6Address:   net.ParseIP("2001:db8::2"),
		},
		// Session interface known but access interface unresolved:
		// no entry, a GARP to sw_if_index 0 egresses local0.
		&models.IPoESession{
			SessionID:   "s3",
			State:       models.SessionStateActive,
			SRGName:     "srg1",
			IfIndex:     19,
			IPv4Address: net.ParseIP("100.64.0.3"),
		},
		&models.IPoESession{
			SessionID:     "other-srg",
			State:         models.SessionStateActive,
			SRGName:       "srg2",
			IfIndex:       20,
			AccessIfIndex: 4,
			IPv4Address:   net.ParseIP("100.64.0.4"),
		},
		&models.IPoESession{
			SessionID:     "not-active",
			State:         models.SessionStateRequesting,
			SRGName:       "srg1",
			IfIndex:       21,
			AccessIfIndex: 3,
			IPv4Address:   net.ParseIP("100.64.0.5"),
		},
	}})

	entries := m.collectGarpEntries("srg1")

	if len(entries) != 3 {
		t.Fatalf("expected 3 entries (s1 v4, s2 v4, s2 v6), got %d", len(entries))
	}
	for _, e := range entries {
		if e.SwIfIndex != 3 {
			t.Fatalf("entry for %s uses sw_if_index %d, want access if_index 3", e.IP, e.SwIfIndex)
		}
	}
	// Catch-all vlan-any sub-interfaces have no rewrite state, so each
	// entry must carry the session's own tags for the dataplane to build.
	if entries[0].OuterVLAN != 100 || entries[0].InnerVLAN != 7 {
		t.Fatalf("entry vlans %d/%d, want 100/7",
			entries[0].OuterVLAN, entries[0].InnerVLAN)
	}
}
