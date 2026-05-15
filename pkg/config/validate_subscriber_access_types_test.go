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
	cases := []subscriber.AccessType{
		subscriber.AccessTypeIPoE,
		subscriber.AccessTypePPPoE,
		subscriber.AccessTypeLAC,
		subscriber.AccessTypeLNS,
	}
	for _, at := range cases {
		t.Run(string(at), func(t *testing.T) {
			cfg := cfgWithGroup("eth1", &subscriber.SubscriberGroup{
				VLANs: []subscriber.VLANRange{
					{SVLAN: "100", CVLAN: "any", ParentInterface: "eth1", AccessTypes: []subscriber.AccessType{at}},
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
		VLANs: []subscriber.VLANRange{
			{SVLAN: "100-299", CVLAN: "any", ParentInterface: "eth1", AccessTypes: []subscriber.AccessType{subscriber.AccessTypeIPoE, subscriber.AccessTypePPPoE}},
		},
	})
	if err := ValidateSubscriberAccessTypes(cfg); err != nil {
		t.Fatalf("expected accept for [ipoe, pppoe], got %v", err)
	}
}

func TestValidateSubscriberAccessTypes_Empty(t *testing.T) {
	cfg := cfgWithGroup("eth1", &subscriber.SubscriberGroup{
		VLANs: []subscriber.VLANRange{
			{SVLAN: "100", CVLAN: "any", ParentInterface: "eth1", AccessTypes: []subscriber.AccessType{}},
		},
	})
	if err := ValidateSubscriberAccessTypes(cfg); err == nil || !strings.Contains(err.Error(), "access-types is empty") {
		t.Fatalf("expected empty-access-types error, got %v", err)
	}
}

func TestValidateSubscriberAccessTypes_Unknown(t *testing.T) {
	cfg := cfgWithGroup("eth1", &subscriber.SubscriberGroup{
		VLANs: []subscriber.VLANRange{
			{SVLAN: "100", CVLAN: "any", ParentInterface: "eth1", AccessTypes: []subscriber.AccessType{"bogus"}},
		},
	})
	if err := ValidateSubscriberAccessTypes(cfg); err == nil || !strings.Contains(err.Error(), "not one of") {
		t.Fatalf("expected unknown-access-type error, got %v", err)
	}
}

func TestValidateSubscriberAccessTypes_Duplicate(t *testing.T) {
	cfg := cfgWithGroup("eth1", &subscriber.SubscriberGroup{
		VLANs: []subscriber.VLANRange{
			{SVLAN: "100", CVLAN: "any", ParentInterface: "eth1", AccessTypes: []subscriber.AccessType{subscriber.AccessTypeIPoE, subscriber.AccessTypeIPoE}},
		},
	})
	if err := ValidateSubscriberAccessTypes(cfg); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestValidateSubscriberAccessTypes_InvalidCombinations(t *testing.T) {
	cases := []struct {
		name string
		ats  []subscriber.AccessType
	}{
		{"lac_plus_ipoe", []subscriber.AccessType{subscriber.AccessTypeLAC, subscriber.AccessTypeIPoE}},
		{"lac_plus_pppoe", []subscriber.AccessType{subscriber.AccessTypeLAC, subscriber.AccessTypePPPoE}},
		{"lac_plus_lns", []subscriber.AccessType{subscriber.AccessTypeLAC, subscriber.AccessTypeLNS}},
		{"lns_plus_ipoe", []subscriber.AccessType{subscriber.AccessTypeLNS, subscriber.AccessTypeIPoE}},
		{"lns_plus_pppoe", []subscriber.AccessType{subscriber.AccessTypeLNS, subscriber.AccessTypePPPoE}},
		{"three_way", []subscriber.AccessType{subscriber.AccessTypeIPoE, subscriber.AccessTypePPPoE, subscriber.AccessTypeLAC}},
		{"three_way_lns", []subscriber.AccessType{subscriber.AccessTypeIPoE, subscriber.AccessTypePPPoE, subscriber.AccessTypeLNS}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := cfgWithGroup("eth1", &subscriber.SubscriberGroup{
				VLANs: []subscriber.VLANRange{
					{SVLAN: "100", CVLAN: "any", ParentInterface: "eth1", AccessTypes: c.ats},
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
		VLANs: []subscriber.VLANRange{
			{SVLAN: "100", CVLAN: "any", AccessTypes: []subscriber.AccessType{subscriber.AccessTypeIPoE}},
		},
	})
	if err := ValidateSubscriberAccessTypes(cfg); err == nil || !strings.Contains(err.Error(), "parent-interface is required") {
		t.Fatalf("expected parent-interface-required error, got %v", err)
	}
}

func TestValidateSubscriberAccessTypes_ParentInterfaceMissing(t *testing.T) {
	cfg := cfgWithGroup("eth1", &subscriber.SubscriberGroup{
		VLANs: []subscriber.VLANRange{
			{SVLAN: "100", CVLAN: "any", ParentInterface: "ghost0", AccessTypes: []subscriber.AccessType{subscriber.AccessTypeIPoE}},
		},
	})
	if err := ValidateSubscriberAccessTypes(cfg); err == nil || !strings.Contains(err.Error(), "not defined in interfaces") {
		t.Fatalf("expected missing-interface error, got %v", err)
	}
}

func TestValidateSubscriberAccessTypes_ParentInterfaceOptionalForLNS(t *testing.T) {
	cfg := cfgWithGroup("eth1", &subscriber.SubscriberGroup{
		VLANs: []subscriber.VLANRange{
			{SVLAN: "100", CVLAN: "any", AccessTypes: []subscriber.AccessType{subscriber.AccessTypeLNS}},
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
					VLANs: []subscriber.VLANRange{
						{SVLAN: "100", CVLAN: "any", ParentInterface: "eth1", AccessTypes: []subscriber.AccessType{subscriber.AccessTypeIPoE}},
					},
				},
				"g2": {
					VLANs: []subscriber.VLANRange{
						{SVLAN: "200", CVLAN: "any", ParentInterface: "eth2", AccessTypes: []subscriber.AccessType{subscriber.AccessTypePPPoE}},
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
