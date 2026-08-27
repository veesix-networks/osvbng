// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ifmgr

import (
	"net"
	"testing"
)

func TestAddressIndex_SameAddressOncePerTable(t *testing.T) {
	m := New()
	m.Add(&Interface{SwIfIndex: 1, Name: "loop101", FIBTableID: 100})
	m.Add(&Interface{SwIfIndex: 2, Name: "loop102", FIBTableID: 101})
	gw := net.ParseIP("100.64.0.1")
	m.AddIPv4Address(1, gw)
	m.AddIPv4Address(2, gw)

	if !m.HasIPv4(gw) {
		t.Fatal("gateway must be known")
	}
	if !m.HasIPv4InFIB(gw, 100) || !m.HasIPv4InFIB(gw, 101) {
		t.Fatal("gateway must be found in both customer tables")
	}
	if m.HasIPv4InFIB(gw, 0) {
		t.Fatal("gateway is not in the default table")
	}

	m.RemoveIPv4Address(1, gw)
	if m.HasIPv4InFIB(gw, 100) || !m.HasIPv4InFIB(gw, 101) {
		t.Fatal("removing one interface's address must leave the other's")
	}
	m.Remove(2)
	if m.HasIPv4(gw) {
		t.Fatal("no interface carries the address any more")
	}
}

func TestAddressIndex_FollowsTableRebind(t *testing.T) {
	m := New()
	m.Add(&Interface{SwIfIndex: 1, Name: "loop101"})
	gw := net.ParseIP("100.64.0.1")
	m.AddIPv4Address(1, gw)
	if !m.HasIPv4InFIB(gw, 0) {
		t.Fatal("address starts in the default table")
	}
	m.SetFIBTableID(1, 100)
	if m.HasIPv4InFIB(gw, 0) || !m.HasIPv4InFIB(gw, 100) {
		t.Fatal("index must follow the interface into its table")
	}
}
