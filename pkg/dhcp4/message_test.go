// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package dhcp4

import (
	"encoding/binary"
	"net"
	"testing"
)

func buildTestDiscover() []byte {
	buf := make([]byte, 240)
	buf[0] = 1
	buf[1] = 1
	buf[2] = 6
	binary.BigEndian.PutUint32(buf[4:8], 0xDEADBEEF)
	copy(buf[28:34], []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF})
	binary.BigEndian.PutUint32(buf[236:240], 0x63825363)

	buf = append(buf, OptMessageType, 1, byte(MsgTypeDiscover))
	buf = append(buf, OptHostname, 5, 't', 'e', 's', 't', '1')
	buf = append(buf, OptClientID, 7, 1, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF)
	buf = append(buf, OptEnd)

	return buf
}

func TestParseMessageDiscover(t *testing.T) {
	data := buildTestDiscover()
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Op != 1 {
		t.Fatalf("op = %d, want 1", msg.Op)
	}
	if msg.XID != 0xDEADBEEF {
		t.Fatalf("XID = 0x%x, want 0xDEADBEEF", msg.XID)
	}
	if msg.ClientHWAddr.String() != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("MAC = %s, want aa:bb:cc:dd:ee:ff", msg.ClientHWAddr)
	}
	if msg.Options.MessageType != MsgTypeDiscover {
		t.Fatalf("msg type = %d, want Discover", msg.Options.MessageType)
	}
	if msg.Options.Hostname != "test1" {
		t.Fatalf("hostname = %q, want %q", msg.Options.Hostname, "test1")
	}
	if msg.Options.ClientID == nil {
		t.Fatal("expected ClientID")
	}
}

func TestParseMessageTooShort(t *testing.T) {
	_, err := ParseMessage(make([]byte, 100))
	if err != ErrTooShort {
		t.Fatalf("expected ErrTooShort, got %v", err)
	}
}

func TestParseMessageNoMagic(t *testing.T) {
	buf := make([]byte, 240)
	buf[0] = 1
	buf[1] = 1
	buf[2] = 6
	msg, err := ParseMessage(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Options.MessageType != 0 {
		t.Fatal("expected no options parsed without magic cookie")
	}
}

func TestParseWithDNS(t *testing.T) {
	buf := make([]byte, 240)
	buf[0] = 1
	buf[1] = 1
	buf[2] = 6
	binary.BigEndian.PutUint32(buf[236:240], 0x63825363)

	buf = append(buf, OptMessageType, 1, byte(MsgTypeOffer))
	buf = append(buf, OptDNS, 8, 8, 8, 8, 8, 8, 8, 4, 4)
	buf = append(buf, OptRouter, 4, 10, 0, 0, 1)
	buf = append(buf, OptSubnetMask, 4, 255, 255, 255, 0)
	buf = append(buf, OptLeaseTime, 4, 0, 0, 0x0E, 0x10)
	buf = append(buf, OptEnd)

	msg, err := ParseMessage(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msg.Options.DNS) != 2 {
		t.Fatalf("DNS count = %d, want 2", len(msg.Options.DNS))
	}
	if !msg.Options.DNS[0].Equal(net.ParseIP("8.8.8.8")) {
		t.Fatalf("DNS[0] = %v, want 8.8.8.8", msg.Options.DNS[0])
	}
	if !msg.Options.Router.Equal(net.ParseIP("10.0.0.1")) {
		t.Fatalf("router = %v, want 10.0.0.1", msg.Options.Router)
	}
	if msg.Options.LeaseTime != 3600 {
		t.Fatalf("lease time = %d, want 3600", msg.Options.LeaseTime)
	}
}

func TestParseTruncatedOption(t *testing.T) {
	buf := make([]byte, 240)
	buf[0] = 1
	buf[1] = 1
	buf[2] = 6
	binary.BigEndian.PutUint32(buf[236:240], 0x63825363)
	buf = append(buf, OptMessageType, 1, byte(MsgTypeDiscover))
	buf = append(buf, OptHostname, 0xFF)

	msg, err := ParseMessage(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Options.MessageType != MsgTypeDiscover {
		t.Fatal("should have parsed message type before truncation")
	}
	if msg.Options.Hostname != "" {
		t.Fatal("truncated option should not produce hostname")
	}
}

func TestDeepCopy(t *testing.T) {
	msg, _ := ParseMessage(buildTestDiscover())
	clone := msg.DeepCopy()

	msg.ClientHWAddr[0] = 0xFF
	msg.Options.Hostname = "modified"
	msg.XID = 0

	if clone.ClientHWAddr[0] == 0xFF {
		t.Fatal("DeepCopy MAC not independent")
	}
	if clone.Options.Hostname == "modified" {
		t.Fatal("DeepCopy hostname not independent")
	}
	if clone.XID == 0 {
		t.Fatal("DeepCopy XID not independent")
	}
}

func TestParseOption82(t *testing.T) {
	buf := make([]byte, 240)
	buf[0] = 1
	buf[1] = 1
	buf[2] = 6
	binary.BigEndian.PutUint32(buf[236:240], 0x63825363)

	buf = append(buf, OptMessageType, 1, byte(MsgTypeDiscover))
	opt82 := []byte{0x01, 0x04, 'e', 't', 'h', '0', 0x02, 0x03, 'b', 'n', 'g'}
	buf = append(buf, OptOption82, byte(len(opt82)))
	buf = append(buf, opt82...)
	buf = append(buf, OptEnd)

	msg, err := ParseMessage(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Options.Option82 == nil {
		t.Fatal("expected Option82")
	}
	if len(msg.Options.Option82) != len(opt82) {
		t.Fatalf("Option82 len = %d, want %d", len(msg.Options.Option82), len(opt82))
	}
}
