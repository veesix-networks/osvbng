// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ip

import (
	"bytes"
	"strings"
	"testing"
)

func TestDHCPOption_Decode_ASCIIDefault(t *testing.T) {
	o := DHCPOption{Tag: 43, Value: "http://acs.example/"}
	got, err := o.Decode()
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	want := []byte("http://acs.example/")
	if !bytes.Equal(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDHCPOption_Decode_HexCompact(t *testing.T) {
	o := DHCPOption{Tag: 43, Encoding: "hex", Value: "deadbeef"}
	got, err := o.Decode()
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	want := []byte{0xde, 0xad, 0xbe, 0xef}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %x want %x", got, want)
	}
}

func TestDHCPOption_Decode_HexSeparators(t *testing.T) {
	cases := []string{
		"de:ad:be:ef",
		"de-ad-be-ef",
		"de ad be ef",
		"de\tad\nbe ef",
	}
	want := []byte{0xde, 0xad, 0xbe, 0xef}
	for _, in := range cases {
		o := DHCPOption{Tag: 43, Encoding: "hex", Value: in}
		got, err := o.Decode()
		if err != nil {
			t.Fatalf("Decode(%q): %v", in, err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("Decode(%q) = %x want %x", in, got, want)
		}
	}
}

func TestDHCPOption_Decode_OddNibble(t *testing.T) {
	o := DHCPOption{Tag: 43, Encoding: "hex", Value: "deadbee"}
	_, err := o.Decode()
	if err == nil {
		t.Fatal("expected odd-nibble error")
	}
}

func TestDHCPOption_Decode_UnknownEncoding(t *testing.T) {
	o := DHCPOption{Tag: 43, Encoding: "base64", Value: "ZGVhZGJlZWY="}
	_, err := o.Decode()
	if err == nil || !strings.Contains(err.Error(), "unknown encoding") {
		t.Fatalf("expected unknown-encoding error, got %v", err)
	}
}

func TestDHCPOption_Validate_TagBounds(t *testing.T) {
	for _, tag := range []uint8{0, 255} {
		o := DHCPOption{Tag: tag, Value: "x"}
		if err := o.Validate(); err == nil {
			t.Fatalf("tag %d should be rejected", tag)
		}
	}
}

func TestDHCPOption_Validate_Denylist(t *testing.T) {
	for tag := range dhcpv4Denylist {
		o := DHCPOption{Tag: tag, Value: "x"}
		if err := o.Validate(); err == nil {
			t.Fatalf("denylisted tag %d should be rejected", tag)
		}
	}
}

func TestDHCPOption_Validate_AcceptedTags(t *testing.T) {
	for _, tag := range []uint8{12, 15, 43, 60, 66, 67, 77} {
		o := DHCPOption{Tag: tag, Value: "x"}
		if err := o.Validate(); err != nil {
			t.Fatalf("tag %d should be accepted: %v", tag, err)
		}
	}
}

func TestDHCPOption_Validate_PayloadCap(t *testing.T) {
	o := DHCPOption{Tag: 43, Value: strings.Repeat("a", dhcpv4ValueMaxLen+1)}
	if err := o.Validate(); err == nil {
		t.Fatal("payload exceeding 255 bytes should be rejected")
	}
	o.Value = strings.Repeat("a", dhcpv4ValueMaxLen)
	if err := o.Validate(); err != nil {
		t.Fatalf("payload of 255 bytes should be accepted: %v", err)
	}
}

func TestDHCPv6Option_Decode_ASCIIDefault(t *testing.T) {
	o := DHCPv6Option{Code: 17, Value: "vendor"}
	got, err := o.Decode()
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !bytes.Equal(got, []byte("vendor")) {
		t.Fatalf("got %q want vendor", got)
	}
}

func TestDHCPv6Option_Decode_Hex(t *testing.T) {
	o := DHCPv6Option{Code: 17, Encoding: "hex", Value: "00:00:0d:e9"}
	got, err := o.Decode()
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	want := []byte{0x00, 0x00, 0x0d, 0xe9}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %x want %x", got, want)
	}
}

func TestDHCPv6Option_Validate_CodeZero(t *testing.T) {
	o := DHCPv6Option{Code: 0, Value: "x"}
	if err := o.Validate(); err == nil {
		t.Fatal("code 0 should be rejected")
	}
}

func TestDHCPv6Option_Validate_Denylist(t *testing.T) {
	for code := range dhcpv6Denylist {
		o := DHCPv6Option{Code: code, Value: "x"}
		if err := o.Validate(); err == nil {
			t.Fatalf("denylisted code %d should be rejected", code)
		}
	}
}

func TestDHCPv6Option_Validate_AcceptedCodes(t *testing.T) {
	for _, code := range []uint16{17, 24, 27, 31, 32, 82} {
		o := DHCPv6Option{Code: code, Value: "x"}
		if err := o.Validate(); err != nil {
			t.Fatalf("code %d should be accepted: %v", code, err)
		}
	}
}

func TestDHCPv6Option_Validate_PayloadCap(t *testing.T) {
	o := DHCPv6Option{Code: 17, Value: strings.Repeat("a", dhcpv6ValueMaxLen+1)}
	if err := o.Validate(); err == nil {
		t.Fatal("payload exceeding 65535 bytes should be rejected")
	}
}
