// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"encoding/binary"
	"testing"
)

func TestIsL2TPv3(t *testing.T) {
	// V2 flags word (T=1, L=1, S=1, ver=2).
	v2 := make([]byte, 2)
	binary.BigEndian.PutUint16(v2, flagT|flagL|flagS|uint16(Version2))
	if IsL2TPv3(v2) {
		t.Fatal("V2 header misidentified as V3")
	}

	v3 := make([]byte, 2)
	binary.BigEndian.PutUint16(v3, flagT|uint16(Version3))
	if !IsL2TPv3(v3) {
		t.Fatal("V3 header not detected")
	}

	if IsL2TPv3([]byte{0x00}) {
		t.Fatal("single-byte slice should return false")
	}
}

func TestBuildV3RejectStopCCN(t *testing.T) {
	out := BuildV3RejectStopCCN(0xAABB, 0xCCDD)
	if len(out) == 0 {
		t.Fatal("empty StopCCN")
	}

	h, body, err := Parse(out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !h.IsControl {
		t.Fatal("StopCCN must be a control message")
	}
	if h.TunnelID != 0xCCDD {
		t.Fatalf("tunnel id should be peer-assigned, got %x", h.TunnelID)
	}

	avps, err := ParseAVPs(body)
	if err != nil {
		t.Fatalf("parse avps: %v", err)
	}

	if got := DecodeMessageType(avps); got != MsgTypeStopCCN {
		t.Fatalf("first AVP should be MessageType=StopCCN, got %d", got)
	}

	rc := FindFirst(avps, 0, AVPResultCode)
	if rc == nil {
		t.Fatal("missing Result Code AVP")
	}
	if len(rc.Value) < 4 {
		t.Fatalf("result code AVP must carry result + error code, got %d bytes", len(rc.Value))
	}
	if got := ResultCode(binary.BigEndian.Uint16(rc.Value[:2])); got != ResultStopVersionUnsupported {
		t.Fatalf("want result %d, got %d", ResultStopVersionUnsupported, got)
	}
	if got := binary.BigEndian.Uint16(rc.Value[2:4]); got != 0x0100 {
		t.Fatalf("want error code 0x0100, got 0x%x", got)
	}

	if FindFirst(avps, 0, AVPAssignedTunnelID) == nil {
		t.Fatal("missing Assigned Tunnel ID AVP")
	}
}
