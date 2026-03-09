// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package radius

import (
	"encoding/binary"
	"net"
	"regexp"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/aaa"
	"layeh.com/radius"
)

func TestPapEncode(t *testing.T) {
	secret := []byte("testing123")
	authenticator := make([]byte, 16)
	for i := range authenticator {
		authenticator[i] = byte(i)
	}

	t.Run("short password", func(t *testing.T) {
		result := papEncode([]byte("hello"), secret, authenticator)
		if len(result) != 16 {
			t.Fatalf("expected 16 bytes, got %d", len(result))
		}
	})

	t.Run("empty password", func(t *testing.T) {
		result := papEncode([]byte{}, secret, authenticator)
		if len(result) != 16 {
			t.Fatalf("expected 16 bytes, got %d", len(result))
		}
	})

	t.Run("17 byte password pads to 32", func(t *testing.T) {
		pw := make([]byte, 17)
		for i := range pw {
			pw[i] = 'a'
		}
		result := papEncode(pw, secret, authenticator)
		if len(result) != 32 {
			t.Fatalf("expected 32 bytes, got %d", len(result))
		}
	})

	t.Run("deterministic output", func(t *testing.T) {
		r1 := papEncode([]byte("test"), secret, authenticator)
		r2 := papEncode([]byte("test"), secret, authenticator)
		if string(r1) != string(r2) {
			t.Fatal("same inputs produced different outputs")
		}
	})

	t.Run("different passwords differ", func(t *testing.T) {
		r1 := papEncode([]byte("foo"), secret, authenticator)
		r2 := papEncode([]byte("bar"), secret, authenticator)
		if string(r1) == string(r2) {
			t.Fatal("different passwords produced same output")
		}
	})
}

func TestEncodeUint32(t *testing.T) {
	cases := []struct {
		input    uint32
		expected uint32
	}{
		{0, 0},
		{1, 1},
		{0xFFFFFFFF, 0xFFFFFFFF},
		{256, 256},
	}

	for _, tc := range cases {
		attr := encodeUint32(tc.input)
		if len(attr) != 4 {
			t.Fatalf("expected 4 bytes, got %d", len(attr))
		}
		got := binary.BigEndian.Uint32(attr)
		if got != tc.expected {
			t.Fatalf("encodeUint32(%d): got %d", tc.input, got)
		}
	}
}

func TestServiceTypeForAccess(t *testing.T) {
	if serviceTypeForAccess("pppoe") != 2 {
		t.Fatal("pppoe should be 2 (Framed)")
	}
	if serviceTypeForAccess("ipoe") != 5 {
		t.Fatal("ipoe should be 5 (Outbound)")
	}
	if serviceTypeForAccess("") != 5 {
		t.Fatal("empty should default to 5")
	}
}

func TestNasPortTypeValue(t *testing.T) {
	cases := []struct {
		name     string
		expected uint32
	}{
		{"Async", 0},
		{"Virtual", 5},
		{"Ethernet", 15},
		{"PPPoEoQinQ", 34},
		{"unknown", 5},
		{"", 5},
	}
	for _, tc := range cases {
		got := nasPortTypeValue(tc.name)
		if got != tc.expected {
			t.Fatalf("nasPortTypeValue(%q): got %d, want %d", tc.name, got, tc.expected)
		}
	}
}

func TestDecodeIPv4(t *testing.T) {
	ip := net.ParseIP("10.255.0.1").To4()
	result := decodeIPv4(radius.Attribute(ip))
	if result != "10.255.0.1" {
		t.Fatalf("got %q, want 10.255.0.1", result)
	}

	if decodeIPv4(radius.Attribute([]byte{1, 2})) != "" {
		t.Fatal("short input should return empty")
	}
}

func TestDecodeIPv6Address(t *testing.T) {
	ip := net.ParseIP("2001:db8::1")
	result := decodeIPv6Address(radius.Attribute(ip))
	if result != "2001:db8::1" {
		t.Fatalf("got %q, want 2001:db8::1", result)
	}

	if decodeIPv6Address(radius.Attribute([]byte{1})) != "" {
		t.Fatal("short input should return empty")
	}
}

func TestDecodeIPv6Prefix(t *testing.T) {
	attr := make([]byte, 10)
	attr[0] = 0
	attr[1] = 56
	copy(attr[2:], net.ParseIP("2001:db8:100::").To16()[:8])
	result := decodeIPv6Prefix(radius.Attribute(attr))
	if result != "2001:db8:100::/56" {
		t.Fatalf("got %q", result)
	}

	if decodeIPv6Prefix(radius.Attribute([]byte{0, 0})) != "" {
		t.Fatal("short input should return empty")
	}
}

func TestDecodeUint32Attr(t *testing.T) {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, 3600)
	result := decodeUint32(radius.Attribute(buf))
	if result != "3600" {
		t.Fatalf("got %q, want 3600", result)
	}

	if decodeUint32(radius.Attribute([]byte{1})) != "" {
		t.Fatal("short input should return empty")
	}
}

func TestDecodeVSAIPv4(t *testing.T) {
	ip := net.ParseIP("8.8.8.8").To4()
	result := decodeVSAIPv4(ip)
	if result != "8.8.8.8" {
		t.Fatalf("got %q, want 8.8.8.8", result)
	}

	if decodeVSAIPv4([]byte{1}) != "" {
		t.Fatal("short input should return empty")
	}
}

func TestBuildTier1Index(t *testing.T) {
	idx := buildTier1Index()
	if idx[8] == nil {
		t.Fatal("type 8 (Framed-IP-Address) should be in tier1")
	}
	if idx[8].internal != aaa.AttrIPv4Address {
		t.Fatalf("type 8 internal: got %q, want %q", idx[8].internal, aaa.AttrIPv4Address)
	}
	if idx[168] == nil {
		t.Fatal("type 168 (Framed-IPv6-Address) should be in tier1")
	}
	if idx[0] != nil {
		t.Fatal("type 0 should not be in tier1")
	}
}

func TestBuildTier2Index(t *testing.T) {
	idx := buildTier2Index()
	key := vendorKey{vendorID: 311, vendorType: 28}
	if idx[key] == nil {
		t.Fatal("MS-Primary-DNS-Server should be in tier2")
	}
	if idx[key].internal != aaa.AttrDNSPrimary {
		t.Fatalf("got %q, want %q", idx[key].internal, aaa.AttrDNSPrimary)
	}
}

func TestExtractAttributes(t *testing.T) {
	p := &Provider{
		tier1Index: buildTier1Index(),
		tier2Index: buildTier2Index(),
	}

	resp := radius.New(radius.CodeAccessAccept, []byte("testing123456789"))
	resp.Add(8, radius.Attribute(net.ParseIP("10.255.0.100").To4()))
	resp.Add(9, radius.Attribute(net.ParseIP("255.255.255.255").To4()))

	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, 3600)
	resp.Add(27, radius.Attribute(buf))

	attrs := p.extractAttributes(resp)

	if attrs[aaa.AttrIPv4Address] != "10.255.0.100" {
		t.Fatalf("ipv4_address: got %q", attrs[aaa.AttrIPv4Address])
	}
	if attrs[aaa.AttrIPv4Netmask] != "255.255.255.255" {
		t.Fatalf("ipv4_netmask: got %q", attrs[aaa.AttrIPv4Netmask])
	}
	if attrs[aaa.AttrSessionTimeout] != "3600" {
		t.Fatalf("session_timeout: got %q", attrs[aaa.AttrSessionTimeout])
	}
}

func TestExtractVSA(t *testing.T) {
	p := &Provider{
		tier1Index: buildTier1Index(),
		tier2Index: buildTier2Index(),
	}

	t.Run("tier2 MS-DNS", func(t *testing.T) {
		vsa := buildVSA(311, 28, net.ParseIP("8.8.8.8").To4())
		attrs := make(map[string]string)
		p.extractVSA(radius.Attribute(vsa), attrs)
		if attrs[aaa.AttrDNSPrimary] != "8.8.8.8" {
			t.Fatalf("dns_primary: got %q", attrs[aaa.AttrDNSPrimary])
		}
	})

	t.Run("tier3 custom mapping", func(t *testing.T) {
		p2 := &Provider{
			tier1Index: buildTier1Index(),
			tier2Index: buildTier2Index(),
			tier3: []compiledCustomMapping{
				{vendorID: 9, vendorType: 1, internal: "vrf"},
			},
		}
		vsa := buildVSA(9, 1, []byte("ip:vrf-name=CUSTOMER-A"))
		attrs := make(map[string]string)
		p2.extractVSA(radius.Attribute(vsa), attrs)
		if attrs["vrf"] != "ip:vrf-name=CUSTOMER-A" {
			t.Fatalf("vrf: got %q", attrs["vrf"])
		}
	})

	t.Run("tier3 with regex extract", func(t *testing.T) {
		re := mustCompileRegexp(`vrf-name=(.+)`)
		p3 := &Provider{
			tier1Index: buildTier1Index(),
			tier2Index: buildTier2Index(),
			tier3: []compiledCustomMapping{
				{vendorID: 9, vendorType: 1, internal: "vrf", extract: re},
			},
		}
		vsa := buildVSA(9, 1, []byte("ip:vrf-name=CUSTOMER-A"))
		attrs := make(map[string]string)
		p3.extractVSA(radius.Attribute(vsa), attrs)
		if attrs["vrf"] != "CUSTOMER-A" {
			t.Fatalf("vrf: got %q", attrs["vrf"])
		}
	})

	t.Run("short VSA ignored", func(t *testing.T) {
		attrs := make(map[string]string)
		p.extractVSA(radius.Attribute([]byte{0, 0}), attrs)
		if len(attrs) != 0 {
			t.Fatal("short VSA should produce no attributes")
		}
	})
}

func TestConfigValidation(t *testing.T) {
	t.Run("no servers", func(t *testing.T) {
		cfg := &Config{}
		if err := cfg.validate(); err == nil {
			t.Fatal("expected error for no servers")
		}
	})

	t.Run("empty host", func(t *testing.T) {
		cfg := &Config{Servers: []ServerConfig{{Secret: "test"}}}
		if err := cfg.validate(); err == nil {
			t.Fatal("expected error for empty host")
		}
	})

	t.Run("empty secret", func(t *testing.T) {
		cfg := &Config{Servers: []ServerConfig{{Host: "127.0.0.1"}}}
		if err := cfg.validate(); err == nil {
			t.Fatal("expected error for empty secret")
		}
	})

	t.Run("valid config", func(t *testing.T) {
		cfg := &Config{Servers: []ServerConfig{{Host: "127.0.0.1", Secret: "test"}}}
		if err := cfg.validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("response mapping without internal", func(t *testing.T) {
		cfg := &Config{
			Servers:          []ServerConfig{{Host: "127.0.0.1", Secret: "test"}},
			ResponseMappings: []CustomMapping{{VendorID: 9}},
		}
		if err := cfg.validate(); err == nil {
			t.Fatal("expected error for missing internal")
		}
	})

	t.Run("response mapping without identifier", func(t *testing.T) {
		cfg := &Config{
			Servers:          []ServerConfig{{Host: "127.0.0.1", Secret: "test"}},
			ResponseMappings: []CustomMapping{{Internal: "vrf"}},
		}
		if err := cfg.validate(); err == nil {
			t.Fatal("expected error for missing radius_attr and vendor_id")
		}
	})
}

func TestConfigApplyDefaults(t *testing.T) {
	cfg := &Config{}
	cfg.applyDefaults()

	if cfg.AuthPort != DefaultAuthPort {
		t.Fatalf("auth_port: got %d, want %d", cfg.AuthPort, DefaultAuthPort)
	}
	if cfg.AcctPort != DefaultAcctPort {
		t.Fatalf("acct_port: got %d, want %d", cfg.AcctPort, DefaultAcctPort)
	}
	if cfg.Timeout != DefaultTimeout {
		t.Fatalf("timeout: got %v, want %v", cfg.Timeout, DefaultTimeout)
	}
	if cfg.Retries != DefaultRetries {
		t.Fatalf("retries: got %d, want %d", cfg.Retries, DefaultRetries)
	}
	if cfg.NASPortType != DefaultNASPortType {
		t.Fatalf("nas_port_type: got %q, want %q", cfg.NASPortType, DefaultNASPortType)
	}
	if cfg.DeadTime != DefaultDeadTime {
		t.Fatalf("dead_time: got %v, want %v", cfg.DeadTime, DefaultDeadTime)
	}
	if cfg.DeadThreshold != DefaultDeadThreshold {
		t.Fatalf("dead_threshold: got %d, want %d", cfg.DeadThreshold, DefaultDeadThreshold)
	}
}

func TestConfigApplyDefaultsPreservesExisting(t *testing.T) {
	cfg := &Config{
		AuthPort:    1645,
		AcctPort:    1646,
		NASPortType: "Ethernet",
		Retries:     5,
	}
	cfg.applyDefaults()

	if cfg.AuthPort != 1645 {
		t.Fatalf("auth_port should be preserved: got %d", cfg.AuthPort)
	}
	if cfg.AcctPort != 1646 {
		t.Fatalf("acct_port should be preserved: got %d", cfg.AcctPort)
	}
	if cfg.NASPortType != "Ethernet" {
		t.Fatalf("nas_port_type should be preserved: got %q", cfg.NASPortType)
	}
	if cfg.Retries != 5 {
		t.Fatalf("retries should be preserved: got %d", cfg.Retries)
	}
}

func TestDeadServerDetection(t *testing.T) {
	rc := &radiusConn{}

	if rc.isDead(DefaultDeadTime) {
		t.Fatal("new conn should not be dead")
	}

	rc.recordFailure(3)
	if rc.isDead(DefaultDeadTime) {
		t.Fatal("1 failure should not mark dead (threshold=3)")
	}

	rc.recordFailure(3)
	if rc.isDead(DefaultDeadTime) {
		t.Fatal("2 failures should not mark dead (threshold=3)")
	}

	rc.recordFailure(3)
	if !rc.isDead(DefaultDeadTime) {
		t.Fatal("3 failures should mark dead")
	}

	rc.recordSuccess()
	if rc.isDead(DefaultDeadTime) {
		t.Fatal("success should clear dead state")
	}
}

// buildVSA constructs a Vendor-Specific Attribute payload.
func buildVSA(vendorID uint32, vendorType byte, data []byte) []byte {
	buf := make([]byte, 4+2+len(data))
	binary.BigEndian.PutUint32(buf[0:4], vendorID)
	buf[4] = vendorType
	buf[5] = byte(2 + len(data))
	copy(buf[6:], data)
	return buf
}

func mustCompileRegexp(pattern string) *regexp.Regexp {
	re, err := regexp.Compile(pattern)
	if err != nil {
		panic(err)
	}
	return re
}
