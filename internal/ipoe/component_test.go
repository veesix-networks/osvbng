// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ipoe

import (
	"net"
	"sync"
	"testing"

	"github.com/google/gopacket/layers"

	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/config"
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

func (f *fakeSRGProvider) GetVirtualMAC(string) net.HardwareAddr  { return f.virtualMAC }
func (f *fakeSRGProvider) IsActive(string) bool                   { return f.active }
func (f *fakeSRGProvider) GetSRGForGroup(string) string           { return f.srgForGrp }
func (f *fakeSRGProvider) RequestGARP(string)                     {}

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
	mu     sync.Mutex
	egress []events.EgressEvent
}

func (b *captureBus) Publish(topic string, ev events.Event) {
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
					VLANs: []subscriber.VLANRange{{SVLAN: "100"}},
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
