// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ipoe

import (
	"net"
	"testing"

	"github.com/google/gopacket/layers"
	"github.com/veesix-networks/osvbng/pkg/allocator"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/ip"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
)

func familyTestComponent(t *testing.T, group *subscriber.SubscriberGroup) (*Component, *captureBus) {
	t.Helper()

	ifMgr := ifmgr.New()
	ifMgr.Add(&ifmgr.Interface{SwIfIndex: 10, SupSwIfIndex: 2, Name: "TenGigE0/0.100", Type: ifmgr.IfTypeSub, OuterVlanID: 100})
	ifMgr.Add(&ifmgr.Interface{SwIfIndex: 2, Name: "TenGigE0/0", Type: ifmgr.IfTypeHardware, MAC: []byte{0x52, 0x54, 0x00, 0x11, 0x22, 0x33}})

	cfg := &config.Config{
		SubscriberGroups: &subscriber.SubscriberGroupsConfig{
			Groups: map[string]*subscriber.SubscriberGroup{"grp": group},
		},
		IPv6Profiles: map[string]*ip.IPv6Profile{
			"default-v6": {IANAPools: []ip.IANAPool{{Network: "2001:db8:0:1::/64", PreferredTime: 3600, ValidTime: 7200}}},
		},
	}

	bus := &captureBus{}
	srg := &fakeSRGProvider{active: true, virtualMAC: net.HardwareAddr{0xaa, 0xc1, 0xab, 0x1f, 0xe2, 0xfa}, srgForGrp: "grp"}

	c := &Component{
		Base:     component.NewBase("ipoe-test"),
		logger:   logger.NewTest(),
		eventBus: bus,
		srgMgr:   srg,
		ifMgr:    ifMgr,
		cfgMgr:   &fakeConfigManager{cfg: cfg},
	}
	return c, bus
}

func rsPacket(src net.IP) *dataplane.ParsedPacket {
	icmp := &layers.ICMPv6{TypeCode: layers.CreateICMPv6TypeCode(layers.ICMPv6TypeRouterSolicitation, 0)}
	icmp.BaseLayer = layers.BaseLayer{Contents: []byte{layers.ICMPv6TypeRouterSolicitation, 0, 0, 0}}
	return &dataplane.ParsedPacket{
		Protocol:  models.ProtocolIPv6ND,
		MAC:       net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
		OuterVLAN: 100,
		SwIfIndex: 10,
		IPv6:      &layers.IPv6{SrcIP: src},
		ICMPv6:    icmp,
	}
}

func TestProcessRSPacketFamilyGate(t *testing.T) {
	src := net.ParseIP("fe80::baad:f00d")

	t.Run("v4_only_group_emits_no_RA", func(t *testing.T) {
		c, bus := familyTestComponent(t, &subscriber.SubscriberGroup{
			IPv4Profile: "default",
			VLANs:       []subscriber.VLANRange{{SVLAN: "100"}},
		})
		before := ndDropFamily.WithLabelValues("grp", "rs").Value()

		if err := c.processRSPacket(rsPacket(src)); err != nil {
			t.Fatalf("processRSPacket: %v", err)
		}
		bus.mu.Lock()
		n := len(bus.egress)
		bus.mu.Unlock()
		if n != 0 {
			t.Fatalf("v4-only group must emit no RA, got %d egress", n)
		}
		if got := ndDropFamily.WithLabelValues("grp", "rs").Value(); got != before+1 {
			t.Fatalf("rs drop counter = %d, want %d", got, before+1)
		}
	})

	t.Run("v6_enabled_group_emits_RA", func(t *testing.T) {
		c, bus := familyTestComponent(t, &subscriber.SubscriberGroup{
			IPv6Profile: "default-v6",
			VLANs:       []subscriber.VLANRange{{SVLAN: "100"}},
		})
		if err := c.processRSPacket(rsPacket(src)); err != nil {
			t.Fatalf("processRSPacket: %v", err)
		}
		bus.mu.Lock()
		n := len(bus.egress)
		bus.mu.Unlock()
		if n != 1 {
			t.Fatalf("v6-enabled group must emit an RA, got %d egress", n)
		}
	})
}

func TestProcessNSPacketFamilyGate(t *testing.T) {
	vmac := net.HardwareAddr{0xaa, 0xc1, 0xab, 0x1f, 0xe2, 0xfa}
	ourLinkLocal := linkLocalFromMAC(vmac)
	host := net.ParseIP("fe80::baad:f00d")

	c, bus := familyTestComponent(t, &subscriber.SubscriberGroup{
		IPv4Profile: "default",
		VLANs:       []subscriber.VLANRange{{SVLAN: "100"}},
	})
	before := ndDropFamily.WithLabelValues("grp", "ns").Value()

	if err := c.processNSPacket(nsPacket(t, ourLinkLocal, host)); err != nil {
		t.Fatalf("processNSPacket: %v", err)
	}
	bus.mu.Lock()
	n := len(bus.egress)
	bus.mu.Unlock()
	if n != 0 {
		t.Fatalf("v4-only group must not answer NS, got %d egress", n)
	}
	if got := ndDropFamily.WithLabelValues("grp", "ns").Value(); got != before+1 {
		t.Fatalf("ns drop counter = %d, want %d", got, before+1)
	}
}

func TestSessionFamilyEnabled(t *testing.T) {
	cfg := &config.Config{SubscriberGroups: &subscriber.SubscriberGroupsConfig{
		Groups: map[string]*subscriber.SubscriberGroup{
			"v4only": {IPv4Profile: "default", VLANs: []subscriber.VLANRange{{SVLAN: "100"}}},
		},
	}}
	c := &Component{logger: logger.NewTest(), cfgMgr: &fakeConfigManager{cfg: cfg}}

	t.Run("approved_session_uses_alloc_ctx_read_once", func(t *testing.T) {
		// Group is v4-only, but the resolved policy enabled v6: the live session
		// keeps v6 because AllocCtx is authoritative once approved.
		sess := &SessionState{OuterVLAN: 100, AllocCtx: &allocator.Context{ProfileName: "p4", IPv6ProfileName: "p6"}}
		if !c.sessionV4Enabled(sess) || !c.sessionV6Enabled(sess) {
			t.Fatal("approved session must follow its allocator context")
		}
		sess.AllocCtx = &allocator.Context{ProfileName: "p4"}
		if !c.sessionV4Enabled(sess) || c.sessionV6Enabled(sess) {
			t.Fatal("empty IPv6ProfileName in ctx means v6 disabled")
		}
	})

	t.Run("preapproval_falls_back_to_group", func(t *testing.T) {
		sess := &SessionState{OuterVLAN: 100} // no AllocCtx yet
		if !c.sessionV4Enabled(sess) {
			t.Fatal("v4-only group: v4 must be enabled pre-approval")
		}
		if c.sessionV6Enabled(sess) {
			t.Fatal("v4-only group: v6 must be disabled pre-approval")
		}
	})
}

func TestCountFamilyAttrs(t *testing.T) {
	attrs := map[string]interface{}{
		"ipv6_address": "2001:db8::1",
		"pd_pool":      "pool",
		"ipv4_address": "10.0.0.5",
	}
	if n := countFamilyAttrs(attrs, v6FamilyAttrs); n != 2 {
		t.Fatalf("v6 family attr count = %d, want 2", n)
	}
	if n := countFamilyAttrs(attrs, v4FamilyAttrs); n != 1 {
		t.Fatalf("v4 family attr count = %d, want 1", n)
	}
}

func TestBuildAllocContextDropsOffFamily(t *testing.T) {
	cfg := &config.Config{SubscriberGroups: &subscriber.SubscriberGroupsConfig{
		Groups: map[string]*subscriber.SubscriberGroup{
			"v4only": {IPv4Profile: "default", VLANs: []subscriber.VLANRange{{SVLAN: "100"}}},
		},
	}}
	c := &Component{logger: logger.NewTest(), cfgMgr: &fakeConfigManager{cfg: cfg}}

	sess := &SessionState{SessionID: "s1", OuterVLAN: 100}
	attrs := map[string]interface{}{
		"ipv4_address": "10.0.0.5",
		"ipv6_address": "2001:db8::1",
		"pd_pool":      "pd",
	}
	ctx := c.buildAllocContext(sess, attrs)
	if ctx.IPv6ProfileName != "" {
		t.Fatalf("v4-only group must not resolve a v6 profile, got %q", ctx.IPv6ProfileName)
	}
	if ctx.IPv6Address != nil {
		t.Fatalf("v6 address must not flow into a v4-only session, got %v", ctx.IPv6Address)
	}
	if ctx.PDPoolOverride != "" {
		t.Fatalf("v6 pd pool must not flow into a v4-only session, got %q", ctx.PDPoolOverride)
	}
	if !ctx.IPv4Address.Equal(net.ParseIP("10.0.0.5")) {
		t.Fatalf("v4 address must still flow, got %v", ctx.IPv4Address)
	}
}
