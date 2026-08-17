// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package pppoe

import (
	"net"
	"testing"
	"time"

	"github.com/google/gopacket/layers"
	"github.com/veesix-networks/osvbng/internal/ra"
)

func TestPPPoEPeriodicRAEmitsToAllNodes(t *testing.T) {
	s, bus := ndSession(t)
	s.ipv6cpOpen = true
	cfg, _ := s.component.cfgMgr.GetRunning()
	now := time.Now()

	s.component.emitPeriodicRA(s, cfg, now)
	if len(bus.egress) != 1 {
		t.Fatalf("want 1 periodic RA, got %d", len(bus.egress))
	}
	pkt := ndDecode(t, bus.egress[0].Packet.RawData)
	ip6 := pkt.Layer(layers.LayerTypeIPv6).(*layers.IPv6)
	if !ip6.DstIP.Equal(net.IPv6linklocalallnodes) {
		t.Fatalf("periodic RA dst = %s, want ff02::1", ip6.DstIP)
	}
	if !ip6.SrcIP.Equal(ra.LinkLocalFromMAC(ndParentMAC)) {
		t.Fatalf("periodic RA src = %s, want BNG link-local", ip6.SrcIP)
	}
	if s.nextRADue.IsZero() {
		t.Fatal("nextRADue must be set after emit")
	}

	// Not due yet: a second emit at the same instant sends nothing.
	s.component.emitPeriodicRA(s, cfg, now)
	if len(bus.egress) != 1 {
		t.Fatalf("RA must not re-send before nextRADue, got %d", len(bus.egress))
	}
}

func TestPPPoEPeriodicRACeasesOnGroupV6Disable(t *testing.T) {
	s, bus := ndSession(t)
	s.ipv6cpOpen = true
	s.nextRADue = time.Now().Add(-time.Second) // had been advertising

	cfg, _ := s.component.cfgMgr.GetRunning()
	cfg.SubscriberGroups.Groups["grp"].IPv6Profile = "" // disable group v6

	s.component.emitPeriodicRA(s, cfg, time.Now())
	if len(bus.egress) != 1 {
		t.Fatalf("want 1 cease RA, got %d", len(bus.egress))
	}
	pkt := ndDecode(t, bus.egress[0].Packet.RawData)
	raLayer := pkt.Layer(layers.LayerTypeICMPv6RouterAdvertisement).(*layers.ICMPv6RouterAdvertisement)
	if raLayer.RouterLifetime != 0 {
		t.Fatalf("cease RA RouterLifetime = %d, want 0", raLayer.RouterLifetime)
	}
	if !s.nextRADue.IsZero() {
		t.Fatal("nextRADue must be cleared after cease")
	}
}

func TestPPPoERABucketMaintenance(t *testing.T) {
	s, _ := ndSession(t)
	bucket := s.component.raBucketOf(s.SessionID)

	s.component.placeSessionInRABucket(s)
	s.component.placeSessionInRABucket(s) // idempotent

	s.component.raBucketMu.RLock()
	got := len(s.component.raBuckets[bucket])
	s.component.raBucketMu.RUnlock()
	if got != 1 {
		t.Fatalf("bucket should hold exactly 1 entry, got %d", got)
	}

	s.component.removeSessionFromRABucket(s)
	s.component.raBucketMu.RLock()
	got = len(s.component.raBuckets[bucket])
	s.component.raBucketMu.RUnlock()
	if got != 0 {
		t.Fatalf("bucket should be empty after remove, got %d", got)
	}
}

// The initial burst: three advertisements at MAX_INITIAL_RTR_ADVERT_INTERVAL
// spacing (RFC 4861 section 6.2.4), then the wheel's refresh cadence.
func TestPPPoEInitialRABurstSpacing(t *testing.T) {
	s, bus := ndSession(t)
	s.ipv6cpOpen = true
	s.raInitialLeft = raInitialAdverts
	cfg, _ := s.component.cfgMgr.GetRunning()
	t0 := time.Now()

	s.component.emitPeriodicRA(s, cfg, t0)
	if len(bus.egress) != 1 {
		t.Fatalf("first burst RA: want 1 egress, got %d", len(bus.egress))
	}
	if want := t0.Add(raInitialAdvertInterval); !s.nextRADue.Equal(want) {
		t.Fatalf("second burst RA due = %v, want %v", s.nextRADue, want)
	}

	// Not due inside the burst interval.
	s.component.emitPeriodicRA(s, cfg, t0.Add(time.Second))
	if len(bus.egress) != 1 {
		t.Fatalf("burst RA re-sent before its interval, got %d", len(bus.egress))
	}

	s.component.emitPeriodicRA(s, cfg, t0.Add(raInitialAdvertInterval))
	s.component.emitPeriodicRA(s, cfg, t0.Add(2*raInitialAdvertInterval))
	if len(bus.egress) != 3 {
		t.Fatalf("want 3 burst RAs, got %d", len(bus.egress))
	}
	if s.raInitialLeft != 0 {
		t.Fatalf("raInitialLeft = %d after burst, want 0", s.raInitialLeft)
	}
	// Fourth advertisement is a full refresh away, not another 16s.
	if due := s.nextRADue.Sub(t0.Add(2 * raInitialAdvertInterval)); due <= raInitialAdvertInterval {
		t.Fatalf("post-burst due %v not on refresh cadence", due)
	}
}

// walkInitialRAs drops finished, torn-down and IPv6CP-down sessions from the
// burst set and emits for the rest.
func TestPPPoEWalkInitialRAs(t *testing.T) {
	s, bus := ndSession(t)
	s.ipv6cpOpen = true
	s.raInitialLeft = raInitialAdverts
	c := s.component
	c.sessionIDIndex = map[string]*SessionState{s.SessionID: s}

	initial := map[string]struct{}{s.SessionID: {}, "gone": {}}
	c.walkInitialRAs(initial)
	if len(bus.egress) != 1 {
		t.Fatalf("want 1 RA from walk, got %d", len(bus.egress))
	}
	if _, ok := initial["gone"]; ok {
		t.Fatal("torn-down session must leave the burst set")
	}
	if _, ok := initial[s.SessionID]; !ok {
		t.Fatal("bursting session must stay in the set")
	}

	s.raInitialLeft = 0
	c.walkInitialRAs(initial)
	if _, ok := initial[s.SessionID]; ok {
		t.Fatal("finished session must leave the burst set")
	}
}
