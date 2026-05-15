// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"strings"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
)

func cfgWithGroup(parent string, group *subscriber.SubscriberGroup) *Config {
	return &Config{
		SubscriberGroups: &subscriber.SubscriberGroupsConfig{
			Groups: map[string]*subscriber.SubscriberGroup{
				"g": group,
			},
		},
		Interfaces: map[string]*interfaces.InterfaceConfig{
			parent: {Name: parent, Enabled: true},
		},
	}
}

func TestValidateSubscriberAccessTypes_NilConfig(t *testing.T) {
	if err := ValidateSubscriberAccessTypes(nil); err != nil {
		t.Fatalf("expected nil for nil cfg, got %v", err)
	}
	if err := ValidateSubscriberAccessTypes(&Config{}); err != nil {
		t.Fatalf("expected nil for empty cfg, got %v", err)
	}
}

func TestValidateSubscriberAccessTypes_ValidSingleElement(t *testing.T) {
	cases := []string{"ipoe", "pppoe", "lac", "lns"}
	for _, at := range cases {
		t.Run(at, func(t *testing.T) {
			cfg := cfgWithGroup("eth1", &subscriber.SubscriberGroup{
				AccessTypes: []string{at},
				VLANs: []subscriber.VLANRange{
					{SVLAN: "100", CVLAN: "any", ParentInterface: "eth1"},
				},
			})
			if err := ValidateSubscriberAccessTypes(cfg); err != nil {
				t.Fatalf("expected accept for [%s], got %v", at, err)
			}
		})
	}
}

func TestValidateSubscriberAccessTypes_ValidMixed(t *testing.T) {
	cfg := cfgWithGroup("eth1", &subscriber.SubscriberGroup{
		AccessTypes: []string{"ipoe", "pppoe"},
		VLANs: []subscriber.VLANRange{
			{SVLAN: "100-299", CVLAN: "any", ParentInterface: "eth1"},
		},
	})
	if err := ValidateSubscriberAccessTypes(cfg); err != nil {
		t.Fatalf("expected accept for [ipoe, pppoe], got %v", err)
	}
}

func TestValidateSubscriberAccessTypes_Empty(t *testing.T) {
	cfg := cfgWithGroup("eth1", &subscriber.SubscriberGroup{
		AccessTypes: []string{},
		VLANs: []subscriber.VLANRange{
			{SVLAN: "100", CVLAN: "any", ParentInterface: "eth1"},
		},
	})
	if err := ValidateSubscriberAccessTypes(cfg); err == nil || !strings.Contains(err.Error(), "access-types is empty") {
		t.Fatalf("expected empty-access-types error, got %v", err)
	}
}

func TestValidateSubscriberAccessTypes_Unknown(t *testing.T) {
	cfg := cfgWithGroup("eth1", &subscriber.SubscriberGroup{
		AccessTypes: []string{"bogus"},
		VLANs: []subscriber.VLANRange{
			{SVLAN: "100", CVLAN: "any", ParentInterface: "eth1"},
		},
	})
	if err := ValidateSubscriberAccessTypes(cfg); err == nil || !strings.Contains(err.Error(), "not one of") {
		t.Fatalf("expected unknown-access-type error, got %v", err)
	}
}

func TestValidateSubscriberAccessTypes_Duplicate(t *testing.T) {
	cfg := cfgWithGroup("eth1", &subscriber.SubscriberGroup{
		AccessTypes: []string{"ipoe", "ipoe"},
		VLANs: []subscriber.VLANRange{
			{SVLAN: "100", CVLAN: "any", ParentInterface: "eth1"},
		},
	})
	if err := ValidateSubscriberAccessTypes(cfg); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestValidateSubscriberAccessTypes_InvalidCombinations(t *testing.T) {
	cases := []struct {
		name string
		ats  []string
	}{
		{"lac_plus_ipoe", []string{"lac", "ipoe"}},
		{"lac_plus_pppoe", []string{"lac", "pppoe"}},
		{"lac_plus_lns", []string{"lac", "lns"}},
		{"lns_plus_ipoe", []string{"lns", "ipoe"}},
		{"lns_plus_pppoe", []string{"lns", "pppoe"}},
		{"three_way", []string{"ipoe", "pppoe", "lac"}},
		{"three_way_lns", []string{"ipoe", "pppoe", "lns"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := cfgWithGroup("eth1", &subscriber.SubscriberGroup{
				AccessTypes: c.ats,
				VLANs: []subscriber.VLANRange{
					{SVLAN: "100", CVLAN: "any", ParentInterface: "eth1"},
				},
			})
			if err := ValidateSubscriberAccessTypes(cfg); err == nil {
				t.Fatalf("expected rejection for %v, got nil", c.ats)
			}
		})
	}
}

func TestValidateSubscriberAccessTypes_ParentInterfaceRequired(t *testing.T) {
	cfg := cfgWithGroup("eth1", &subscriber.SubscriberGroup{
		AccessTypes: []string{"ipoe"},
		VLANs: []subscriber.VLANRange{
			{SVLAN: "100", CVLAN: "any"},
		},
	})
	if err := ValidateSubscriberAccessTypes(cfg); err == nil || !strings.Contains(err.Error(), "parent-interface is required") {
		t.Fatalf("expected parent-interface-required error, got %v", err)
	}
}

func TestValidateSubscriberAccessTypes_ParentInterfaceMissing(t *testing.T) {
	cfg := cfgWithGroup("eth1", &subscriber.SubscriberGroup{
		AccessTypes: []string{"ipoe"},
		VLANs: []subscriber.VLANRange{
			{SVLAN: "100", CVLAN: "any", ParentInterface: "ghost0"},
		},
	})
	if err := ValidateSubscriberAccessTypes(cfg); err == nil || !strings.Contains(err.Error(), "not defined in interfaces") {
		t.Fatalf("expected missing-interface error, got %v", err)
	}
}

func TestValidateSubscriberAccessTypes_ParentInterfaceOptionalForLNS(t *testing.T) {
	cfg := cfgWithGroup("eth1", &subscriber.SubscriberGroup{
		AccessTypes: []string{"lns"},
		VLANs: []subscriber.VLANRange{
			{SVLAN: "100", CVLAN: "any"},
		},
	})
	if err := ValidateSubscriberAccessTypes(cfg); err != nil {
		t.Fatalf("expected LNS to allow missing parent-interface, got %v", err)
	}
}

func TestValidateSubscriberAccessTypes_MultipleParentInterfaces(t *testing.T) {
	cfg := &Config{
		SubscriberGroups: &subscriber.SubscriberGroupsConfig{
			Groups: map[string]*subscriber.SubscriberGroup{
				"g1": {
					AccessTypes: []string{"ipoe"},
					VLANs: []subscriber.VLANRange{
						{SVLAN: "100", CVLAN: "any", ParentInterface: "eth1"},
					},
				},
				"g2": {
					AccessTypes: []string{"pppoe"},
					VLANs: []subscriber.VLANRange{
						{SVLAN: "200", CVLAN: "any", ParentInterface: "eth2"},
					},
				},
			},
		},
		Interfaces: map[string]*interfaces.InterfaceConfig{
			"eth1": {Name: "eth1", Enabled: true},
			"eth2": {Name: "eth2", Enabled: true},
		},
	}
	if err := ValidateSubscriberAccessTypes(cfg); err == nil || !strings.Contains(err.Error(), "multiple parent-interfaces") {
		t.Fatalf("expected multi-parent-interface error, got %v", err)
	}
}
