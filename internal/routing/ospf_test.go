// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package routing

import "testing"

func TestOSPFVRFPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		vrf     string
		want    string
		wantErr bool
	}{
		{"empty means default RT", "", "", false},
		{"vrf all", "all", "vrf all ", false},
		{"simple name", "mgmt", "vrf mgmt ", false},
		{"mixed case + digits + dash + underscore", "VRF_1-foo", "vrf VRF_1-foo ", false},
		{"trailing space rejected", "mgmt ", "", true},
		{"leading space rejected", " mgmt", "", true},
		{"shell-metachar semicolon rejected", "foo; rm -rf /", "", true},
		{"shell-metachar backtick rejected", "foo`whoami`", "", true},
		{"shell-metachar dollar-paren rejected", "$(id)", "", true},
		{"pipe rejected", "foo|bar", "", true},
		{"dot rejected", "foo.bar", "", true},
		{"slash rejected", "foo/bar", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ospfVRFPrefix(tt.vrf)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ospfVRFPrefix(%q) err=%v, wantErr=%v", tt.vrf, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ospfVRFPrefix(%q) = %q, want %q", tt.vrf, got, tt.want)
			}
		})
	}
}

func TestOSPFValidateIPv4(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"router-id form", "10.254.0.1", false},
		{"loopback", "127.0.0.1", false},
		{"zero", "0.0.0.0", false},
		{"five octets", "10.254.0.1.5", true},
		{"three octets", "10.254.0", true},
		{"ipv6 rejected", "::1", true},
		{"ipv4-mapped ipv6 accepted as ipv4 by net.ParseIP", "::ffff:10.0.0.1", false},
		{"garbage", "abc", true},
		{"empty", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ospfValidateIPv4("test", tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ospfValidateIPv4(%q) err=%v, wantErr=%v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestValidOSPFLSATypes(t *testing.T) {
	t.Parallel()
	want := []string{
		"router", "network", "summary", "asbr-summary",
		"external", "nssa-external",
		"opaque-link", "opaque-area", "opaque-as",
	}
	if len(validOSPFLSATypes) != len(want) {
		t.Errorf("validOSPFLSATypes has %d entries, want %d", len(validOSPFLSATypes), len(want))
	}
	for _, name := range want {
		if _, ok := validOSPFLSATypes[name]; !ok {
			t.Errorf("validOSPFLSATypes missing %q", name)
		}
	}
	for _, bad := range []string{"", "Router", "router-lsa", "foo", "external-lsa"} {
		if _, ok := validOSPFLSATypes[bad]; ok {
			t.Errorf("validOSPFLSATypes unexpectedly contains %q", bad)
		}
	}
}

func TestValidMPLSTEDatabaseScopes(t *testing.T) {
	t.Parallel()
	want := []string{"vertex", "edge", "subnet"}
	if len(validMPLSTEDatabaseScopes) != len(want) {
		t.Errorf("validMPLSTEDatabaseScopes has %d entries, want %d", len(validMPLSTEDatabaseScopes), len(want))
	}
	for _, name := range want {
		if _, ok := validMPLSTEDatabaseScopes[name]; !ok {
			t.Errorf("validMPLSTEDatabaseScopes missing %q", name)
		}
	}
	for _, bad := range []string{"", "all", "router", "vertices"} {
		if _, ok := validMPLSTEDatabaseScopes[bad]; ok {
			t.Errorf("validMPLSTEDatabaseScopes unexpectedly contains %q", bad)
		}
	}
}
