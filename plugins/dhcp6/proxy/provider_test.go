// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package proxy

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
)

func TestGetDUID_UsesCorrectMAC(t *testing.T) {
	p := &Provider{duidMap: make(map[string][]byte)}
	mac := net.HardwareAddr{0x02, 0xab, 0xcd, 0x00, 0x00, 0x01}

	duid := p.getDUID(mac)
	if duid == nil {
		t.Fatal("getDUID returned nil")
	}
	if len(duid) != 14 {
		t.Fatalf("expected DUID length 14, got %d", len(duid))
	}

	duidType := binary.BigEndian.Uint16(duid[0:2])
	if duidType != 1 {
		t.Errorf("expected DUID-LLT type 1, got %d", duidType)
	}

	hwType := binary.BigEndian.Uint16(duid[2:4])
	if hwType != 1 {
		t.Errorf("expected hardware type 1 (Ethernet), got %d", hwType)
	}

	timestamp := binary.BigEndian.Uint32(duid[4:8])
	if timestamp == 0 {
		t.Error("expected non-zero DUID-LLT timestamp")
	}

	if !bytes.Equal(duid[8:14], mac) {
		t.Errorf("expected MAC %s in DUID, got %s", mac, net.HardwareAddr(duid[8:14]))
	}
}

func TestGetDUID_CachesByMAC(t *testing.T) {
	p := &Provider{duidMap: make(map[string][]byte)}
	mac1 := net.HardwareAddr{0x02, 0xab, 0xcd, 0x00, 0x00, 0x01}
	mac2 := net.HardwareAddr{0x02, 0xab, 0xcd, 0x00, 0x00, 0x02}

	duid1a := p.getDUID(mac1)
	duid1b := p.getDUID(mac1)
	duid2 := p.getDUID(mac2)

	if &duid1a[0] != &duid1b[0] {
		t.Error("expected same DUID slice for same MAC (cache hit)")
	}

	if bytes.Equal(duid2[8:14], mac1) {
		t.Error("expected different DUID for different MAC")
	}
	if !bytes.Equal(duid2[8:14], mac2) {
		t.Errorf("expected MAC %s in DUID, got %s", mac2, net.HardwareAddr(duid2[8:14]))
	}
}

func TestGetDUID_NilMAC(t *testing.T) {
	p := &Provider{duidMap: make(map[string][]byte)}

	duid := p.getDUID(nil)
	if duid != nil {
		t.Error("expected nil DUID for nil MAC")
	}
}

func TestGetDUID_ShortMAC(t *testing.T) {
	p := &Provider{duidMap: make(map[string][]byte)}

	duid := p.getDUID(net.HardwareAddr{0x01, 0x02})
	if duid != nil {
		t.Error("expected nil DUID for short MAC")
	}
}
