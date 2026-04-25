// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"strings"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/config/ip"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
)

func cfgWithSubscriberVRFInputs() *Config {
	return &Config{
		VRFS: map[string]*ip.VRFSConfig{
			"CUSTOMER-A": {},
			"CUSTOMER-B": {},
		},
		Interfaces: map[string]*interfaces.InterfaceConfig{
			"loop100": {Name: "loop100"},
			"loop101": {Name: "loop101", VRF: "CUSTOMER-A"},
			"loop102": {Name: "loop102", VRF: "CUSTOMER-B"},
		},
		SubscriberGroups: &subscriber.SubscriberGroupsConfig{
			Groups: map[string]*subscriber.SubscriberGroup{},
		},
	}
}

func TestValidateSubscriberGroupVRF_PositiveCases(t *testing.T) {
	tests := []struct {
		name  string
		group *subscriber.SubscriberGroup
	}{
		{
			name: "default vrf",
			group: &subscriber.SubscriberGroup{
				VLANs: []subscriber.VLANRange{
					{SVLAN: "100", Interface: "loop100"},
				},
			},
		},
		{
			name: "group vrf matches loopback",
			group: &subscriber.SubscriberGroup{
				VRF: "CUSTOMER-A",
				VLANs: []subscriber.VLANRange{
					{SVLAN: "200", Interface: "loop101"},
				},
			},
		},
		{
			name: "range vrf overrides group vrf",
			group: &subscriber.SubscriberGroup{
				VRF: "CUSTOMER-A",
				VLANs: []subscriber.VLANRange{
					{SVLAN: "300", Interface: "loop102", VRF: "CUSTOMER-B"},
				},
			},
		},
		{
			name: "mixed ranges",
			group: &subscriber.SubscriberGroup{
				VLANs: []subscriber.VLANRange{
					{SVLAN: "100-104", Interface: "loop100"},
					{SVLAN: "200-204", Interface: "loop101", VRF: "CUSTOMER-A"},
					{SVLAN: "300-304", Interface: "loop102", VRF: "CUSTOMER-B"},
				},
			},
		},
		{
			name: "range default sentinel overrides group vrf",
			group: &subscriber.SubscriberGroup{
				VRF: "CUSTOMER-A",
				VLANs: []subscriber.VLANRange{
					{SVLAN: "900", Interface: "loop100", VRF: "default"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := cfgWithSubscriberVRFInputs()
			cfg.SubscriberGroups.Groups["g1"] = tt.group

			if err := ValidateSubscriberGroupVRF(cfg); err != nil {
				t.Fatalf("expected pass, got %v", err)
			}
		})
	}
}

func TestValidateSubscriberGroupVRF_NegativeCases(t *testing.T) {
	tests := []struct {
		name      string
		group     *subscriber.SubscriberGroup
		wantError string
	}{
		{
			name: "missing loopback reference",
			group: &subscriber.SubscriberGroup{
				VLANs: []subscriber.VLANRange{
					{SVLAN: "100", Interface: "loop999"},
				},
			},
			wantError: `subscriber group "g1" vlan "100": interface "loop999" is not declared under interfaces:`,
		},
		{
			name: "missing vrf declaration",
			group: &subscriber.SubscriberGroup{
				VLANs: []subscriber.VLANRange{
					{SVLAN: "200", Interface: "loop101"},
				},
			},
			wantError: `subscriber group "g1" vlan "200": interface "loop101" is in VRF "CUSTOMER-A", but neither range nor group declares vrf (use vrf: "default" to explicitly opt out)`,
		},
		{
			name: "vrf mismatch",
			group: &subscriber.SubscriberGroup{
				VRF: "CUSTOMER-A",
				VLANs: []subscriber.VLANRange{
					{SVLAN: "300", Interface: "loop102"},
				},
			},
			wantError: `subscriber group "g1" vlan "300": resolved vrf "CUSTOMER-A" does not match interface "loop102" vrf "CUSTOMER-B"`,
		},
		{
			name: "undeclared vrf",
			group: &subscriber.SubscriberGroup{
				VRF: "MISSING",
				VLANs: []subscriber.VLANRange{
					{SVLAN: "400", Interface: "loop101"},
				},
			},
			wantError: `subscriber group "g1" vlan "400": vrf "MISSING" is not declared under vrfs:`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := cfgWithSubscriberVRFInputs()
			cfg.SubscriberGroups.Groups["g1"] = tt.group

			err := ValidateSubscriberGroupVRF(cfg)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("expected error containing %q, got %q", tt.wantError, err.Error())
			}
		})
	}
}
