// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package autoconfig

import (
	"strings"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/operations"
)

func cfgWithGroups(groups map[string]*subscriber.SubscriberGroup) *config.Config {
	return &config.Config{
		SubscriberGroups: &subscriber.SubscriberGroupsConfig{Groups: groups},
	}
}

func deriveOrFail(t *testing.T, cfg *config.Config) []Change {
	t.Helper()
	changes, err := New(cfg, "eth1").DeriveConfig()
	if err != nil {
		t.Fatalf("DeriveConfig: %v", err)
	}
	return changes
}

func pathPresent(changes []Change, path string) bool {
	for _, c := range changes {
		if c.Path == path {
			return true
		}
	}
	return false
}

func pathCount(changes []Change, prefix string) int {
	n := 0
	for _, c := range changes {
		if strings.HasPrefix(c.Path, prefix) {
			n++
		}
	}
	return n
}

func TestDerive_IPoEEmitsPuntAndAccess(t *testing.T) {
	changes := deriveOrFail(t, cfgWithGroups(map[string]*subscriber.SubscriberGroup{
		"g": {
			AccessTypes: []subscriber.AccessType{subscriber.AccessTypeIPoE},
			VLANs: []subscriber.VLANRange{
				{SVLAN: "100", CVLAN: "any", Interface: "loop100", ParentInterface: "eth1"},
			},
		},
	}))
	for _, p := range []string{
		"_internal.punt.eth1.100.dhcpv4",
		"_internal.punt.eth1.100.dhcpv6",
		"_internal.punt.eth1.100.arp",
		"_internal.punt.eth1.100.ipv6nd",
		"_internal.access.eth1.100.ipoe-input",
	} {
		if !pathPresent(changes, p) {
			t.Errorf("expected path %s", p)
		}
	}
	if pathPresent(changes, "_internal.punt.eth1.100.pppoe") {
		t.Error("[ipoe] must not emit pppoe punt")
	}
}

func TestDerive_PPPoEEmitsPuntAndParentPromisc(t *testing.T) {
	changes := deriveOrFail(t, cfgWithGroups(map[string]*subscriber.SubscriberGroup{
		"g": {
			AccessTypes: []subscriber.AccessType{subscriber.AccessTypePPPoE},
			VLANs: []subscriber.VLANRange{
				{SVLAN: "100", CVLAN: "any", Interface: "loop100", ParentInterface: "eth1"},
			},
		},
	}))
	if !pathPresent(changes, "_internal.punt.eth1.100.pppoe") {
		t.Error("[pppoe] must emit pppoe punt")
	}
	if !pathPresent(changes, "_internal.access.eth1.promiscuous") {
		t.Error("[pppoe] must emit parent promiscuous")
	}
}

func TestDerive_MixedEmitsUnion(t *testing.T) {
	changes := deriveOrFail(t, cfgWithGroups(map[string]*subscriber.SubscriberGroup{
		"g": {
			AccessTypes: []subscriber.AccessType{subscriber.AccessTypeIPoE, subscriber.AccessTypePPPoE},
			VLANs: []subscriber.VLANRange{
				{SVLAN: "100", CVLAN: "any", Interface: "loop100", ParentInterface: "eth1"},
			},
		},
	}))
	for _, p := range []string{
		"_internal.punt.eth1.100.dhcpv4",
		"_internal.punt.eth1.100.pppoe",
		"_internal.access.eth1.100.ipoe-input",
		"_internal.access.eth1.promiscuous",
	} {
		if !pathPresent(changes, p) {
			t.Errorf("[ipoe, pppoe] missing %s", p)
		}
	}
}

func TestDerive_MixedReverseOrderEmitsSameUnion(t *testing.T) {
	changes := deriveOrFail(t, cfgWithGroups(map[string]*subscriber.SubscriberGroup{
		"g": {
			AccessTypes: []subscriber.AccessType{subscriber.AccessTypePPPoE, subscriber.AccessTypeIPoE},
			VLANs: []subscriber.VLANRange{
				{SVLAN: "100", CVLAN: "any", Interface: "loop100", ParentInterface: "eth1"},
			},
		},
	}))
	for _, p := range []string{
		"_internal.punt.eth1.100.dhcpv4",
		"_internal.punt.eth1.100.pppoe",
		"_internal.access.eth1.100.ipoe-input",
		"_internal.access.eth1.promiscuous",
	} {
		if !pathPresent(changes, p) {
			t.Errorf("[pppoe, ipoe] missing %s (order should not matter)", p)
		}
	}
}

func TestDerive_LNSEmitsL2TPPunt(t *testing.T) {
	changes := deriveOrFail(t, cfgWithGroups(map[string]*subscriber.SubscriberGroup{
		"lns": {
			AccessTypes: []subscriber.AccessType{subscriber.AccessTypeLNS},
			VLANs: []subscriber.VLANRange{
				{SVLAN: "1-4094", CVLAN: "any", Interface: "loop600", ParentInterface: "eth1"},
			},
		},
	}))
	if !pathPresent(changes, "_internal.punt.eth1.1.l2tp") {
		t.Error("[lns] must emit l2tp punt")
	}
	if pathPresent(changes, "_internal.access.eth1.promiscuous") {
		t.Error("[lns] must not emit parent promiscuous")
	}
}

func TestDerive_DedupesPromiscuousAcrossSVLANs(t *testing.T) {
	changes := deriveOrFail(t, cfgWithGroups(map[string]*subscriber.SubscriberGroup{
		"g": {
			AccessTypes: []subscriber.AccessType{subscriber.AccessTypePPPoE},
			VLANs: []subscriber.VLANRange{
				{SVLAN: "100-199", CVLAN: "any", Interface: "loop100", ParentInterface: "eth1"},
			},
		},
	}))
	if got := pathCount(changes, "_internal.access.eth1.promiscuous"); got != 1 {
		t.Errorf("expected 1 promiscuous emission, got %d", got)
	}
}

func TestDerive_AccessConfigValuesSensible(t *testing.T) {
	changes := deriveOrFail(t, cfgWithGroups(map[string]*subscriber.SubscriberGroup{
		"g": {
			AccessTypes: []subscriber.AccessType{subscriber.AccessTypePPPoE},
			VLANs: []subscriber.VLANRange{
				{SVLAN: "100", CVLAN: "any", Interface: "loop100", ParentInterface: "eth1"},
			},
		},
	}))
	for _, c := range changes {
		if c.Path == "_internal.access.eth1.promiscuous" {
			ac, ok := c.Value.(*operations.AccessConfig)
			if !ok || !ac.Enabled {
				t.Fatalf("promiscuous AccessConfig not enabled: %T %+v", c.Value, c.Value)
			}
			return
		}
	}
	t.Fatal("promiscuous path not found")
}
