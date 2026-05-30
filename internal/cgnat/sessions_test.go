// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"net"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func TestDumpSessionsMapsAndPaginates(t *testing.T) {
	dp := &stubDP{
		sessionCount: 5,
		sessions: []southbound.CGNATSession{
			{SessionIndex: 10, PoolID: 42, InsideIP: net.IPv4(100, 64, 0, 2), OutsideIP: net.IPv4(203, 0, 113, 5), RemoteIP: net.IPv4(8, 8, 8, 8), InsidePort: 34074, OutsidePort: 2185, RemotePort: 443, Proto: 6, TotalPackets: 12, TotalBytes: 3400, Age: 1.5, Timeout: 7200},
			{SessionIndex: 11, PoolID: 42, InsideIP: net.IPv4(100, 64, 0, 3), OutsideIP: net.IPv4(203, 0, 113, 5), RemoteIP: net.IPv4(1, 1, 1, 1), InsidePort: 5060, OutsidePort: 2200, RemotePort: 53, Proto: 17, Timeout: 300},
		},
	}
	c := &Component{
		logger:    logger.Get("cgnat-test"),
		dataplane: dp,
		poolIDMap: map[string]uint32{"residential": 42},
	}

	page, err := c.DumpSessions(models.CGNATSessionFilter{Limit: 2, Cursor: 7})
	if err != nil {
		t.Fatalf("DumpSessions: %v", err)
	}

	if dp.lastSessionFilter.StartIndex != 7 || dp.lastSessionFilter.Limit != 2 {
		t.Fatalf("filter not threaded to dataplane: got start=%d limit=%d", dp.lastSessionFilter.StartIndex, dp.lastSessionFilter.Limit)
	}
	if page.Total != 5 {
		t.Errorf("Total = %d, want 5", page.Total)
	}
	if page.Returned != 2 {
		t.Errorf("Returned = %d, want 2", page.Returned)
	}
	if !page.HasMore {
		t.Error("HasMore = false, want true (returned == limit)")
	}
	if page.NextCursor != 12 {
		t.Errorf("NextCursor = %d, want 12 (last index 11 + 1)", page.NextCursor)
	}
	if page.Sessions[0].PoolName != "residential" || page.Sessions[0].PoolID != 42 {
		t.Errorf("pool resolution: got name=%q id=%d", page.Sessions[0].PoolName, page.Sessions[0].PoolID)
	}
	if page.Sessions[0].Proto != "tcp" || page.Sessions[1].Proto != "udp" {
		t.Errorf("proto strings: got %q, %q", page.Sessions[0].Proto, page.Sessions[1].Proto)
	}
	if page.Sessions[0].InsidePort != 34074 || page.Sessions[0].OutsidePort != 2185 {
		t.Errorf("ports: got inside=%d outside=%d (host-order expected)", page.Sessions[0].InsidePort, page.Sessions[0].OutsidePort)
	}
}

func TestDumpSessionsNoMoreWhenUnderLimit(t *testing.T) {
	dp := &stubDP{
		sessions: []southbound.CGNATSession{{SessionIndex: 1, PoolID: 1, Proto: 6}},
	}
	c := &Component{logger: logger.Get("cgnat-test"), dataplane: dp, poolIDMap: map[string]uint32{}}

	page, err := c.DumpSessions(models.CGNATSessionFilter{Limit: 100})
	if err != nil {
		t.Fatalf("DumpSessions: %v", err)
	}
	if page.HasMore {
		t.Error("HasMore = true, want false (returned < limit)")
	}
	if page.NextCursor != 0 {
		t.Errorf("NextCursor = %d, want 0", page.NextCursor)
	}
}

func TestDumpSessionsNilSafe(t *testing.T) {
	var c *Component
	page, err := c.DumpSessions(models.CGNATSessionFilter{})
	if err != nil {
		t.Fatalf("DumpSessions on nil component: %v", err)
	}
	if page.Sessions == nil || len(page.Sessions) != 0 {
		t.Errorf("expected empty non-nil sessions, got %v", page.Sessions)
	}
}
