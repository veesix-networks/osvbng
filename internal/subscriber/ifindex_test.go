// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package subscriber

import (
	"net"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/models"
)

func testComponent() *Component {
	return &Component{
		sessionByIfIndex: make(map[uint32]string),
		ifIndexBySession: make(map[string]uint32),
	}
}

func testSession(id string, ifIndex uint32) *models.IPoESession {
	return &models.IPoESession{
		SessionID: id,
		IfIndex:   ifIndex,
		MAC:       net.HardwareAddr{0x52, 0x54, 0, 0, 0, 1},
	}
}

func TestSessionIfIndexRoundTrip(t *testing.T) {
	c := testComponent()

	c.indexSessionIfIndex(testSession("ipoe:a:100", 7))
	if id, ok := c.SessionIDByIfIndex(7); !ok || id != "ipoe:a:100" {
		t.Fatalf("expected ipoe:a:100 at 7, got %q %v", id, ok)
	}

	c.unindexSessionIfIndex("ipoe:a:100")
	if _, ok := c.SessionIDByIfIndex(7); ok {
		t.Fatal("expected 7 to be unindexed")
	}
	if len(c.sessionByIfIndex) != 0 || len(c.ifIndexBySession) != 0 {
		t.Fatal("expected both maps empty after unindex")
	}
}

// A session that reconnects and lands on a new interface must not leave its
// old sw_if_index resolving to it.
func TestSessionIfIndexReindexMovesEntry(t *testing.T) {
	c := testComponent()

	c.indexSessionIfIndex(testSession("ipoe:a:100", 7))
	c.indexSessionIfIndex(testSession("ipoe:a:100", 9))

	if _, ok := c.SessionIDByIfIndex(7); ok {
		t.Fatal("stale index at old sw_if_index 7")
	}
	if id, ok := c.SessionIDByIfIndex(9); !ok || id != "ipoe:a:100" {
		t.Fatalf("expected ipoe:a:100 at 9, got %q %v", id, ok)
	}
}

func TestSessionIfIndexIgnoresZeroAndEmpty(t *testing.T) {
	c := testComponent()

	c.indexSessionIfIndex(testSession("ipoe:a:100", 0))
	c.indexSessionIfIndex(testSession("", 7))

	if len(c.sessionByIfIndex) != 0 {
		t.Fatalf("expected nothing indexed, got %v", c.sessionByIfIndex)
	}
	c.unindexSessionIfIndex("never-indexed")
}
