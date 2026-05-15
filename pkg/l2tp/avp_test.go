// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"bytes"
	"testing"
)

func TestAVPRoundTrip(t *testing.T) {
	value := []byte{0x01, 0x02, 0x03, 0x04}
	buf := AppendAVP(nil, true, false, 0, 42, value)
	avps, err := ParseAVPs(buf)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(avps) != 1 {
		t.Fatalf("want 1 avp, got %d", len(avps))
	}
	a := avps[0]
	if !a.Mandatory || a.Hidden || a.VendorID != 0 || a.Type != 42 {
		t.Fatalf("flags/type wrong: %+v", a)
	}
	if !bytes.Equal(a.Value, value) {
		t.Fatalf("value mismatch")
	}
}

func TestAVPMultipleSequential(t *testing.T) {
	buf := AppendAVP(nil, true, false, 0, 0, []byte{0, 1})
	buf = AppendAVP(buf, false, false, 0, 7, []byte("host"))
	buf = AppendAVP(buf, true, false, 0, 9, []byte{0x12, 0x34})

	avps, err := ParseAVPs(buf)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(avps) != 3 {
		t.Fatalf("want 3 avps, got %d", len(avps))
	}
	if avps[1].Type != 7 || string(avps[1].Value) != "host" {
		t.Fatalf("middle avp wrong: %+v", avps[1])
	}
}

func TestAVPReservedBitsRejected(t *testing.T) {
	bad := []byte{0x04, 0x06, 0x00, 0x00, 0x00, 0x00} // reserved bit 10 set
	if _, err := ParseAVPs(bad); err != ErrAVPReserved {
		t.Fatalf("want ErrAVPReserved, got %v", err)
	}
}

func TestAVPHiddenWithoutRVRejected(t *testing.T) {
	// AVP with H=1 but no preceding Random Vector AVP.
	buf := AppendAVP(nil, true, true, 0, 7, []byte("oops"))
	if _, err := ParseAVPs(buf); err != ErrAVPHiddenNoRV {
		t.Fatalf("want ErrAVPHiddenNoRV, got %v", err)
	}
}

func TestAVPHiddenAfterRVAccepted(t *testing.T) {
	buf := AppendAVP(nil, false, false, 0, AVPRandomVector, bytes.Repeat([]byte{0xAA}, 16))
	buf = AppendAVP(buf, true, true, 0, 7, []byte("ciphertext-here"))
	if _, err := ParseAVPs(buf); err != nil {
		t.Fatalf("hidden after RV should be accepted: %v", err)
	}
}

func TestAVPLengthTooBig(t *testing.T) {
	// Build an AVP claiming length 200 but only provide 6 bytes total.
	buf := []byte{0x80, 0xC8, 0x00, 0x00, 0x00, 0x00}
	if _, err := ParseAVPs(buf); err != ErrAVPLengthTooBig {
		t.Fatalf("want ErrAVPLengthTooBig, got %v", err)
	}
}

func TestFindFirst(t *testing.T) {
	buf := AppendAVP(nil, true, false, 0, 0, []byte{0, 1})
	buf = AppendAVP(buf, true, false, 0, 9, []byte{0x42, 0x42})
	avps, _ := ParseAVPs(buf)

	if FindFirst(avps, 0, 9) == nil {
		t.Fatal("expected to find AVP type 9")
	}
	if FindFirst(avps, 0, 99) != nil {
		t.Fatal("expected not to find AVP type 99")
	}
}
