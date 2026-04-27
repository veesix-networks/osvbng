// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package netbind

import (
	"errors"
	"net"
	"net/netip"
	"strings"
	"testing"

	"github.com/vishvananda/netlink"
)

type fakeVRFResolver struct {
	vrfs map[string]struct{ v4, v6 bool }
}

func (f *fakeVRFResolver) ResolveVRF(name string) (uint32, bool, bool, error) {
	v, ok := f.vrfs[name]
	if !ok {
		return 0, false, false, errors.New("vrf not found")
	}
	return 100, v.v4, v.v6, nil
}

type fakeLink struct {
	name        string
	idx         int
	masterIndex int
}

func (l *fakeLink) Attrs() *netlink.LinkAttrs {
	return &netlink.LinkAttrs{Name: l.name, Index: l.idx, MasterIndex: l.masterIndex}
}
func (l *fakeLink) Type() string { return "device" }

type fakeLinkLister struct {
	links []netlink.Link
	addrs map[int][]netlink.Addr // by link index
}

func (f *fakeLinkLister) LinkList() ([]netlink.Link, error) { return f.links, nil }

func (f *fakeLinkLister) LinkByName(name string) (netlink.Link, error) {
	for _, l := range f.links {
		if l.Attrs().Name == name {
			return l, nil
		}
	}
	return nil, errors.New("link not found")
}

func (f *fakeLinkLister) AddrList(link netlink.Link, family int) ([]netlink.Addr, error) {
	idx := link.Attrs().Index
	out := []netlink.Addr{}
	for _, a := range f.addrs[idx] {
		if a.IPNet == nil {
			continue
		}
		switch family {
		case netlink.FAMILY_V4:
			if a.IPNet.IP.To4() != nil {
				out = append(out, a)
			}
		case netlink.FAMILY_V6:
			if a.IPNet.IP.To4() == nil {
				out = append(out, a)
			}
		default:
			out = append(out, a)
		}
	}
	return out, nil
}

func TestValidateBinding_Empty(t *testing.T) {
	err := ValidateBinding(nil, nil, Binding{}, FamilyV4)
	if err != nil {
		t.Fatalf("empty binding should validate: %v", err)
	}
}

func TestValidateBinding_VRFNotFound(t *testing.T) {
	v := &fakeVRFResolver{vrfs: map[string]struct{ v4, v6 bool }{}}
	err := ValidateBinding(v, nil, Binding{VRF: "MGMT"}, FamilyV4)
	if err == nil || !strings.Contains(err.Error(), "vrf") {
		t.Fatalf("want vrf-not-found error, got %v", err)
	}
}

func TestValidateBinding_VRFFamilyMismatch(t *testing.T) {
	v := &fakeVRFResolver{vrfs: map[string]struct{ v4, v6 bool }{
		"MGMT": {v4: true, v6: false},
	}}
	err := ValidateBinding(v, nil, Binding{VRF: "MGMT"}, FamilyV6)
	if err == nil || !strings.Contains(err.Error(), "ipv6") {
		t.Fatalf("want ipv6-not-enabled error, got %v", err)
	}
}

func TestValidateBinding_SourceIPFamilyMismatch(t *testing.T) {
	err := ValidateBinding(nil, nil, Binding{SourceIP: netip.MustParseAddr("2001:db8::1")}, FamilyV4)
	if err == nil || !strings.Contains(err.Error(), "IPv6") {
		t.Fatalf("want family mismatch error, got %v", err)
	}
}

func TestValidateBinding_SourceIPOnInterfaceInVRF(t *testing.T) {
	vrfLink := &fakeLink{name: "MGMT", idx: 10}
	mgmt := &fakeLink{name: "eth0", idx: 11, masterIndex: 10}
	other := &fakeLink{name: "eth1", idx: 12}

	srcIP := net.ParseIP("10.99.0.254")
	otherIP := net.ParseIP("10.50.0.254")
	mask := net.CIDRMask(24, 32)

	nl := &fakeLinkLister{
		links: []netlink.Link{vrfLink, mgmt, other},
		addrs: map[int][]netlink.Addr{
			11: {{IPNet: &net.IPNet{IP: srcIP, Mask: mask}}},
			12: {{IPNet: &net.IPNet{IP: otherIP, Mask: mask}}},
		},
	}
	v := &fakeVRFResolver{vrfs: map[string]struct{ v4, v6 bool }{
		"MGMT": {v4: true},
	}}

	t.Run("source on enslaved interface", func(t *testing.T) {
		err := ValidateBinding(v, nl, Binding{
			VRF:      "MGMT",
			SourceIP: netip.MustParseAddr("10.99.0.254"),
		}, FamilyV4)
		if err != nil {
			t.Fatalf("want pass, got %v", err)
		}
	})

	t.Run("source on wrong-VRF interface", func(t *testing.T) {
		err := ValidateBinding(v, nl, Binding{
			VRF:      "MGMT",
			SourceIP: netip.MustParseAddr("10.50.0.254"),
		}, FamilyV4)
		if err == nil || !strings.Contains(err.Error(), "not enslaved") {
			t.Fatalf("want not-enslaved error, got %v", err)
		}
	})

	t.Run("source not assigned anywhere", func(t *testing.T) {
		err := ValidateBinding(v, nl, Binding{
			VRF:      "MGMT",
			SourceIP: netip.MustParseAddr("10.1.2.3"),
		}, FamilyV4)
		if err == nil {
			t.Fatal("want not-found error")
		}
	})

	t.Run("source without VRF, must exist somewhere", func(t *testing.T) {
		err := ValidateBinding(nil, nl, Binding{
			SourceIP: netip.MustParseAddr("10.50.0.254"),
		}, FamilyV4)
		if err != nil {
			t.Fatalf("source on any interface (no VRF) should pass: %v", err)
		}
	})
}
