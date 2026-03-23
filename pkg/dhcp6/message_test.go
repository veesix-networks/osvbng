// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package dhcp6

import (
	"encoding/binary"
	"net"
	"testing"
)

func buildTestSolicit() []byte {
	buf := make([]byte, 0, 128)
	buf = append(buf, byte(MsgTypeSolicit))
	buf = append(buf, 0xAA, 0xBB, 0xCC)

	clientDUID := []byte{0x00, 0x01, 0x00, 0x01, 0x01, 0x02, 0x03, 0x04, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	buf = appendOption(buf, OptClientID, clientDUID)

	ianaData := make([]byte, 40)
	binary.BigEndian.PutUint32(ianaData[0:4], 1)
	binary.BigEndian.PutUint32(ianaData[4:8], 3600)
	binary.BigEndian.PutUint32(ianaData[8:12], 5400)
	binary.BigEndian.PutUint16(ianaData[12:14], OptIAAddr)
	binary.BigEndian.PutUint16(ianaData[14:16], 24)
	copy(ianaData[16:32], net.ParseIP("2001:db8::1").To16())
	binary.BigEndian.PutUint32(ianaData[32:36], 3600)
	binary.BigEndian.PutUint32(ianaData[36:40], 7200)
	buf = appendOption(buf, OptIANA, ianaData)

	return buf
}

func appendOption(buf []byte, code uint16, data []byte) []byte {
	opt := make([]byte, 4)
	binary.BigEndian.PutUint16(opt[0:2], code)
	binary.BigEndian.PutUint16(opt[2:4], uint16(len(data)))
	buf = append(buf, opt...)
	buf = append(buf, data...)
	return buf
}

func TestParseMessageSolicit(t *testing.T) {
	data := buildTestSolicit()
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.MsgType != MsgTypeSolicit {
		t.Fatalf("msg type = %d, want %d", msg.MsgType, MsgTypeSolicit)
	}
	if msg.TransactionID != [3]byte{0xAA, 0xBB, 0xCC} {
		t.Fatalf("xid = %v, want [AA BB CC]", msg.TransactionID)
	}
	if msg.Options.ClientID == nil {
		t.Fatal("expected ClientID")
	}
	if msg.Options.IANA == nil {
		t.Fatal("expected IANA option")
	}
	if msg.Options.IANA.IAID != 1 {
		t.Fatalf("IANA IAID = %d, want 1", msg.Options.IANA.IAID)
	}
	if !msg.Options.IANA.Address.Equal(net.ParseIP("2001:db8::1")) {
		t.Fatalf("IANA addr = %v, want 2001:db8::1", msg.Options.IANA.Address)
	}
	if msg.Options.IANA.ValidTime != 7200 {
		t.Fatalf("IANA valid time = %d, want 7200", msg.Options.IANA.ValidTime)
	}
}

func TestParseMessageTooShort(t *testing.T) {
	_, err := ParseMessage([]byte{0x01, 0x02})
	if err != ErrTooShort {
		t.Fatalf("expected ErrTooShort, got %v", err)
	}
}

func TestParseMessageTruncatedOption(t *testing.T) {
	data := []byte{byte(MsgTypeSolicit), 0x00, 0x00, 0x01}
	data = append(data, 0x00, 0x01, 0x00, 0xFF)
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Options.ClientID != nil {
		t.Fatal("truncated option should not produce ClientID")
	}
}

func TestParseIAPD(t *testing.T) {
	data := make([]byte, 4)
	data[0] = byte(MsgTypeSolicit)
	copy(data[1:4], []byte{0x01, 0x02, 0x03})

	iapdData := make([]byte, 41)
	binary.BigEndian.PutUint32(iapdData[0:4], 2)
	binary.BigEndian.PutUint32(iapdData[4:8], 3600)
	binary.BigEndian.PutUint32(iapdData[8:12], 5400)
	binary.BigEndian.PutUint16(iapdData[12:14], OptIAPrefix)
	binary.BigEndian.PutUint16(iapdData[14:16], 25)
	binary.BigEndian.PutUint32(iapdData[16:20], 1800)
	binary.BigEndian.PutUint32(iapdData[20:24], 3600)
	iapdData[24] = 56
	copy(iapdData[25:41], net.ParseIP("2001:db8:1::").To16())

	data = appendOption(data, OptIAPD, iapdData)

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Options.IAPD == nil {
		t.Fatal("expected IAPD option")
	}
	if msg.Options.IAPD.IAID != 2 {
		t.Fatalf("IAPD IAID = %d, want 2", msg.Options.IAPD.IAID)
	}
	if msg.Options.IAPD.PrefixLen != 56 {
		t.Fatalf("prefix len = %d, want 56", msg.Options.IAPD.PrefixLen)
	}
	if !msg.Options.IAPD.Prefix.Equal(net.ParseIP("2001:db8:1::")) {
		t.Fatalf("prefix = %v, want 2001:db8:1::", msg.Options.IAPD.Prefix)
	}
}

func TestSerializeRoundTrip(t *testing.T) {
	resp := &Response{
		MsgType:       MsgTypeAdvertise,
		TransactionID: [3]byte{0xAA, 0xBB, 0xCC},
		ClientID:      []byte{0x00, 0x01, 0x00, 0x01, 0x01, 0x02, 0x03, 0x04, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF},
		ServerID:      []byte{0x00, 0x01, 0x00, 0x01, 0x05, 0x06, 0x07, 0x08, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66},
		IANA: &IANAOption{
			IAID:          1,
			T1:            3600,
			T2:            5400,
			Address:       net.ParseIP("2001:db8::1"),
			PreferredTime: 3600,
			ValidTime:     7200,
		},
		DNS: []net.IP{net.ParseIP("2001:4860:4860::8888")},
	}

	data := resp.Serialize()
	if len(data) == 0 {
		t.Fatal("serialize returned empty")
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if msg.MsgType != MsgTypeAdvertise {
		t.Fatalf("msg type = %d, want %d", msg.MsgType, MsgTypeAdvertise)
	}
	if msg.TransactionID != [3]byte{0xAA, 0xBB, 0xCC} {
		t.Fatalf("xid mismatch")
	}
	if msg.Options.ClientID == nil {
		t.Fatal("missing ClientID")
	}
	if msg.Options.ServerID == nil {
		t.Fatal("missing ServerID")
	}
	if msg.Options.IANA == nil {
		t.Fatal("missing IANA")
	}
	if !msg.Options.IANA.Address.Equal(net.ParseIP("2001:db8::1")) {
		t.Fatalf("IANA addr = %v, want 2001:db8::1", msg.Options.IANA.Address)
	}
	if len(msg.Options.DNS) != 1 {
		t.Fatalf("DNS count = %d, want 1", len(msg.Options.DNS))
	}
}

func TestSerializeWithIAPD(t *testing.T) {
	resp := &Response{
		MsgType:       MsgTypeReply,
		TransactionID: [3]byte{0x01, 0x02, 0x03},
		ClientID:      []byte{0x00, 0x01},
		ServerID:      []byte{0x00, 0x02},
		IANA: &IANAOption{
			IAID:          1,
			Address:       net.ParseIP("2001:db8::100"),
			PreferredTime: 3600,
			ValidTime:     7200,
		},
		IAPD: &IAPDOption{
			IAID:          2,
			PrefixLen:     56,
			Prefix:        net.ParseIP("2001:db8:1::"),
			PreferredTime: 1800,
			ValidTime:     3600,
		},
	}

	data := resp.Serialize()
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if msg.Options.IANA == nil || msg.Options.IAPD == nil {
		t.Fatal("expected both IANA and IAPD")
	}
	if msg.Options.IAPD.PrefixLen != 56 {
		t.Fatalf("PD prefix len = %d, want 56", msg.Options.IAPD.PrefixLen)
	}
}

func TestDeepCopy(t *testing.T) {
	msg, _ := ParseMessage(buildTestSolicit())
	clone := msg.DeepCopy()

	msg.Options.ClientID[0] = 0xFF
	msg.Options.IANA.Address[0] = 0xFF
	msg.TransactionID[0] = 0xFF

	if clone.Options.ClientID[0] == 0xFF {
		t.Fatal("DeepCopy ClientID not independent")
	}
	if clone.Options.IANA.Address[0] == 0xFF {
		t.Fatal("DeepCopy IANA Address not independent")
	}
	if clone.TransactionID[0] == 0xFF {
		t.Fatal("DeepCopy TransactionID not independent")
	}
}

func TestUnwrapRelay(t *testing.T) {
	inner := buildTestSolicit()

	relay := make([]byte, 0, 128)
	relay = append(relay, byte(MsgTypeRelayForward))
	relay = append(relay, 0)
	relay = append(relay, make([]byte, 16)...)
	relay = append(relay, make([]byte, 16)...)

	ifID := []byte("eth0:100:200")
	relay = appendOption(relay, OptInterfaceID, ifID)
	relay = appendOption(relay, OptRelayMsg, inner)

	msg, info := UnwrapRelay(relay)
	if msg == nil {
		t.Fatal("expected unwrapped message")
	}
	if info == nil {
		t.Fatal("expected relay info")
	}
	if msg.MsgType != MsgTypeSolicit {
		t.Fatalf("inner msg type = %d, want %d", msg.MsgType, MsgTypeSolicit)
	}
	if string(info.InterfaceID) != "eth0:100:200" {
		t.Fatalf("interface ID = %q, want %q", info.InterfaceID, "eth0:100:200")
	}
}

func TestUnwrapRelayMultiHop(t *testing.T) {
	inner := buildTestSolicit()

	relay1 := make([]byte, 0, 128)
	relay1 = append(relay1, byte(MsgTypeRelayForward))
	relay1 = append(relay1, 0)
	relay1 = append(relay1, make([]byte, 32)...)
	relay1 = appendOption(relay1, OptInterfaceID, []byte("inner"))
	relay1 = appendOption(relay1, OptRelayMsg, inner)

	relay2 := make([]byte, 0, 256)
	relay2 = append(relay2, byte(MsgTypeRelayForward))
	relay2 = append(relay2, 1)
	relay2 = append(relay2, make([]byte, 32)...)
	relay2 = appendOption(relay2, OptInterfaceID, []byte("outer"))
	relay2 = appendOption(relay2, OptRelayMsg, relay1)

	msg, info := UnwrapRelay(relay2)
	if msg == nil {
		t.Fatal("expected unwrapped message")
	}
	if msg.MsgType != MsgTypeSolicit {
		t.Fatalf("inner msg type = %d, want Solicit", msg.MsgType)
	}
	if string(info.InterfaceID) != "inner" {
		t.Fatalf("interface ID = %q, want %q", info.InterfaceID, "inner")
	}
}

func TestParseEmptyOptions(t *testing.T) {
	data := []byte{byte(MsgTypeSolicit), 0x00, 0x00, 0x01}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Options.ClientID != nil {
		t.Fatal("expected nil ClientID")
	}
}

func TestUnwrapRelayNotRelay(t *testing.T) {
	data := buildTestSolicit()
	msg, info := UnwrapRelay(data)
	if msg != nil || info != nil {
		t.Fatal("expected nil for non-relay message")
	}
}
