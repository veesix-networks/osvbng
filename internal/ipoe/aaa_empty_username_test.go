// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ipoe

import (
	"net"
	"testing"

	"github.com/google/gopacket/layers"
	"github.com/veesix-networks/osvbng/pkg/aaa"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/config"
	aaacfg "github.com/veesix-networks/osvbng/pkg/config/aaa"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

func emptyUsernameComponent(t *testing.T, format string) (*Component, *captureBus) {
	t.Helper()

	ifMgr := ifmgr.New()
	ifMgr.Add(&ifmgr.Interface{SwIfIndex: 10, SupSwIfIndex: 2, Name: "TenGigE0/0.100", Type: ifmgr.IfTypeSub, OuterVlanID: 100})
	ifMgr.Add(&ifmgr.Interface{SwIfIndex: 2, Name: "TenGigE0/0", Type: ifmgr.IfTypeHardware, MAC: []byte{0x52, 0x54, 0x00, 0x11, 0x22, 0x33}})

	cfg := &config.Config{
		SubscriberGroups: &subscriber.SubscriberGroupsConfig{
			Groups: map[string]*subscriber.SubscriberGroup{
				"grp": {
					IPv4Profile: "v4",
					AAAPolicy:   "p1",
					VLANs:       []subscriber.VLANRange{{SVLAN: "100"}},
				},
			},
		},
		AAA: aaacfg.AAAConfig{
			Policy: []aaacfg.AAAPolicy{{Name: "p1", Type: aaacfg.PolicyTypeDHCP, Format: format}},
		},
	}

	bus := &captureBus{}
	c := &Component{
		Base:     component.NewBase("ipoe-test"),
		logger:   logger.NewTest(),
		eventBus: bus,
		srgMgr:   &fakeSRGProvider{active: true, srgForGrp: "grp"},
		ifMgr:    ifMgr,
		cfgMgr:   &fakeConfigManager{cfg: cfg},
	}
	c.SetReadyState(component.StateReady)
	return c, bus
}

func discoverPacket(mac net.HardwareAddr) *dataplane.ParsedPacket {
	dh := &layers.DHCPv4{
		Operation:    layers.DHCPOpRequest,
		HardwareType: layers.LinkTypeEthernet,
		HardwareLen:  6,
		Xid:          0x1234,
		ClientHWAddr: mac,
		Options: layers.DHCPOptions{
			layers.NewDHCPOption(layers.DHCPOptMessageType, []byte{byte(layers.DHCPMsgTypeDiscover)}),
		},
	}
	return &dataplane.ParsedPacket{
		MAC:       mac,
		OuterVLAN: 100,
		SwIfIndex: 10,
		DHCPv4:    dh,
	}
}

// A policy format that references an absent identity token ($remote-id$ with no
// Option 82) must drop the DISCOVER without publishing an AAA request, bump the
// drop counter, and clear AAAInFlight so a retransmit re-fires the guard.
func TestHandleDiscoverEmptyUsernameDrops(t *testing.T) {
	c, bus := emptyUsernameComponent(t, "$remote-id$")
	mac := net.HardwareAddr{0xaa, 0x42, 0xa1, 0x0a, 0x54, 0x97}

	before := aaa.UsernameEmptyDrops.WithLabelValues("p1", "grp", "ipoe-dhcpv4").Value()

	if err := c.handleDiscover(discoverPacket(mac)); err != nil {
		t.Fatalf("handleDiscover returned error: %v", err)
	}

	if bus.aaaReqs != 0 {
		t.Fatalf("expected no AAA request published, got %d", bus.aaaReqs)
	}
	if got := aaa.UsernameEmptyDrops.WithLabelValues("p1", "grp", "ipoe-dhcpv4").Value(); got != before+1 {
		t.Fatalf("drop counter: want %d, got %d", before+1, got)
	}

	val, ok := c.sessions.Load(c.makeSessionKeyV4(mac, 100, 0))
	if !ok {
		t.Fatalf("session not found after DISCOVER")
	}
	sess := val.(*SessionState)
	sess.mu.Lock()
	inFlight := sess.AAAInFlight
	sess.mu.Unlock()
	if inFlight {
		t.Fatalf("AAAInFlight must be reset to false on empty-username drop")
	}
}

// A resolvable format ($mac-address$ always expands) must NOT drop: the AAA
// request is published and the drop counter is untouched.
func TestHandleDiscoverResolvableUsernamePublishes(t *testing.T) {
	c, bus := emptyUsernameComponent(t, "$mac-address$")
	mac := net.HardwareAddr{0xaa, 0x42, 0xa1, 0x0a, 0x54, 0x98}

	before := aaa.UsernameEmptyDrops.WithLabelValues("p1", "grp", "ipoe-dhcpv4").Value()

	if err := c.handleDiscover(discoverPacket(mac)); err != nil {
		t.Fatalf("handleDiscover returned error: %v", err)
	}

	if bus.aaaReqs != 1 {
		t.Fatalf("expected exactly one AAA request published, got %d", bus.aaaReqs)
	}
	if got := aaa.UsernameEmptyDrops.WithLabelValues("p1", "grp", "ipoe-dhcpv4").Value(); got != before {
		t.Fatalf("drop counter must not change on resolvable username: want %d, got %d", before, got)
	}
}
