// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package netbind

import (
	"net/netip"
	"strings"
	"testing"
)

func vrfLookup(vrfs map[string]VRFInfo) VRFLookup {
	return func(name string) (VRFInfo, bool) {
		v, ok := vrfs[name]
		return v, ok
	}
}

func TestBinding_Validate_Empty(t *testing.T) {
	if err := (Binding{}).Validate(FamilyV4, nil); err != nil {
		t.Fatalf("empty binding should validate: %v", err)
	}
}

func TestBinding_Validate_VRFNotDeclared(t *testing.T) {
	err := Binding{VRF: "MGMT"}.Validate(FamilyV4, vrfLookup(nil))
	if err == nil || !strings.Contains(err.Error(), "not declared") {
		t.Fatalf("want vrf-not-declared error, got %v", err)
	}
}

func TestBinding_Validate_VRFFamilyMismatch(t *testing.T) {
	v := vrfLookup(map[string]VRFInfo{"MGMT": {IPv4: true}})
	err := Binding{VRF: "MGMT"}.Validate(FamilyV6, v)
	if err == nil || !strings.Contains(err.Error(), "ipv6") {
		t.Fatalf("want ipv6-not-enabled error, got %v", err)
	}
}

func TestBinding_Validate_SourceIPFamilyMismatch(t *testing.T) {
	err := Binding{SourceIP: netip.MustParseAddr("2001:db8::1")}.Validate(FamilyV4, nil)
	if err == nil || !strings.Contains(err.Error(), "IPv6") {
		t.Fatalf("want family mismatch error, got %v", err)
	}
}

func TestBinding_Validate_SourceIPWithoutVRF(t *testing.T) {
	if err := (Binding{SourceIP: netip.MustParseAddr("10.0.0.1")}).Validate(FamilyV4, nil); err != nil {
		t.Fatalf("source IP without VRF should pass: %v", err)
	}
}

func TestBinding_Validate_VRFOnly_OK(t *testing.T) {
	v := vrfLookup(map[string]VRFInfo{"MGMT": {IPv4: true, IPv6: true}})
	if err := (Binding{VRF: "MGMT"}).Validate(FamilyV4, v); err != nil {
		t.Errorf("v4: %v", err)
	}
	if err := (Binding{VRF: "MGMT"}).Validate(FamilyV6, v); err != nil {
		t.Errorf("v6: %v", err)
	}
}
