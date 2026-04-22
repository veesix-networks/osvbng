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

func u16Ptr(v uint16) *uint16 { return &v }
func boolPtr(b bool) *bool    { return &b }

func cfgWithAccessIface(name string, mtu int) *Config {
	return &Config{
		Interfaces: map[string]*interfaces.InterfaceConfig{
			name: {Name: name, BNGMode: "access", MTU: mtu, Enabled: true},
		},
		SubscriberGroups: &subscriber.SubscriberGroupsConfig{
			Groups: map[string]*subscriber.SubscriberGroup{},
		},
	}
}

func TestValidateMSSClampParentMTU_NoSubscriberGroups(t *testing.T) {
	if err := ValidateMSSClampParentMTU(&Config{}); err != nil {
		t.Fatalf("nil subscriber groups should be a no-op, got %v", err)
	}
}

func TestValidateMSSClampParentMTU_PPPoEDot1QBabyGiantsPasses(t *testing.T) {
	cfg := cfgWithAccessIface("eth1", 1512)
	cfg.SubscriberGroups.Groups["g1"] = &subscriber.SubscriberGroup{
		AccessType: "pppoe",
		VLANs: []subscriber.VLANRange{
			{SVLAN: "100", CVLAN: "any", Interface: "loop100"},
		},
		PPPoE: &subscriber.PPPoEConfig{MRU: u16Ptr(1500)},
	}

	if err := ValidateMSSClampParentMTU(cfg); err != nil {
		t.Fatalf("expected pass, got %v", err)
	}
}

func TestValidateMSSClampParentMTU_PPPoEDot1QBabyGiantsFailsParentTooSmall(t *testing.T) {
	cfg := cfgWithAccessIface("eth1", 1500)
	cfg.SubscriberGroups.Groups["g1"] = &subscriber.SubscriberGroup{
		AccessType: "pppoe",
		VLANs: []subscriber.VLANRange{
			{SVLAN: "100", CVLAN: "any", Interface: "loop100"},
		},
		PPPoE: &subscriber.PPPoEConfig{MRU: u16Ptr(1500)},
	}

	err := ValidateMSSClampParentMTU(cfg)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	for _, want := range []string{`"g1"`, `"eth1"`, "1512", "1500"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got %q", want, err.Error())
		}
	}
}

func TestValidateMSSClampParentMTU_PPPoEQinQBabyGiantsRequiresLargerParent(t *testing.T) {
	cfg := cfgWithAccessIface("eth1", 1512)
	cfg.SubscriberGroups.Groups["g1"] = &subscriber.SubscriberGroup{
		AccessType: "pppoe",
		VLANs: []subscriber.VLANRange{
			{SVLAN: "100", CVLAN: "200", Interface: "loop100"},
		},
		PPPoE: &subscriber.PPPoEConfig{MRU: u16Ptr(1500)},
	}

	err := ValidateMSSClampParentMTU(cfg)
	if err == nil {
		t.Fatal("QinQ + baby giants should require parent mtu >= 1516; got nil")
	}
	if !strings.Contains(err.Error(), "1516") {
		t.Errorf("error should mention 1516, got %q", err.Error())
	}

	cfg.Interfaces["eth1"].MTU = 1516
	if err := ValidateMSSClampParentMTU(cfg); err != nil {
		t.Fatalf("with parent 1516 should pass, got %v", err)
	}
}

func TestValidateMSSClampParentMTU_BabyGiantsOnNonPPPoERejected(t *testing.T) {
	cfg := cfgWithAccessIface("eth1", 1512)
	cfg.SubscriberGroups.Groups["g1"] = &subscriber.SubscriberGroup{
		AccessType: "ipoe",
		VLANs: []subscriber.VLANRange{
			{SVLAN: "100", CVLAN: "any", Interface: "loop100"},
		},
		PPPoE: &subscriber.PPPoEConfig{MRU: u16Ptr(1500)},
	}

	err := ValidateMSSClampParentMTU(cfg)
	if err == nil || !strings.Contains(err.Error(), "PPPoE") {
		t.Fatalf("expected PPPoE-only error, got %v", err)
	}
}

func TestValidateMSSClampParentMTU_AbsentBlockNoValidation(t *testing.T) {
	cfg := cfgWithAccessIface("eth1", 1500)
	cfg.SubscriberGroups.Groups["g1"] = &subscriber.SubscriberGroup{
		AccessType: "pppoe",
		VLANs: []subscriber.VLANRange{
			{SVLAN: "100", CVLAN: "any", Interface: "loop100"},
		},
	}

	if err := ValidateMSSClampParentMTU(cfg); err != nil {
		t.Fatalf("absent pppoe block should not trigger validation, got %v", err)
	}
}

func TestValidateMSSClampParentMTU_MRU1492NoValidation(t *testing.T) {
	cfg := cfgWithAccessIface("eth1", 1500)
	cfg.SubscriberGroups.Groups["g1"] = &subscriber.SubscriberGroup{
		AccessType: "pppoe",
		VLANs: []subscriber.VLANRange{
			{SVLAN: "100", CVLAN: "any", Interface: "loop100"},
		},
		PPPoE: &subscriber.PPPoEConfig{MRU: u16Ptr(1492)},
	}

	if err := ValidateMSSClampParentMTU(cfg); err != nil {
		t.Fatalf("pppoe.mru=1492 means legacy semantics, no validation needed, got %v", err)
	}
}

func TestValidateMSSClampParentMTU_DefaultParentMTUAssumed1500(t *testing.T) {
	cfg := cfgWithAccessIface("eth1", 0)
	cfg.SubscriberGroups.Groups["g1"] = &subscriber.SubscriberGroup{
		AccessType: "pppoe",
		VLANs: []subscriber.VLANRange{
			{SVLAN: "100", CVLAN: "any", Interface: "loop100"},
		},
		PPPoE: &subscriber.PPPoEConfig{MRU: u16Ptr(1500)},
	}

	err := ValidateMSSClampParentMTU(cfg)
	if err == nil {
		t.Fatal("parent mtu unset is assumed 1500 and should fail baby-giants validation, got nil")
	}
	if !strings.Contains(err.Error(), "current: 1500") {
		t.Errorf("error should mention current: 1500, got %q", err.Error())
	}
}
