// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ipoe

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/google/gopacket/layers"

	"github.com/veesix-networks/osvbng/pkg/allocator"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/ip"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
)

func TestLinkLocalFromMAC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mac  net.HardwareAddr
		want net.IP
	}{
		{
			name: "rfc4291_appendix_a_example",
			mac:  net.HardwareAddr{0x00, 0x0d, 0x88, 0xc0, 0x0a, 0x1d},
			want: net.ParseIP("fe80::20d:88ff:fec0:a1d"),
		},
		{
			name: "issue_89_repro_mac",
			mac:  net.HardwareAddr{0xaa, 0xc1, 0xab, 0x1f, 0xe2, 0xfa},
			want: net.ParseIP("fe80::a8c1:abff:fe1f:e2fa"),
		},
		{
			name: "locally_administered_bit_clears",
			mac:  net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x00, 0x01},
			want: net.ParseIP("fe80::ff:fe00:1"),
		},
		{
			name: "short_slice_returns_nil",
			mac:  net.HardwareAddr{0x00, 0x00, 0x00},
			want: nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := linkLocalFromMAC(tt.mac)

			if tt.want == nil {
				if got != nil {
					t.Fatalf("expected nil for short MAC, got %s", got)
				}
				return
			}

			if !got.Equal(tt.want) {
				t.Fatalf("linkLocalFromMAC(%s) = %s, want %s", tt.mac, got, tt.want)
			}
			if !got.IsLinkLocalUnicast() {
				t.Fatalf("output %s is not a link-local unicast", got)
			}
			if len(got) != 16 {
				t.Fatalf("output length %d, want 16", len(got))
			}
			if got[0] != 0xfe || got[1] != 0x80 {
				t.Fatalf("output prefix is %02x:%02x, want fe:80", got[0], got[1])
			}
		})
	}
}

type fakeSRGProvider struct {
	active     bool
	virtualMAC net.HardwareAddr
	srgForGrp  string
}

func (f *fakeSRGProvider) GetVirtualMAC(string) net.HardwareAddr { return f.virtualMAC }
func (f *fakeSRGProvider) IsActive(string) bool                  { return f.active }
func (f *fakeSRGProvider) GetSRGForGroup(string) string          { return f.srgForGrp }
func (f *fakeSRGProvider) RequestGARP(string)                    {}

type fakeConfigManager struct {
	cfg *config.Config
}

func (f *fakeConfigManager) GetRunning() (*config.Config, error) { return f.cfg, nil }
func (f *fakeConfigManager) GetStartup() (*config.Config, error) { return f.cfg, nil }

func (f *fakeConfigManager) LookupSubscriberGroup(svlan, cvlan uint16) (subscriber.GroupMatch, bool) {
	var groups *subscriber.SubscriberGroupsConfig
	if f.cfg != nil {
		groups = f.cfg.SubscriberGroups
	}
	return subscriber.BuildMatchIndex(groups).Lookup(svlan, cvlan)
}

type captureBus struct {
	mu      sync.Mutex
	egress  []events.EgressEvent
	aaaReqs int
}

func (b *captureBus) Publish(topic string, ev events.Event) {
	if topic == events.TopicAAARequest {
		b.mu.Lock()
		b.aaaReqs++
		b.mu.Unlock()
		return
	}
	if topic != events.TopicEgress {
		return
	}
	if eg, ok := ev.Data.(*events.EgressEvent); ok {
		b.mu.Lock()
		b.egress = append(b.egress, *eg)
		b.mu.Unlock()
	}
}

func (b *captureBus) Subscribe(string, events.Handler) events.Subscription { return nopSub{} }
func (b *captureBus) SubscribeAll(events.Handler) events.Subscription      { return nopSub{} }
func (b *captureBus) Stats() events.Stats                                  { return events.Stats{} }
func (b *captureBus) SetDebugTopics([]string)                              {}
func (b *captureBus) DebugTopics() []string                                { return nil }
func (b *captureBus) Close() error                                         { return nil }

type nopSub struct{}

func (nopSub) Unsubscribe() {}

func newNSPTestComponent(t *testing.T, virtualMAC net.HardwareAddr, active bool) (*Component, *captureBus) {
	t.Helper()

	ifMgr := ifmgr.New()
	ifMgr.Add(&ifmgr.Interface{
		SwIfIndex:    10,
		SupSwIfIndex: 2,
		Name:         "TenGigE0/0.100",
		Type:         ifmgr.IfTypeSub,
		OuterVlanID:  100,
	})
	ifMgr.Add(&ifmgr.Interface{
		SwIfIndex: 2,
		Name:      "TenGigE0/0",
		Type:      ifmgr.IfTypeHardware,
		MAC:       []byte{0x52, 0x54, 0x00, 0x11, 0x22, 0x33},
	})

	cfg := &config.Config{
		SubscriberGroups: &subscriber.SubscriberGroupsConfig{
			Groups: map[string]*subscriber.SubscriberGroup{
				"access1": {
					IPv6Profile: "default-v6",
					VLANs:       []subscriber.VLANRange{{SVLAN: "100"}},
				},
			},
		},
	}

	bus := &captureBus{}
	srg := &fakeSRGProvider{active: active, virtualMAC: virtualMAC, srgForGrp: "access1"}

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

func nsPacket(t *testing.T, target net.IP, source net.IP) *dataplane.ParsedPacket {
	t.Helper()

	body := make([]byte, 20)
	copy(body[4:20], target.To16())

	header := []byte{
		layers.ICMPv6TypeNeighborSolicitation, 0,
		0, 0,
	}

	icmp := &layers.ICMPv6{
		TypeCode: layers.CreateICMPv6TypeCode(layers.ICMPv6TypeNeighborSolicitation, 0),
	}
	icmp.BaseLayer = layers.BaseLayer{
		Contents: header,
		Payload:  body,
	}

	return &dataplane.ParsedPacket{
		Protocol:  models.ProtocolIPv6ND,
		MAC:       net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
		OuterVLAN: 100,
		SwIfIndex: 10,
		IPv6: &layers.IPv6{
			SrcIP: source,
		},
		ICMPv6: icmp,
	}
}

func TestProcessNSPacket(t *testing.T) {
	t.Parallel()

	vmac := net.HardwareAddr{0xaa, 0xc1, 0xab, 0x1f, 0xe2, 0xfa}
	ourLinkLocal := linkLocalFromMAC(vmac)

	t.Run("solicited_unicast_NS_for_our_link_local", func(t *testing.T) {
		c, bus := newNSPTestComponent(t, vmac, true)
		hostLinkLocal := net.ParseIP("fe80::baad:f00d")

		err := c.processNSPacket(nsPacket(t, ourLinkLocal, hostLinkLocal))
		if err != nil {
			t.Fatalf("processNSPacket returned error: %v", err)
		}

		bus.mu.Lock()
		defer bus.mu.Unlock()
		if len(bus.egress) != 1 {
			t.Fatalf("expected 1 egress, got %d", len(bus.egress))
		}
		eg := bus.egress[0]
		if eg.Protocol != models.ProtocolIPv6ND {
			t.Fatalf("protocol = %s, want %s", eg.Protocol, models.ProtocolIPv6ND)
		}
		if eg.Packet.SrcMAC != vmac.String() {
			t.Fatalf("SrcMAC = %s, want %s", eg.Packet.SrcMAC, vmac)
		}
	})

	t.Run("NS_for_someone_else_is_dropped", func(t *testing.T) {
		c, bus := newNSPTestComponent(t, vmac, true)
		hostLinkLocal := net.ParseIP("fe80::baad:f00d")
		otherTarget := net.ParseIP("fe80::dead:beef")

		err := c.processNSPacket(nsPacket(t, otherTarget, hostLinkLocal))
		if err != nil {
			t.Fatalf("processNSPacket returned error: %v", err)
		}

		bus.mu.Lock()
		defer bus.mu.Unlock()
		if len(bus.egress) != 0 {
			t.Fatalf("expected 0 egress for unrelated NS, got %d", len(bus.egress))
		}
	})

	t.Run("DAD_unspecified_source_replies_to_all_nodes", func(t *testing.T) {
		c, bus := newNSPTestComponent(t, vmac, true)

		err := c.processNSPacket(nsPacket(t, ourLinkLocal, net.IPv6unspecified))
		if err != nil {
			t.Fatalf("processNSPacket returned error: %v", err)
		}

		bus.mu.Lock()
		defer bus.mu.Unlock()
		if len(bus.egress) != 1 {
			t.Fatalf("expected 1 egress for DAD reply, got %d", len(bus.egress))
		}
	})

	t.Run("SRG_inactive_silent_drop", func(t *testing.T) {
		c, bus := newNSPTestComponent(t, vmac, false)
		hostLinkLocal := net.ParseIP("fe80::baad:f00d")

		err := c.processNSPacket(nsPacket(t, ourLinkLocal, hostLinkLocal))
		if err != nil {
			t.Fatalf("processNSPacket returned error: %v", err)
		}

		bus.mu.Lock()
		defer bus.mu.Unlock()
		if len(bus.egress) != 0 {
			t.Fatalf("standby node should not respond, got %d egress", len(bus.egress))
		}
	})

	t.Run("missing_S_VLAN_rejected", func(t *testing.T) {
		c, _ := newNSPTestComponent(t, vmac, true)
		pkt := nsPacket(t, ourLinkLocal, net.ParseIP("fe80::baad:f00d"))
		pkt.OuterVLAN = 0

		err := c.processNSPacket(pkt)
		if err == nil {
			t.Fatalf("expected error for missing S-VLAN, got nil")
		}
	})

	t.Run("short_NS_body_rejected", func(t *testing.T) {
		c, _ := newNSPTestComponent(t, vmac, true)
		pkt := nsPacket(t, ourLinkLocal, net.ParseIP("fe80::baad:f00d"))
		pkt.ICMPv6.Payload = []byte{1, 2, 3}

		err := c.processNSPacket(pkt)
		if err == nil {
			t.Fatalf("expected error for short NS body, got nil")
		}
	})
}

func TestProcessNSPacketCVLANGate(t *testing.T) {
	t.Parallel()

	vmac := net.HardwareAddr{0xaa, 0xc1, 0xab, 0x1f, 0xe2, 0xfa}
	ourLinkLocal := linkLocalFromMAC(vmac)
	host := net.ParseIP("fe80::baad:f00d")

	cfg := &config.Config{
		SubscriberGroups: &subscriber.SubscriberGroupsConfig{
			Groups: map[string]*subscriber.SubscriberGroup{
				"access1": {IPv6Profile: "default-v6", VLANs: []subscriber.VLANRange{{SVLAN: "100", CVLAN: "10"}}},
			},
		},
	}

	t.Run("matching_cvlan_answers", func(t *testing.T) {
		c, bus := newNSPTestComponent(t, vmac, true)
		c.cfgMgr = &fakeConfigManager{cfg: cfg}
		pkt := nsPacket(t, ourLinkLocal, host)
		pkt.InnerVLAN = 10

		if err := c.processNSPacket(pkt); err != nil {
			t.Fatalf("processNSPacket returned error: %v", err)
		}
		bus.mu.Lock()
		defer bus.mu.Unlock()
		if len(bus.egress) != 1 {
			t.Fatalf("matching C-VLAN: expected 1 egress, got %d", len(bus.egress))
		}
	})

	t.Run("non_matching_cvlan_dropped", func(t *testing.T) {
		c, bus := newNSPTestComponent(t, vmac, true)
		c.cfgMgr = &fakeConfigManager{cfg: cfg}
		pkt := nsPacket(t, ourLinkLocal, host)
		pkt.InnerVLAN = 20

		if err := c.processNSPacket(pkt); err != nil {
			t.Fatalf("processNSPacket returned error: %v", err)
		}
		bus.mu.Lock()
		defer bus.mu.Unlock()
		if len(bus.egress) != 0 {
			t.Fatalf("non-matching C-VLAN: expected 0 egress (gated), got %d", len(bus.egress))
		}
	})
}

func TestDHCPv4Mode(t *testing.T) {
	mkCfg := func(mode string) *config.Config {
		profile := &ip.IPv4Profile{}
		if mode != "" {
			profile.DHCP = &ip.IPv4DHCPOptions{Mode: mode}
		}
		return &config.Config{
			SubscriberGroups: &subscriber.SubscriberGroupsConfig{
				Groups: map[string]*subscriber.SubscriberGroup{
					"tob": {IPv4Profile: "default", VLANs: []subscriber.VLANRange{{SVLAN: "3000", CVLAN: "any"}}},
				},
			},
			IPv4Profiles: map[string]*ip.IPv4Profile{"default": profile},
		}
	}

	tests := []struct {
		name      string
		cfg       *config.Config
		svlan     uint16
		cvlan     uint16
		wantMode  string
		wantGroup string
	}{
		{"unset_mode_defaults_to_server", mkCfg(""), 3000, 618, "server", "tob"},
		{"explicit_server", mkCfg("server"), 3000, 618, "server", "tob"},
		{"relay", mkCfg("relay"), 3000, 618, "relay", "tob"},
		{"proxy", mkCfg("proxy"), 3000, 618, "proxy", "tob"},
		{"no_group_match_is_server", mkCfg("relay"), 4000, 0, "server", ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			c := &Component{cfgMgr: &fakeConfigManager{cfg: tt.cfg}}
			mode, group := c.dhcpv4Mode(tt.svlan, tt.cvlan)
			if mode != tt.wantMode || group != tt.wantGroup {
				t.Fatalf("dhcpv4Mode(%d,%d) = (%q,%q), want (%q,%q)",
					tt.svlan, tt.cvlan, mode, group, tt.wantMode, tt.wantGroup)
			}
		})
	}
}

func TestProcessDHCPForeignServer(t *testing.T) {
	// groupMode sets the base "default" profile's mode; a separate "relay-prof"
	// profile is always present so a per-session AAA override can be tested.
	mkComponent := func(groupMode string) (*Component, *captureBus) {
		base := &ip.IPv4Profile{}
		if groupMode != "" {
			base.DHCP = &ip.IPv4DHCPOptions{Mode: groupMode}
		}
		cfg := &config.Config{
			SubscriberGroups: &subscriber.SubscriberGroupsConfig{
				Groups: map[string]*subscriber.SubscriberGroup{
					"tob": {IPv4Profile: "default", VLANs: []subscriber.VLANRange{{SVLAN: "3000", CVLAN: "any"}}},
				},
			},
			IPv4Profiles: map[string]*ip.IPv4Profile{
				"default":    base,
				"relay-prof": {DHCP: &ip.IPv4DHCPOptions{Mode: "relay"}},
			},
		}
		bus := &captureBus{}
		c := &Component{
			Base:     component.NewBase("ipoe-test"),
			logger:   logger.NewTest(),
			eventBus: bus,
			srgMgr:   &fakeSRGProvider{active: true, virtualMAC: net.HardwareAddr{0x02, 0, 0, 0, 0, 1}, srgForGrp: "tob"},
			ifMgr:    ifmgr.New(),
			cfgMgr:   &fakeConfigManager{cfg: cfg},
		}
		return c, bus
	}

	clientMAC := net.HardwareAddr{0x50, 0x91, 0xe3, 0xcd, 0xb4, 0x2f}
	srvPkt := func(xid uint32, msgType layers.DHCPMsgType, yourIP net.IP) *dataplane.ParsedPacket {
		return &dataplane.ParsedPacket{
			MAC:       clientMAC,
			OuterVLAN: 3000,
			InnerVLAN: 618,
			SwIfIndex: 10,
			DHCPv4: &layers.DHCPv4{
				Xid:          xid,
				YourClientIP: yourIP,
				Flags:        0x8000,
				Options: layers.DHCPOptions{
					{Type: layers.DHCPOptMessageType, Data: []byte{byte(msgType)}, Length: 1},
				},
			},
		}
	}
	newSess := func(xid uint32, allocCtx *allocator.Context) *SessionState {
		return &SessionState{SessionID: "s", MAC: clientMAC, OuterVLAN: 3000, InnerVLAN: 618, GroupName: "tob", SRGName: "tob", EncapIfIndex: 10, XID: xid, AllocCtx: allocCtx}
	}
	egressCount := func(b *captureBus) int {
		b.mu.Lock()
		defer b.mu.Unlock()
		return len(b.egress)
	}

	t.Run("server_mode_drops_foreign_ack_without_binding", func(t *testing.T) {
		c, bus := mkComponent("")
		sess := newSess(0x1234, nil)
		c.xidIndex.Store(uint32(0x1234), sess)

		before := ipoeDropForeignServer.WithLabelValues("tob", layers.DHCPMsgTypeAck.String()).Value()
		if err := c.processDHCPPacket(srvPkt(0x1234, layers.DHCPMsgTypeAck, net.IPv4(100, 74, 90, 68))); err != nil {
			t.Fatalf("processDHCPPacket: %v", err)
		}
		if sess.IPv4 != nil {
			t.Fatalf("session bound foreign IP %v; must stay unbound in server mode", sess.IPv4)
		}
		if n := egressCount(bus); n != 0 {
			t.Fatalf("foreign ACK forwarded: %d egress, want 0", n)
		}
		if got := ipoeDropForeignServer.WithLabelValues("tob", layers.DHCPMsgTypeAck.String()).Value(); got != before+1 {
			t.Fatalf("drop counter = %d, want %d", got, before+1)
		}
	})

	t.Run("relay_group_forwards_server_response", func(t *testing.T) {
		c, bus := mkComponent("relay")
		c.xidIndex.Store(uint32(0x2222), newSess(0x2222, nil))

		before := ipoeDropForeignServer.WithLabelValues("tob", layers.DHCPMsgTypeNak.String()).Value()
		if err := c.processDHCPPacket(srvPkt(0x2222, layers.DHCPMsgTypeNak, net.IPv4zero)); err != nil {
			t.Fatalf("processDHCPPacket: %v", err)
		}
		if got := ipoeDropForeignServer.WithLabelValues("tob", layers.DHCPMsgTypeNak.String()).Value(); got != before {
			t.Fatalf("relay mode incremented foreign-server drop counter: %d -> %d", before, got)
		}
		if n := egressCount(bus); n != 1 {
			t.Fatalf("relay mode must forward server response: %d egress, want 1", n)
		}
	})

	t.Run("per_session_relay_override_forwards_despite_server_group", func(t *testing.T) {
		c, bus := mkComponent("") // group base profile is server mode
		c.xidIndex.Store(uint32(0x3333), newSess(0x3333, &allocator.Context{ProfileName: "relay-prof"}))

		before := ipoeDropForeignServer.WithLabelValues("tob", layers.DHCPMsgTypeNak.String()).Value()
		if err := c.processDHCPPacket(srvPkt(0x3333, layers.DHCPMsgTypeNak, net.IPv4zero)); err != nil {
			t.Fatalf("processDHCPPacket: %v", err)
		}
		if got := ipoeDropForeignServer.WithLabelValues("tob", layers.DHCPMsgTypeNak.String()).Value(); got != before {
			t.Fatalf("per-session relay override was wrongly dropped: %d -> %d", before, got)
		}
		if n := egressCount(bus); n != 1 {
			t.Fatalf("per-session relay override must forward: %d egress, want 1", n)
		}
	})
}

func TestSessionPastLease(t *testing.T) {
	c := &Component{}
	now := time.Unix(1_000_000, 0)

	v4 := func(boundAt time.Time, lease uint32) *SessionState {
		return &SessionState{State: "bound", IPv4: net.IPv4(100, 64, 0, 5), LeaseTime: lease, BoundAt: boundAt}
	}

	t.Run("within_lease_not_reaped", func(t *testing.T) {
		// 24h lease, bound 12h ago (well past the old 30-min idle reaper)
		s := v4(now.Add(-12*time.Hour), 86400)
		if c.sessionPastLease(s, now) {
			t.Fatal("bound session mid-lease must not be reaped")
		}
	})

	t.Run("past_lease_plus_grace_reaped", func(t *testing.T) {
		s := v4(now.Add(-24*time.Hour-reclaimGrace-time.Second), 86400)
		if !c.sessionPastLease(s, now) {
			t.Fatal("bound session past lease+grace must be reaped")
		}
	})

	t.Run("v4_lapsed_v6_valid_not_reaped", func(t *testing.T) {
		s := v4(now.Add(-25*time.Hour), 86400)
		s.IPv6Bound = true
		s.IPv6Address = net.ParseIP("2001:db8::1")
		s.IPv6LeaseTime = 86400
		s.IPv6BoundAt = now.Add(-1 * time.Hour)
		if c.sessionPastLease(s, now) {
			t.Fatal("session with a still-valid v6 lease must not be reaped")
		}
	})

	t.Run("both_families_lapsed_reaped", func(t *testing.T) {
		s := v4(now.Add(-25*time.Hour), 86400)
		s.IPv6Bound = true
		s.IPv6LeaseTime = 86400
		s.IPv6BoundAt = now.Add(-25 * time.Hour)
		if !c.sessionPastLease(s, now) {
			t.Fatal("session with both leases lapsed must be reaped")
		}
	})

	t.Run("short_lease_grace_capped", func(t *testing.T) {
		// 2-min lease: grace caps at lease/4 = 30s, not the full 5m.
		if !c.sessionPastLease(v4(now.Add(-2*time.Minute-31*time.Second), 120), now) {
			t.Fatal("2-min lease past lease+30s grace must be reaped (grace capped)")
		}
		if c.sessionPastLease(v4(now.Add(-2*time.Minute-20*time.Second), 120), now) {
			t.Fatal("2-min lease within lease+30s grace must not be reaped")
		}
	})

	t.Run("bound_no_lease_info_not_reaped", func(t *testing.T) {
		s := &SessionState{State: "bound"}
		if c.sessionPastLease(s, now) {
			t.Fatal("bound session with no lease info must not be reaped on lease basis")
		}
	})
}
