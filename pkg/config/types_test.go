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

func TestGetAccessInterface_FromParentInterface(t *testing.T) {
	cfg := &Config{
		SubscriberGroups: &subscriber.SubscriberGroupsConfig{
			Groups: map[string]*subscriber.SubscriberGroup{
				"residential": {
					AccessTypes: []subscriber.AccessType{subscriber.AccessTypeIPoE, subscriber.AccessTypePPPoE},
					VLANs: []subscriber.VLANRange{
						{SVLAN: "100-299", CVLAN: "any", ParentInterface: "eth1"},
					},
				},
			},
		},
		Interfaces: map[string]*interfaces.InterfaceConfig{
			"eth1": {Name: "eth1", Enabled: true},
		},
	}
	got, err := cfg.GetAccessInterface()
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if got != "eth1" {
		t.Fatalf("want eth1, got %q", got)
	}
}

func TestGetAccessInterface_NoSubscriberGroup(t *testing.T) {
	cfg := &Config{}
	if _, err := cfg.GetAccessInterface(); err == nil {
		t.Fatal("expected error when no subscriber-groups configured")
	}
}

func TestGetAccessInterface_LNSOnlyReturnsNoInterface(t *testing.T) {
	cfg := &Config{
		SubscriberGroups: &subscriber.SubscriberGroupsConfig{
			Groups: map[string]*subscriber.SubscriberGroup{
				"lns": {
					AccessTypes: []subscriber.AccessType{subscriber.AccessTypeLNS},
					VLANs: []subscriber.VLANRange{
						{SVLAN: "100", CVLAN: "any", ParentInterface: "eth1"},
					},
				},
			},
		},
	}
	_, err := cfg.GetAccessInterface()
	if err == nil || !strings.Contains(err.Error(), "parent-interface") {
		t.Fatalf("expected no-access-interface for LNS-only, got %v", err)
	}
}

func TestGetAccessInterface_MultipleParents(t *testing.T) {
	cfg := &Config{
		SubscriberGroups: &subscriber.SubscriberGroupsConfig{
			Groups: map[string]*subscriber.SubscriberGroup{
				"g1": {
					AccessTypes: []subscriber.AccessType{subscriber.AccessTypeIPoE},
					VLANs: []subscriber.VLANRange{
						{SVLAN: "100", CVLAN: "any", ParentInterface: "eth1"},
					},
				},
				"g2": {
					AccessTypes: []subscriber.AccessType{subscriber.AccessTypePPPoE},
					VLANs: []subscriber.VLANRange{
						{SVLAN: "200", CVLAN: "any", ParentInterface: "eth2"},
					},
				},
			},
		},
	}
	_, err := cfg.GetAccessInterface()
	if err == nil || !strings.Contains(err.Error(), "multiple") {
		t.Fatalf("expected multi-parent error, got %v", err)
	}
}

func TestNeedsAccessInterface(t *testing.T) {
	cases := []struct {
		name string
		ats  []subscriber.AccessType
		want bool
	}{
		{"ipoe", []subscriber.AccessType{subscriber.AccessTypeIPoE}, true},
		{"pppoe", []subscriber.AccessType{subscriber.AccessTypePPPoE}, true},
		{"mixed", []subscriber.AccessType{subscriber.AccessTypeIPoE, subscriber.AccessTypePPPoE}, true},
		{"lac", []subscriber.AccessType{subscriber.AccessTypeLAC}, true},
		{"lns", []subscriber.AccessType{subscriber.AccessTypeLNS}, false},
		{"empty", []subscriber.AccessType{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := &Config{
				SubscriberGroups: &subscriber.SubscriberGroupsConfig{
					Groups: map[string]*subscriber.SubscriberGroup{
						"g": {AccessTypes: c.ats},
					},
				},
			}
			if got := cfg.NeedsAccessInterface(); got != c.want {
				t.Fatalf("NeedsAccessInterface for %v: got %v, want %v", c.ats, got, c.want)
			}
		})
	}
}
