// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package vpp

import (
	"testing"

	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/fib_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip_types"
)

func TestFibPathTypeName(t *testing.T) {
	cases := map[fib_types.FibPathType]string{
		fib_types.FIB_API_PATH_TYPE_NORMAL:        "normal",
		fib_types.FIB_API_PATH_TYPE_LOCAL:         "local",
		fib_types.FIB_API_PATH_TYPE_DROP:          "drop",
		fib_types.FIB_API_PATH_TYPE_UDP_ENCAP:     "udp-encap",
		fib_types.FIB_API_PATH_TYPE_BIER_IMP:      "bier-imp",
		fib_types.FIB_API_PATH_TYPE_ICMP_UNREACH:  "icmp-unreach",
		fib_types.FIB_API_PATH_TYPE_ICMP_PROHIBIT: "icmp-prohibit",
		fib_types.FIB_API_PATH_TYPE_SOURCE_LOOKUP: "source-lookup",
		fib_types.FIB_API_PATH_TYPE_DVR:           "dvr",
		fib_types.FIB_API_PATH_TYPE_INTERFACE_RX:  "interface-rx",
		fib_types.FIB_API_PATH_TYPE_CLASSIFY:      "classify",
		fib_types.FibPathType(99):                 "unknown",
	}
	for in, want := range cases {
		if got := fibPathTypeName(in); got != want {
			t.Errorf("fibPathTypeName(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestFibPathProtoName(t *testing.T) {
	cases := map[fib_types.FibPathNhProto]string{
		fib_types.FIB_API_PATH_NH_PROTO_IP4:      "ip4",
		fib_types.FIB_API_PATH_NH_PROTO_IP6:      "ip6",
		fib_types.FIB_API_PATH_NH_PROTO_MPLS:     "mpls",
		fib_types.FIB_API_PATH_NH_PROTO_ETHERNET: "ethernet",
		fib_types.FIB_API_PATH_NH_PROTO_BIER:     "bier",
		fib_types.FibPathNhProto(99):             "unknown",
	}
	for in, want := range cases {
		if got := fibPathProtoName(in); got != want {
			t.Errorf("fibPathProtoName(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestFibNextHopStringIPv4(t *testing.T) {
	var un ip_types.AddressUnion
	un.SetIP4(ip_types.IP4Address{10, 0, 0, 1})
	if got := fibNextHopString(fib_types.FIB_API_PATH_NH_PROTO_IP4, un); got != "10.0.0.1" {
		t.Errorf("ipv4 next hop = %q, want 10.0.0.1", got)
	}
}

func TestFibNextHopStringIPv6(t *testing.T) {
	var un ip_types.AddressUnion
	un.SetIP6(ip_types.IP6Address{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x01})
	if got := fibNextHopString(fib_types.FIB_API_PATH_NH_PROTO_IP6, un); got != "2001:db8::1" {
		t.Errorf("ipv6 next hop = %q, want 2001:db8::1", got)
	}
}

func TestFibNextHopStringZeroAddressReturnsEmpty(t *testing.T) {
	var un ip_types.AddressUnion
	if got := fibNextHopString(fib_types.FIB_API_PATH_NH_PROTO_IP4, un); got != "" {
		t.Errorf("zero ipv4 next hop = %q, want empty", got)
	}
	if got := fibNextHopString(fib_types.FIB_API_PATH_NH_PROTO_IP6, un); got != "" {
		t.Errorf("zero ipv6 next hop = %q, want empty", got)
	}
}

func TestFibNextHopStringMPLSReturnsEmpty(t *testing.T) {
	var un ip_types.AddressUnion
	un.SetIP4(ip_types.IP4Address{10, 0, 0, 1})
	if got := fibNextHopString(fib_types.FIB_API_PATH_NH_PROTO_MPLS, un); got != "" {
		t.Errorf("mpls next hop = %q, want empty (no peer IP encoded)", got)
	}
}

func TestFibTableNameDefaultZero(t *testing.T) {
	if got := fibTableName(0, ""); got != "default" {
		t.Errorf("table 0 unnamed = %q, want default", got)
	}
}

func TestFibTableNameKeepsExplicit(t *testing.T) {
	if got := fibTableName(200, "CUSTOMER-A"); got != "CUSTOMER-A" {
		t.Errorf("table 200 named = %q, want CUSTOMER-A", got)
	}
}

func TestFibTableNameStripsNul(t *testing.T) {
	if got := fibTableName(200, "CUSTOMER-A\x00\x00"); got != "CUSTOMER-A" {
		t.Errorf("table name nul-padded = %q, want CUSTOMER-A", got)
	}
}

func TestFibTableNameNonZeroUnnamed(t *testing.T) {
	if got := fibTableName(7, ""); got != "" {
		t.Errorf("unnamed non-zero table = %q, want empty (do not call it 'default')", got)
	}
}
