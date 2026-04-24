// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package relay

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/dhcp6"
)

func TestBuildRelayReply_SingleHopRoundTrip(t *testing.T) {
	inner := []byte{byte(dhcp6.MsgTypeReply), 0x11, 0x22, 0x33, 0x00, 0x01, 0x00, 0x00}
	info := &dhcp6.RelayInfo{
		HopCount:    0,
		LinkAddr:    net.ParseIP("2001:db8::1"),
		PeerAddr:    net.ParseIP("fe80::1234"),
		InterfaceID: []byte("eth 0/1/2:100.50"),
	}

	raw := BuildRelayReply(inner, info)
	if len(raw) == 0 {
		t.Fatal("BuildRelayReply returned empty buffer")
	}
	if raw[0] != DHCPv6MsgRelayReply {
		t.Fatalf("msg-type: got %d want %d", raw[0], DHCPv6MsgRelayReply)
	}
	if raw[1] != 0 {
		t.Fatalf("hop-count: got %d want 0", raw[1])
	}
	if !bytes.Equal(raw[2:18], info.LinkAddr.To16()) {
		t.Fatalf("link-addr: got %x want %x", raw[2:18], info.LinkAddr.To16())
	}
	if !bytes.Equal(raw[18:34], info.PeerAddr.To16()) {
		t.Fatalf("peer-addr: got %x want %x", raw[18:34], info.PeerAddr.To16())
	}

	extracted, err := UnwrapRelayReply(raw)
	if err != nil {
		t.Fatalf("UnwrapRelayReply: %v", err)
	}
	if !bytes.Equal(extracted, inner) {
		t.Fatalf("inner mismatch:\n got %x\nwant %x", extracted, inner)
	}
}

func TestBuildRelayReply_InterfaceIDEchoVerbatim(t *testing.T) {
	inner := []byte{byte(dhcp6.MsgTypeReply), 0, 0, 0}
	iid := []byte("port-42/vlan-100/remote-id=WHOLESALER-A-CPE-9876543210")
	info := &dhcp6.RelayInfo{
		LinkAddr:    net.IPv6zero,
		PeerAddr:    net.ParseIP("fe80::cafe"),
		InterfaceID: iid,
	}

	raw := BuildRelayReply(inner, info)
	echoed := findV6Option(t, raw, DHCPv6OptInterfaceID)
	if !bytes.Equal(echoed, iid) {
		t.Fatalf("interface-id echo mismatch:\n got %q\nwant %q", echoed, iid)
	}
}

func TestBuildRelayReply_LargeInterfaceID(t *testing.T) {
	inner := []byte{byte(dhcp6.MsgTypeReply), 0, 0, 0}
	iid := make([]byte, 1024)
	for i := range iid {
		iid[i] = byte(i % 256)
	}
	info := &dhcp6.RelayInfo{
		LinkAddr:    net.IPv6zero,
		PeerAddr:    net.ParseIP("fe80::1"),
		InterfaceID: iid,
	}

	raw := BuildRelayReply(inner, info)
	echoed := findV6Option(t, raw, DHCPv6OptInterfaceID)
	if !bytes.Equal(echoed, iid) {
		t.Fatalf("large interface-id not echoed verbatim")
	}
}

func TestBuildRelayReply_EmptyInterfaceIDOmitted(t *testing.T) {
	inner := []byte{byte(dhcp6.MsgTypeReply), 0, 0, 0}
	info := &dhcp6.RelayInfo{
		LinkAddr:    net.ParseIP("2001:db8::1"),
		PeerAddr:    net.ParseIP("fe80::1"),
		InterfaceID: nil,
	}

	raw := BuildRelayReply(inner, info)
	if len(raw) == 0 {
		t.Fatal("empty")
	}
	if opt := findV6Option(t, raw, DHCPv6OptInterfaceID); opt != nil {
		t.Fatalf("expected no Interface-ID option, got %x", opt)
	}

	extracted, err := UnwrapRelayReply(raw)
	if err != nil {
		t.Fatalf("UnwrapRelayReply: %v", err)
	}
	if !bytes.Equal(extracted, inner) {
		t.Fatal("inner mismatch when Interface-ID omitted")
	}
}

func TestBuildRelayReply_RemoteIDNotEchoed(t *testing.T) {
	inner := []byte{byte(dhcp6.MsgTypeReply), 0, 0, 0}
	info := &dhcp6.RelayInfo{
		LinkAddr:    net.IPv6zero,
		PeerAddr:    net.IPv6zero,
		InterfaceID: []byte("iid"),
		RemoteID:    []byte("wholesaler-remote-id-bytes"),
	}

	raw := BuildRelayReply(inner, info)
	if opt := findV6Option(t, raw, DHCPv6OptRemoteID); opt != nil {
		t.Fatalf("Remote-Id should not be echoed in Relay-Reply; found %x", opt)
	}
}

func TestBuildRelayReply_NilAddressZeroFill(t *testing.T) {
	inner := []byte{byte(dhcp6.MsgTypeReply), 0, 0, 0}
	info := &dhcp6.RelayInfo{
		HopCount:    0,
		LinkAddr:    nil,
		PeerAddr:    nil,
		InterfaceID: []byte("iid"),
	}

	raw := BuildRelayReply(inner, info)
	if !bytes.Equal(raw[2:18], net.IPv6zero.To16()) {
		t.Fatalf("nil LinkAddr not zero-filled: %x", raw[2:18])
	}
	if !bytes.Equal(raw[18:34], net.IPv6zero.To16()) {
		t.Fatalf("nil PeerAddr not zero-filled: %x", raw[18:34])
	}
}

func TestBuildRelayReply_NilInfoReturnsNil(t *testing.T) {
	inner := []byte{byte(dhcp6.MsgTypeReply), 0, 0, 0}
	if raw := BuildRelayReply(inner, nil); raw != nil {
		t.Fatalf("expected nil for nil info, got %x", raw)
	}
}

func TestBuildRelayReply_HopCountPreserved(t *testing.T) {
	inner := []byte{byte(dhcp6.MsgTypeReply), 0, 0, 0}
	info := &dhcp6.RelayInfo{
		HopCount:    7,
		LinkAddr:    net.ParseIP("2001:db8::1"),
		PeerAddr:    net.ParseIP("fe80::1"),
		InterfaceID: []byte("iid"),
	}

	raw := BuildRelayReply(inner, info)
	if raw[1] != 7 {
		t.Fatalf("hop-count: got %d want 7", raw[1])
	}
}

func findV6Option(t *testing.T, pkt []byte, optCode uint16) []byte {
	t.Helper()
	if len(pkt) < DHCPv6RelayHeaderLen {
		return nil
	}
	i := DHCPv6RelayHeaderLen
	for i+4 <= len(pkt) {
		code := binary.BigEndian.Uint16(pkt[i:])
		optLen := binary.BigEndian.Uint16(pkt[i+2:])
		if i+4+int(optLen) > len(pkt) {
			return nil
		}
		if code == optCode {
			out := make([]byte, optLen)
			copy(out, pkt[i+4:i+4+int(optLen)])
			return out
		}
		i += 4 + int(optLen)
	}
	return nil
}
