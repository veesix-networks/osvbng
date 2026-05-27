// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"net"
	"strings"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/cgnat"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

func newTestComponent(t *testing.T, ifaces ...*ifmgr.Interface) *Component {
	t.Helper()
	mgr := ifmgr.New()
	for _, iface := range ifaces {
		mgr.Add(iface)
	}
	return &Component{
		logger: logger.Get("cgnat-test"),
		ifMgr:  mgr,
	}
}

func TestResolveOutsideVRF_SingleInterface(t *testing.T) {
	c := newTestComponent(t,
		&ifmgr.Interface{SwIfIndex: 1, Name: "eth2", FIBTableID: 0},
	)
	vrf, err := c.resolveOutsideVRF("residential", []string{"eth2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vrf != 0 {
		t.Fatalf("expected vrf 0, got %d", vrf)
	}
}

func TestResolveOutsideVRF_SameVRFMultipleInterfaces(t *testing.T) {
	c := newTestComponent(t,
		&ifmgr.Interface{SwIfIndex: 1, Name: "bond0.100", FIBTableID: 100},
		&ifmgr.Interface{SwIfIndex: 2, Name: "bond0.101", FIBTableID: 100},
	)
	vrf, err := c.resolveOutsideVRF("residential", []string{"bond0.100", "bond0.101"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vrf != 100 {
		t.Fatalf("expected vrf 100, got %d", vrf)
	}
}

func TestResolveOutsideVRF_MultipleVRFsRejected(t *testing.T) {
	c := newTestComponent(t,
		&ifmgr.Interface{SwIfIndex: 1, Name: "bond0.100", FIBTableID: 0},
		&ifmgr.Interface{SwIfIndex: 2, Name: "bond0.101", FIBTableID: 100},
	)
	_, err := c.resolveOutsideVRF("residential", []string{"bond0.100", "bond0.101"})
	if err == nil || !strings.Contains(err.Error(), "span multiple VRFs") {
		t.Fatalf("expected multi-VRF rejection, got %v", err)
	}
	if !strings.Contains(err.Error(), `pool "residential"`) {
		t.Fatalf("expected pool name in error, got %v", err)
	}
}

func TestResolveOutsideVRF_UnknownInterface(t *testing.T) {
	c := newTestComponent(t,
		&ifmgr.Interface{SwIfIndex: 1, Name: "eth2", FIBTableID: 0},
	)
	_, err := c.resolveOutsideVRF("ispA", []string{"eth99"})
	if err == nil || !strings.Contains(err.Error(), "not found in dataplane") {
		t.Fatalf("expected not-found error, got %v", err)
	}
	if !strings.Contains(err.Error(), `pool "ispA"`) {
		t.Fatalf("expected pool name in error, got %v", err)
	}
}

func TestResolveOutsideVRF_WholesaleTwoPoolsTwoVRFs(t *testing.T) {
	c := newTestComponent(t,
		&ifmgr.Interface{SwIfIndex: 1, Name: "bond0.100", FIBTableID: 10},
		&ifmgr.Interface{SwIfIndex: 2, Name: "bond0.101", FIBTableID: 10},
		&ifmgr.Interface{SwIfIndex: 3, Name: "bond0.200", FIBTableID: 20},
		&ifmgr.Interface{SwIfIndex: 4, Name: "bond0.201", FIBTableID: 20},
	)
	vrfA, err := c.resolveOutsideVRF("ispA", []string{"bond0.100", "bond0.101"})
	if err != nil {
		t.Fatalf("ispA: %v", err)
	}
	vrfB, err := c.resolveOutsideVRF("ispB", []string{"bond0.200", "bond0.201"})
	if err != nil {
		t.Fatalf("ispB: %v", err)
	}
	if vrfA != 10 || vrfB != 20 {
		t.Fatalf("expected (10, 20), got (%d, %d)", vrfA, vrfB)
	}
}

func TestResolveOutsideVRF_SharedUplinkAcrossPoolsSameVRF(t *testing.T) {
	c := newTestComponent(t,
		&ifmgr.Interface{SwIfIndex: 1, Name: "bond0.100", FIBTableID: 10},
	)
	if _, err := c.resolveOutsideVRF("ispA", []string{"bond0.100"}); err != nil {
		t.Fatalf("ispA: %v", err)
	}
	if _, err := c.resolveOutsideVRF("ispB", []string{"bond0.100"}); err != nil {
		t.Fatalf("ispB sharing the same uplink should be valid: %v", err)
	}
}

func TestRejectOutsideSubscriberOverlap_CleanSeparation(t *testing.T) {
	c := newTestComponent(t)
	cfg := &config.Config{
		SubscriberGroups: &subscriber.SubscriberGroupsConfig{
			Groups: map[string]*subscriber.SubscriberGroup{
				"default": {
					VLANs: []subscriber.VLANRange{
						{SVLAN: "100-199", CVLAN: "any", ParentInterface: "eth1"},
					},
				},
			},
		},
	}
	if err := c.rejectOutsideSubscriberOverlap(cfg, "residential", []string{"eth2"}); err != nil {
		t.Fatalf("expected pass, got %v", err)
	}
}

func TestRejectOutsideSubscriberOverlap_Conflict(t *testing.T) {
	c := newTestComponent(t)
	cfg := &config.Config{
		SubscriberGroups: &subscriber.SubscriberGroupsConfig{
			Groups: map[string]*subscriber.SubscriberGroup{
				"default": {
					VLANs: []subscriber.VLANRange{
						{SVLAN: "100-199", CVLAN: "any", ParentInterface: "eth1"},
					},
				},
			},
		},
	}
	err := c.rejectOutsideSubscriberOverlap(cfg, "residential", []string{"eth1"})
	if err == nil || !strings.Contains(err.Error(), "subscriber access interface") {
		t.Fatalf("expected subscriber-overlap rejection, got %v", err)
	}
	if !strings.Contains(err.Error(), `pool "residential"`) {
		t.Fatalf("expected pool name in error, got %v", err)
	}
}

func TestRejectLocalAddressOverlap_Clean(t *testing.T) {
	c := newTestComponent(t,
		&ifmgr.Interface{
			SwIfIndex:     10,
			Name:          "loop0",
			FIBTableID:    0,
			IPv4Addresses: []net.IP{net.ParseIP("10.254.0.1").To4()},
		},
	)
	pool := &cgnat.Pool{OutsideAddresses: []string{"203.0.113.0/28"}}
	if err := c.rejectLocalAddressOverlap("residential", pool); err != nil {
		t.Fatalf("expected pass, got %v", err)
	}
}

func TestRejectLocalAddressOverlap_Conflict(t *testing.T) {
	c := newTestComponent(t,
		&ifmgr.Interface{
			SwIfIndex:     10,
			Name:          "loop0",
			FIBTableID:    0,
			IPv4Addresses: []net.IP{net.ParseIP("203.0.113.1").To4()},
		},
	)
	pool := &cgnat.Pool{OutsideAddresses: []string{"203.0.113.0/28"}}
	err := c.rejectLocalAddressOverlap("residential", pool)
	if err == nil || !strings.Contains(err.Error(), "overlaps with locally-owned address") {
		t.Fatalf("expected local-address-overlap rejection, got %v", err)
	}
	if !strings.Contains(err.Error(), `pool "residential"`) {
		t.Fatalf("expected pool name in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "loop0") {
		t.Fatalf("expected interface name in error, got %v", err)
	}
}

func TestRejectLocalAddressOverlap_SingleIPLiteral(t *testing.T) {
	c := newTestComponent(t,
		&ifmgr.Interface{
			SwIfIndex:     10,
			Name:          "loop0",
			FIBTableID:    0,
			IPv4Addresses: []net.IP{net.ParseIP("203.0.113.5").To4()},
		},
	)
	pool := &cgnat.Pool{OutsideAddresses: []string{"203.0.113.5"}}
	err := c.rejectLocalAddressOverlap("residential", pool)
	if err == nil || !strings.Contains(err.Error(), "overlaps with locally-owned address") {
		t.Fatalf("expected single-IP overlap rejection, got %v", err)
	}
}
