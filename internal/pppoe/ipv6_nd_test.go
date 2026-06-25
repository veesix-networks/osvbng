// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package pppoe

import (
	"encoding/binary"
	"net"
	"testing"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/veesix-networks/osvbng/internal/ra"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/ip"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

type ndCaptureBus struct{ egress []events.EgressEvent }

func (b *ndCaptureBus) Publish(topic string, ev events.Event) {
	if topic == events.TopicEgress {
		if eg, ok := ev.Data.(*events.EgressEvent); ok {
			b.egress = append(b.egress, *eg)
		}
	}
}
func (b *ndCaptureBus) Subscribe(string, events.Handler) events.Subscription { return pppNopSub{} }
func (b *ndCaptureBus) SubscribeAll(events.Handler) events.Subscription      { return pppNopSub{} }
func (b *ndCaptureBus) Stats() events.Stats                                  { return events.Stats{} }
func (b *ndCaptureBus) SetDebugTopics([]string)                              {}
func (b *ndCaptureBus) DebugTopics() []string                                { return nil }
func (b *ndCaptureBus) Close() error                                         { return nil }

// ndParentMAC is the access parent's hardware MAC; with no SRG the BNG sources
// RA/NA from its link-local.
var ndParentMAC = net.HardwareAddr{0x52, 0x54, 0x00, 0x11, 0x22, 0x33}

func ndSession(t *testing.T) (*SessionState, *ndCaptureBus) {
	t.Helper()
	cfg := &config.Config{
		SubscriberGroups: &subscriber.SubscriberGroupsConfig{
			Groups: map[string]*subscriber.SubscriberGroup{
				"grp": {IPv6Profile: "v6", VLANs: []subscriber.VLANRange{{SVLAN: "100"}}},
			},
		},
		IPv6Profiles: map[string]*ip.IPv6Profile{
			"v6": {IANAPools: []ip.IANAPool{{Network: "2001:db8:0:1::/64", ValidTime: 7200, PreferredTime: 3600}}},
		},
	}
	ifMgr := ifmgr.New()
	ifMgr.Add(&ifmgr.Interface{SwIfIndex: 10, SupSwIfIndex: 2, Name: "TenGigE0/0.100", Type: ifmgr.IfTypeSub, OuterVlanID: 100})
	ifMgr.Add(&ifmgr.Interface{SwIfIndex: 2, Name: "TenGigE0/0", Type: ifmgr.IfTypeHardware, MAC: ndParentMAC})

	bus := &ndCaptureBus{}
	c := &Component{
		Base:          component.NewBase("pppoe-test"),
		logger:        logger.NewTest(),
		eventBus:      bus,
		ifMgr:         ifMgr,
		cfgMgr:        &pppFakeCfgMgr{cfg: cfg},
		raEngine:      ra.NewEngine(false, logger.NewTest()),
		raBuckets:     make(map[int][]string),
		raBucketCount: 16,
	}
	s := &SessionState{
		component:    c,
		SessionID:    "s1",
		MAC:          net.HardwareAddr{0xaa, 0x42, 0xa1, 0x0a, 0x54, 0x97},
		OuterVLAN:    100,
		EncapIfIndex: 10,
	}
	return s, bus
}

// ndDecode extracts the gopacket decode of a PPPoE egress frame (PPPoE -> PPP
// 0x0057 -> IPv6 -> ICMPv6).
func ndDecode(t *testing.T, raw []byte) gopacket.Packet {
	t.Helper()
	pkt := gopacket.NewPacket(raw, layers.LayerTypePPPoE, gopacket.Default)
	if pkt.Layer(layers.LayerTypeIPv6) == nil {
		t.Fatalf("egress frame has no IPv6 layer: %x", raw)
	}
	return pkt
}

func TestPPPoERouterSolicitationGetsOffLinkRA(t *testing.T) {
	s, bus := ndSession(t)
	if err := s.processRSPacket(net.ParseIP("fe80::1111")); err != nil {
		t.Fatalf("processRSPacket: %v", err)
	}
	if len(bus.egress) != 1 {
		t.Fatalf("want 1 egress RA, got %d", len(bus.egress))
	}
	pkt := ndDecode(t, bus.egress[0].Packet.RawData)

	ip6 := pkt.Layer(layers.LayerTypeIPv6).(*layers.IPv6)
	wantSrc := ra.LinkLocalFromMAC(ndParentMAC)
	if !ip6.SrcIP.Equal(wantSrc) {
		t.Fatalf("RA source = %s, want BNG link-local %s", ip6.SrcIP, wantSrc)
	}

	raLayer, _ := pkt.Layer(layers.LayerTypeICMPv6RouterAdvertisement).(*layers.ICMPv6RouterAdvertisement)
	if raLayer == nil {
		t.Fatal("no Router Advertisement layer")
	}
	var pio *layers.ICMPv6Option
	for i := range raLayer.Options {
		switch raLayer.Options[i].Type {
		case layers.ICMPv6OptPrefixInfo:
			pio = &raLayer.Options[i]
		case layers.ICMPv6OptSourceAddress:
			t.Fatal("PPPoE RA must not carry a Source Link-Layer Address option")
		}
	}
	if pio == nil {
		t.Fatal("RA has no Prefix Information option")
	}
	if pio.Data[1]&0x80 == 0 {
		t.Fatal("PIO L flag must be set")
	}
	if vl := binary.BigEndian.Uint32(pio.Data[2:6]); vl != 0 {
		t.Fatalf("off-link PIO valid lifetime must be 0, got %d", vl)
	}
}

func TestPPPoENeighborSolicitationForGatewayGetsNA(t *testing.T) {
	s, bus := ndSession(t)
	bngLL := ra.LinkLocalFromMAC(ndParentMAC)

	// NS for a target other than the BNG gateway is ignored.
	if err := s.processNSPacket(net.ParseIP("fe80::1111"), net.ParseIP("fe80::dead")); err != nil {
		t.Fatalf("processNSPacket(other): %v", err)
	}
	if len(bus.egress) != 0 {
		t.Fatalf("NS for a non-gateway target must be ignored, got %d egress", len(bus.egress))
	}

	// NS for the BNG link-local is answered with an NA.
	if err := s.processNSPacket(net.ParseIP("fe80::1111"), bngLL); err != nil {
		t.Fatalf("processNSPacket(gateway): %v", err)
	}
	if len(bus.egress) != 1 {
		t.Fatalf("want 1 NA, got %d", len(bus.egress))
	}
	pkt := ndDecode(t, bus.egress[0].Packet.RawData)
	na, _ := pkt.Layer(layers.LayerTypeICMPv6NeighborAdvertisement).(*layers.ICMPv6NeighborAdvertisement)
	if na == nil {
		t.Fatal("no Neighbor Advertisement layer")
	}
	if !na.TargetAddress.Equal(bngLL) {
		t.Fatalf("NA target = %s, want %s", na.TargetAddress, bngLL)
	}
	if na.Flags&0x40 == 0 {
		t.Fatal("solicited NA must set the S flag")
	}
	for _, opt := range na.Options {
		if opt.Type == layers.ICMPv6OptTargetAddress {
			t.Fatal("PPPoE NA must not carry a Target Link-Layer Address option")
		}
	}
}
