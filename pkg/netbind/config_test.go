// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package netbind

import (
	"strings"
	"testing"
)

func TestEndpointBinding_IsZero(t *testing.T) {
	if !(EndpointBinding{}).IsZero() {
		t.Fatal("empty EndpointBinding should be zero")
	}
	if (EndpointBinding{VRF: "x"}).IsZero() {
		t.Fatal("VRF=x should not be zero")
	}
}

func TestEndpointBinding_MergeWith(t *testing.T) {
	cases := []struct {
		name   string
		child  EndpointBinding
		parent EndpointBinding
		want   EndpointBinding
	}{
		{
			"both empty",
			EndpointBinding{},
			EndpointBinding{},
			EndpointBinding{},
		},
		{
			"child empty inherits parent",
			EndpointBinding{},
			EndpointBinding{VRF: "MGMT", SourceIP: "10.0.0.1"},
			EndpointBinding{VRF: "MGMT", SourceIP: "10.0.0.1"},
		},
		{
			"child overrides VRF only",
			EndpointBinding{VRF: "OOB"},
			EndpointBinding{VRF: "MGMT", SourceIP: "10.0.0.1"},
			EndpointBinding{VRF: "OOB", SourceIP: "10.0.0.1"},
		},
		{
			"child overrides source_ip only",
			EndpointBinding{SourceIP: "10.0.0.99"},
			EndpointBinding{VRF: "MGMT", SourceIP: "10.0.0.1"},
			EndpointBinding{VRF: "MGMT", SourceIP: "10.0.0.99"},
		},
		{
			"child overrides everything",
			EndpointBinding{VRF: "OOB", SourceIP: "10.50.0.1", SourceIPv6: "fd00::1"},
			EndpointBinding{VRF: "MGMT", SourceIP: "10.0.0.1", SourceIPv6: "fd00::99"},
			EndpointBinding{VRF: "OOB", SourceIP: "10.50.0.1", SourceIPv6: "fd00::1"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.child.MergeWith(c.parent)
			if got != c.want {
				t.Fatalf("MergeWith=%+v want %+v", got, c.want)
			}
		})
	}
}

func TestEndpointBinding_Resolve_V4(t *testing.T) {
	b := EndpointBinding{VRF: "MGMT", SourceIP: "10.0.0.1"}
	got, err := b.Resolve(FamilyV4)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.VRF != "MGMT" {
		t.Errorf("VRF=%q want MGMT", got.VRF)
	}
	if got.SourceIP.String() != "10.0.0.1" {
		t.Errorf("SourceIP=%v want 10.0.0.1", got.SourceIP)
	}
}

func TestEndpointBinding_Resolve_V6(t *testing.T) {
	b := EndpointBinding{VRF: "MGMT", SourceIPv6: "2001:db8::1"}
	got, err := b.Resolve(FamilyV6)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.SourceIP.String() != "2001:db8::1" {
		t.Errorf("SourceIP=%v want 2001:db8::1", got.SourceIP)
	}
}

func TestEndpointBinding_Resolve_FamilyMismatch(t *testing.T) {
	b := EndpointBinding{SourceIP: "2001:db8::1"} // v6 in v4 field
	_, err := b.Resolve(FamilyV4)
	if err == nil || !strings.Contains(err.Error(), "IPv6") {
		t.Fatalf("want IPv6 family-mismatch error, got %v", err)
	}
}

func TestEndpointBinding_Resolve_SourceInterfaceNotImplemented(t *testing.T) {
	b := EndpointBinding{SourceInterface: "Loopback0"}
	_, err := b.Resolve(FamilyV4)
	if err == nil || !strings.Contains(err.Error(), "not yet implemented") {
		t.Fatalf("want source_interface not-implemented error, got %v", err)
	}
}

func TestEndpointBinding_Resolve_InvalidIP(t *testing.T) {
	b := EndpointBinding{SourceIP: "not.an.ip"}
	_, err := b.Resolve(FamilyV4)
	if err == nil {
		t.Fatal("want error on invalid IP")
	}
}

func TestEndpointBinding_Validate_Empty(t *testing.T) {
	if err := (EndpointBinding{}).Validate(FamilyV4, nil); err != nil {
		t.Fatalf("empty binding should validate: %v", err)
	}
}
