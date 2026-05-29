// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package subscriber

import "testing"

func groups(g map[string]*SubscriberGroup) *SubscriberGroupsConfig {
	return &SubscriberGroupsConfig{Groups: g}
}

func TestMatchIndexLookup(t *testing.T) {
	cfg := groups(map[string]*SubscriberGroup{
		"residential": {VLANs: []VLANRange{{SVLAN: "100-200", CVLAN: "10"}}},
		"business":    {VLANs: []VLANRange{{SVLAN: "100-200", CVLAN: "any"}}},
	})
	idx := BuildMatchIndex(cfg)

	if m, ok := idx.Lookup(150, 10); !ok || m.Name != "residential" {
		t.Fatalf("svlan 150 cvlan 10: got %q ok=%v, want residential", m.Name, ok)
	}
	if m, ok := idx.Lookup(150, 999); !ok || m.Name != "business" {
		t.Fatalf("svlan 150 cvlan 999: got %q ok=%v, want business (any)", m.Name, ok)
	}
	if m, ok := idx.Lookup(150, 0); !ok || m.Name != "business" {
		t.Fatalf("svlan 150 cvlan 0: got %q ok=%v, want business (any only)", m.Name, ok)
	}
	if _, ok := idx.Lookup(300, 10); ok {
		t.Fatalf("svlan 300: expected miss")
	}
}

func TestMatchIndexNoWildcardMiss(t *testing.T) {
	idx := BuildMatchIndex(groups(map[string]*SubscriberGroup{
		"residential": {VLANs: []VLANRange{{SVLAN: "100", CVLAN: "10"}}},
	}))
	if _, ok := idx.Lookup(100, 20); ok {
		t.Fatalf("svlan 100 cvlan 20 with no wildcard: expected miss")
	}
	if _, ok := idx.Lookup(100, 0); ok {
		t.Fatalf("svlan 100 cvlan 0 with no wildcard: expected miss (0 matches only any)")
	}
}

func TestValidateMatchIndex(t *testing.T) {
	dupSpecific := groups(map[string]*SubscriberGroup{
		"a": {VLANs: []VLANRange{{SVLAN: "100", CVLAN: "10"}}},
		"b": {VLANs: []VLANRange{{SVLAN: "100", CVLAN: "10"}}},
	})
	if err := ValidateMatchIndex(dupSpecific); err == nil {
		t.Errorf("duplicate specific cvlan: expected collision error")
	}

	dupAny := groups(map[string]*SubscriberGroup{
		"a": {VLANs: []VLANRange{{SVLAN: "100", CVLAN: "any"}}},
		"b": {VLANs: []VLANRange{{SVLAN: "100"}}},
	})
	if err := ValidateMatchIndex(dupAny); err == nil {
		t.Errorf("duplicate wildcard (any + omitted): expected collision error")
	}

	specificPlusAny := groups(map[string]*SubscriberGroup{
		"a": {VLANs: []VLANRange{{SVLAN: "100", CVLAN: "10"}}},
		"b": {VLANs: []VLANRange{{SVLAN: "100", CVLAN: "any"}}},
	})
	if err := ValidateMatchIndex(specificPlusAny); err != nil {
		t.Errorf("specific + wildcard should be allowed, got: %v", err)
	}
}
