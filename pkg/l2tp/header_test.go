// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"bytes"
	"testing"
)

func TestHeaderRoundTripData(t *testing.T) {
	h := NewData(0x1234, 0x5678)
	body := []byte("payload")
	out := h.AppendTo(nil, len(body))
	out = append(out, body...)

	got, rest, err := Parse(out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.IsControl {
		t.Fatal("expected data, got control")
	}
	if got.TunnelID != 0x1234 || got.SessionID != 0x5678 {
		t.Fatalf("ids: %v %v", got.TunnelID, got.SessionID)
	}
	if !bytes.Equal(rest, body) {
		t.Fatalf("payload mismatch: %v", rest)
	}
}

func TestHeaderRoundTripControl(t *testing.T) {
	h := NewControl(0x4242, 0, 7, 9)
	body := []byte("avp-bytes-here")
	out := h.AppendTo(nil, len(body))
	out = append(out, body...)

	got, rest, err := Parse(out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !got.IsControl || !got.HasLength || !got.HasSequence {
		t.Fatalf("flags wrong: %+v", got)
	}
	if got.Ns != 7 || got.Nr != 9 {
		t.Fatalf("seq wrong: ns=%d nr=%d", got.Ns, got.Nr)
	}
	if int(got.Length) != got.HeaderLen+len(body) {
		t.Fatalf("length wrong: %d vs header(%d)+body(%d)",
			got.Length, got.HeaderLen, len(body))
	}
	if !bytes.Equal(rest, body) {
		t.Fatalf("payload mismatch")
	}
}

func TestHeaderShort(t *testing.T) {
	if _, _, err := Parse([]byte{0x00}); err != ErrShortPacket {
		t.Fatalf("want ErrShortPacket, got %v", err)
	}
}

func TestHeaderReservedBits(t *testing.T) {
	// Flags with a reserved bit (bit 2) set.
	bad := []byte{0x20, 0x02, 0x00, 0x01, 0x00, 0x01}
	_, _, err := Parse(bad)
	if err != ErrReservedBits {
		t.Fatalf("want ErrReservedBits, got %v", err)
	}
}

func TestHeaderOffsetRoundTrip(t *testing.T) {
	h := &Header{
		IsControl:  false,
		Version:    Version2,
		HasOffset:  true,
		OffsetSize: 4,
		TunnelID:   1,
		SessionID:  2,
	}
	body := []byte("data")
	out := h.AppendTo(nil, len(body))
	out = append(out, body...)

	got, rest, err := Parse(out)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !got.HasOffset || got.OffsetSize != 4 {
		t.Fatalf("offset wrong: has=%v size=%d", got.HasOffset, got.OffsetSize)
	}
	if string(rest) != "data" {
		t.Fatalf("payload mismatch: %q", rest)
	}
}

func TestHeaderStringContainsKind(t *testing.T) {
	c := NewControl(1, 2, 3, 4)
	if got := c.String(); got == "" {
		t.Fatal("control String empty")
	}
	d := NewData(1, 2)
	if got := d.String(); got == "" {
		t.Fatal("data String empty")
	}
}

func TestHeaderLenInvalid(t *testing.T) {
	// L=1, Length=4 (less than header bytes).
	h := NewControl(1, 0, 0, 0)
	out := h.AppendTo(nil, 0)
	// Patch length to 4 to fake invalid value (less than 12-byte header).
	out[2] = 0
	out[3] = 4
	if _, _, err := Parse(out); err != ErrLengthInvalid {
		t.Fatalf("want ErrLengthInvalid, got %v", err)
	}
}
