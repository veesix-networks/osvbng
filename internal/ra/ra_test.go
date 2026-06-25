// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ra

import (
	"bytes"
	"net"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func TestLinkLocalFromMAC(t *testing.T) {
	got := LinkLocalFromMAC(net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff})
	want := net.ParseIP("fe80::a8bb:ccff:fedd:eeff")
	if !got.Equal(want) {
		t.Fatalf("LinkLocalFromMAC = %s, want %s", got, want)
	}
	if LinkLocalFromMAC(net.HardwareAddr{0x01, 0x02}) != nil {
		t.Fatal("short MAC should return nil")
	}
}

// TestTemplateMatchesFullBuild is the checksum-fold safety net: an RA template
// (dst + checksum zeroed) replicated and patched per destination must be
// byte-identical to a full serialize for that destination. Run for both the
// Ethernet (SLLAO) and point-to-point (no SLLAO) link types so the two callers
// cannot diverge.
func TestTemplateMatchesFullBuild(t *testing.T) {
	raConfig := southbound.IPv6RAConfig{Managed: true, Other: true, RouterLifetime: 1800}
	prefixes := []PrefixInfo{{Network: "2001:db8:0:1::/64", ValidTime: 7200, PreferredTime: 3600, OnLink: false}}
	srcMAC := net.HardwareAddr{0xaa, 0xc1, 0xab, 0x1f, 0xe2, 0xfa}
	srcIP := LinkLocalFromMAC(srcMAC)

	for _, slla := range []bool{true, false} {
		tmpl, err := BuildRARawData(raConfig, prefixes, srcMAC, srcIP, net.IPv6zero, slla, nil)
		if err != nil {
			t.Fatalf("BuildRARawData(template, sllao=%v): %v", slla, err)
		}
		tmpl[42], tmpl[43] = 0, 0

		for _, dst := range []net.IP{net.ParseIP("fe80::baad:f00d"), net.ParseIP("ff02::1")} {
			full, err := BuildRARawData(raConfig, prefixes, srcMAC, srcIP, dst, slla, nil)
			if err != nil {
				t.Fatalf("BuildRARawData(%s, sllao=%v): %v", dst, slla, err)
			}
			repl := make([]byte, len(tmpl))
			copy(repl, tmpl)
			copy(repl[24:40], dst.To16())
			PatchChecksum(repl)
			if !bytes.Equal(full, repl) {
				t.Fatalf("sllao=%v dst=%s: templated RA differs from full build\n full=%x\n repl=%x", slla, dst, full, repl)
			}
		}
	}
}

func TestRefreshIntervalClamps(t *testing.T) {
	if got := RefreshInterval(southbound.IPv6RAConfig{RouterLifetime: 0}); got != 0 {
		t.Fatalf("zero RouterLifetime must yield 0, got %s", got)
	}
	// MaxInterval capped at RouterLifetime/3.
	rc := southbound.IPv6RAConfig{RouterLifetime: 1800, MaxInterval: 600, MinInterval: 200}
	if got := RefreshInterval(rc).Seconds(); got != 600 {
		t.Fatalf("refresh = %vs, want 600", got)
	}
	rc = southbound.IPv6RAConfig{RouterLifetime: 900, MaxInterval: 600, MinInterval: 200}
	if got := RefreshInterval(rc).Seconds(); got != 300 {
		t.Fatalf("refresh = %vs, want 300 (lifetime/3)", got)
	}
}
