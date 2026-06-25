// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package pppoe

import (
	"context"
	"net"
	"testing"

	"github.com/google/gopacket/layers"
	"github.com/veesix-networks/osvbng/pkg/allocator"
	"github.com/veesix-networks/osvbng/pkg/config/ip"
	"github.com/veesix-networks/osvbng/pkg/dhcp6"
	"github.com/veesix-networks/osvbng/pkg/provider"
)

type fakeDHCP6Provider struct {
	called   int
	gotDUID  []byte
	reply    *dhcp6.Packet
	released [][]byte
}

func (f *fakeDHCP6Provider) Info() provider.Info { return provider.Info{Name: "fake"} }
func (f *fakeDHCP6Provider) HandlePacket(_ context.Context, pkt *dhcp6.Packet) (*dhcp6.Packet, error) {
	f.called++
	f.gotDUID = pkt.DUID
	return f.reply, nil
}
func (f *fakeDHCP6Provider) ReleaseLease(duid []byte) { f.released = append(f.released, duid) }

func TestPPPoEDHCPv6SolicitForwardedAndReplySent(t *testing.T) {
	s, bus := ndSession(t)
	s.ipv6cpOpen = true
	s.AllocCtx = &allocator.Context{IPv6ProfileName: "v6"}
	s.ClientLinkLocal = net.ParseIP("fe80::1111")
	s.DHCPv6DUID = []byte{0xaa, 0xbb} // stored by handleDHCPv6 before dispatch

	// relay mode so the handler passes the message through without resolving
	// local pools, and routes to the "relay" provider.
	cfg, _ := s.component.cfgMgr.GetRunning()
	cfg.IPv6Profiles["v6"].DHCPv6 = &ip.IPv6DHCPv6Options{Mode: "relay"}

	fp := &fakeDHCP6Provider{reply: &dhcp6.Packet{Raw: []byte{0x02, 0x00, 0x00, 0x01}}}
	s.component.dhcp6Providers = map[string]dhcp6.DHCPProvider{"relay": fp}

	msg := &dhcp6.Message{
		MsgType: dhcp6.MsgTypeSolicit,
		Raw:     []byte{0x01, 0x00, 0x00, 0x01},
		Options: dhcp6.Options{ClientID: []byte{0xaa, 0xbb}},
	}
	s.component.forwardDHCPv6(s, msg)

	if fp.called != 1 {
		t.Fatalf("provider HandlePacket calls = %d, want 1", fp.called)
	}
	if string(fp.gotDUID) != string([]byte{0xaa, 0xbb}) {
		t.Fatalf("provider got DUID %x, want aabb", fp.gotDUID)
	}
	if len(bus.egress) != 1 {
		t.Fatalf("want 1 DHCPv6 reply egress, got %d", len(bus.egress))
	}
	pkt := ndDecode(t, bus.egress[0].Packet.RawData)
	udp, _ := pkt.Layer(layers.LayerTypeUDP).(*layers.UDP)
	if udp == nil {
		t.Fatal("reply egress has no UDP layer")
	}
	if udp.SrcPort != 547 || udp.DstPort != 546 {
		t.Fatalf("reply ports = %d->%d, want 547->546", udp.SrcPort, udp.DstPort)
	}
}

func TestPPPoEDHCPv6BindSetsAddressAndPrefix(t *testing.T) {
	s, _ := ndSession(t)
	msg := &dhcp6.Message{
		MsgType: dhcp6.MsgTypeReply,
		Options: dhcp6.Options{
			IANA: &dhcp6.IANAOption{Address: net.ParseIP("2001:db8:0:1::1234"), ValidTime: 7200},
			IAPD: &dhcp6.IAPDOption{Prefix: net.ParseIP("2001:db8:beef::"), PrefixLen: 56, ValidTime: 7200},
		},
	}

	// swIdx 0 skips the VPP programming; the binding to session state is asserted.
	s.component.bindDHCPv6(s, 0, msg)

	if s.IPv6Address == nil || !s.IPv6Address.Equal(net.ParseIP("2001:db8:0:1::1234")) {
		t.Fatalf("IA-NA bind = %v, want 2001:db8:0:1::1234", s.IPv6Address)
	}
	if s.IPv6Prefix == nil || s.IPv6Prefix.String() != "2001:db8:beef::/56" {
		t.Fatalf("IA-PD bind = %v, want 2001:db8:beef::/56", s.IPv6Prefix)
	}
	if s.IPv6LeaseTime != 7200 {
		t.Fatalf("lease time = %d, want 7200", s.IPv6LeaseTime)
	}
}

func TestPPPoEDHCPv6NoProviderDropsSilently(t *testing.T) {
	s, bus := ndSession(t)
	s.ipv6cpOpen = true
	s.component.dhcp6Providers = nil // not configured

	if err := s.handleDHCPv6(net.ParseIP("fe80::1111"), []byte{0x01, 0x00, 0x00, 0x01}); err != nil {
		t.Fatalf("handleDHCPv6 with no provider must not error, got %v", err)
	}
	if len(bus.egress) != 0 {
		t.Fatalf("no provider configured must drop, got %d egress", len(bus.egress))
	}
}
