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
