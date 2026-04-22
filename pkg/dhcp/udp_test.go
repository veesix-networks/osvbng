// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package dhcp

import (
	"encoding/binary"
	"net"
	"testing"
)

func TestBuildIPv4UDPFrame(t *testing.T) {
	src := net.ParseIP("10.0.0.1")
	dst := net.ParseIP("10.0.0.2")
	payload := []byte{0x01, 0x02, 0x03, 0x04}

	frame := BuildIPv4UDPFrame(src, dst, 67, 68, payload)
	if frame == nil {
		t.Fatal("expected non-nil frame")
	}

	if frame[0] != 0x45 {
		t.Fatalf("IPv4 version/IHL = 0x%02x, want 0x45", frame[0])
	}
	if frame[9] != 17 {
		t.Fatalf("protocol = %d, want 17 (UDP)", frame[9])
	}
	totalLen := binary.BigEndian.Uint16(frame[2:4])
	if totalLen != uint16(20+8+len(payload)) {
		t.Fatalf("total length = %d, want %d", totalLen, 20+8+len(payload))
	}
	if !net.IP(frame[12:16]).Equal(src.To4()) {
		t.Fatalf("src IP mismatch")
	}
	if !net.IP(frame[16:20]).Equal(dst.To4()) {
		t.Fatalf("dst IP mismatch")
	}

	srcPort := binary.BigEndian.Uint16(frame[20:22])
	dstPort := binary.BigEndian.Uint16(frame[22:24])
	if srcPort != 67 || dstPort != 68 {
		t.Fatalf("ports = %d/%d, want 67/68", srcPort, dstPort)
	}

	udpLen := binary.BigEndian.Uint16(frame[24:26])
	if udpLen != uint16(8+len(payload)) {
		t.Fatalf("UDP length = %d, want %d", udpLen, 8+len(payload))
	}

	for i, b := range payload {
		if frame[28+i] != b {
			t.Fatalf("payload[%d] = 0x%02x, want 0x%02x", i, frame[28+i], b)
		}
	}
}

func TestBuildIPv6UDPFrame(t *testing.T) {
	src := net.ParseIP("2001:db8::1")
	dst := net.ParseIP("2001:db8::2")
	payload := []byte{0x01, 0x02, 0x03, 0x04}

	frame := BuildIPv6UDPFrame(src, dst, 547, 546, payload)
	if frame == nil {
		t.Fatal("expected non-nil frame")
	}

	if frame[0]>>4 != 6 {
		t.Fatalf("IPv6 version = %d, want 6", frame[0]>>4)
	}
	if frame[6] != 17 {
		t.Fatalf("next header = %d, want 17 (UDP)", frame[6])
	}
	payloadLen := binary.BigEndian.Uint16(frame[4:6])
	if payloadLen != uint16(8+len(payload)) {
		t.Fatalf("payload length = %d, want %d", payloadLen, 8+len(payload))
	}
	if !net.IP(frame[8:24]).Equal(src.To16()) {
		t.Fatalf("src IP mismatch")
	}
	if !net.IP(frame[24:40]).Equal(dst.To16()) {
		t.Fatalf("dst IP mismatch")
	}

	srcPort := binary.BigEndian.Uint16(frame[40:42])
	dstPort := binary.BigEndian.Uint16(frame[42:44])
	if srcPort != 547 || dstPort != 546 {
		t.Fatalf("ports = %d/%d, want 547/546", srcPort, dstPort)
	}

	udpChecksum := binary.BigEndian.Uint16(frame[46:48])
	if udpChecksum == 0 {
		t.Fatal("IPv6 UDP checksum must not be zero")
	}
}

func TestBuildIPv4UDPFrameNilIP(t *testing.T) {
	frame := BuildIPv4UDPFrame(nil, net.ParseIP("10.0.0.1"), 67, 68, []byte{0x01})
	if frame != nil {
		t.Fatal("expected nil frame for nil src IP")
	}
}

func TestBuildIPv6UDPFrameNilIP(t *testing.T) {
	frame := BuildIPv6UDPFrame(nil, net.ParseIP("::1"), 547, 546, []byte{0x01})
	if frame != nil {
		t.Fatal("expected nil frame for nil src IP")
	}
}

func TestIPv4HeaderChecksum(t *testing.T) {
	src := net.ParseIP("10.0.0.1")
	dst := net.ParseIP("10.0.0.2")
	frame := BuildIPv4UDPFrame(src, dst, 67, 68, []byte{0xAA})

	var sum uint32
	for i := 0; i+1 < 20; i += 2 {
		sum += uint32(frame[i])<<8 | uint32(frame[i+1])
	}
	for sum > 0xFFFF {
		sum = (sum >> 16) + (sum & 0xFFFF)
	}
	if uint16(sum) != 0xFFFF {
		t.Fatalf("IPv4 header checksum verification failed: sum = 0x%04x, want 0xFFFF", sum)
	}
}

func TestUDPv6ChecksumNonZero(t *testing.T) {
	src := net.ParseIP("fe80::1")
	dst := net.ParseIP("ff02::1:2")
	payload := make([]byte, 100)
	for i := range payload {
		payload[i] = byte(i)
	}
	frame := BuildIPv6UDPFrame(src, dst, 547, 546, payload)

	checksum := binary.BigEndian.Uint16(frame[46:48])
	if checksum == 0 {
		t.Fatal("IPv6 UDP checksum must never be zero (RFC 8200)")
	}
}

func TestEmptyPayload(t *testing.T) {
	frame4 := BuildIPv4UDPFrame(net.ParseIP("10.0.0.1"), net.ParseIP("10.0.0.2"), 67, 68, nil)
	if frame4 == nil {
		t.Fatal("IPv4 frame should handle nil payload")
	}
	if len(frame4) != 28 {
		t.Fatalf("IPv4 frame len = %d, want 28", len(frame4))
	}

	frame6 := BuildIPv6UDPFrame(net.ParseIP("::1"), net.ParseIP("::2"), 547, 546, nil)
	if frame6 == nil {
		t.Fatal("IPv6 frame should handle nil payload")
	}
	if len(frame6) != 48 {
		t.Fatalf("IPv6 frame len = %d, want 48", len(frame6))
	}
}
