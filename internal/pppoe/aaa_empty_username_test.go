// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package pppoe

import (
	"net"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/aaa"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/config"
	aaacfg "github.com/veesix-networks/osvbng/pkg/config/aaa"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

type pppCaptureBus struct {
	aaaReqs int
	egress  int
}

func (b *pppCaptureBus) Publish(topic string, _ events.Event) {
	switch topic {
	case events.TopicAAARequest:
		b.aaaReqs++
	case events.TopicEgress:
		b.egress++
	}
}
func (b *pppCaptureBus) Subscribe(string, events.Handler) events.Subscription { return pppNopSub{} }
func (b *pppCaptureBus) SubscribeAll(events.Handler) events.Subscription      { return pppNopSub{} }
func (b *pppCaptureBus) Stats() events.Stats                                  { return events.Stats{} }
func (b *pppCaptureBus) SetDebugTopics([]string)                              {}
func (b *pppCaptureBus) DebugTopics() []string                                { return nil }
func (b *pppCaptureBus) Close() error                                         { return nil }

type pppNopSub struct{}

func (pppNopSub) Unsubscribe() {}

type pppFakeCfgMgr struct{ cfg *config.Config }

func (f *pppFakeCfgMgr) GetRunning() (*config.Config, error) { return f.cfg, nil }
func (f *pppFakeCfgMgr) GetStartup() (*config.Config, error) { return f.cfg, nil }
func (f *pppFakeCfgMgr) LookupSubscriberGroup(svlan, cvlan uint16) (subscriber.GroupMatch, bool) {
	var groups *subscriber.SubscriberGroupsConfig
	if f.cfg != nil {
		groups = f.cfg.SubscriberGroups
	}
	return subscriber.BuildMatchIndex(groups).Lookup(svlan, cvlan)
}

func pppEmptyUsernameSession(t *testing.T, format string) (*SessionState, *pppCaptureBus) {
	t.Helper()
	cfg := &config.Config{
		SubscriberGroups: &subscriber.SubscriberGroupsConfig{
			Groups: map[string]*subscriber.SubscriberGroup{
				"grp": {AAAPolicy: "p1", VLANs: []subscriber.VLANRange{{SVLAN: "100"}}},
			},
		},
		AAA: aaacfg.AAAConfig{
			Policy: []aaacfg.AAAPolicy{{Name: "p1", Type: aaacfg.PolicyTypePPP, Format: format}},
		},
	}
	ifMgr := ifmgr.New()
	ifMgr.Add(&ifmgr.Interface{SwIfIndex: 10, SupSwIfIndex: 2, Name: "TenGigE0/0.100", Type: ifmgr.IfTypeSub, OuterVlanID: 100})
	ifMgr.Add(&ifmgr.Interface{SwIfIndex: 2, Name: "TenGigE0/0", Type: ifmgr.IfTypeHardware, MAC: []byte{0x52, 0x54, 0x00, 0x11, 0x22, 0x33}})

	bus := &pppCaptureBus{}
	c := &Component{
		Base:     component.NewBase("pppoe-test"),
		logger:   logger.NewTest(),
		eventBus: bus,
		ifMgr:    ifMgr,
		cfgMgr:   &pppFakeCfgMgr{cfg: cfg},
	}
	s := &SessionState{
		component:            c,
		SessionID:            "s1",
		MAC:                  net.HardwareAddr{0xaa, 0x42, 0xa1, 0x0a, 0x54, 0x97},
		OuterVLAN:            100,
		EncapIfIndex:         10,
		Username:             "subscriber@isp",
		pendingAuthType:      "pap",
		pendingPAPID:         7,
		pendingAuthRequestID: "stale",
	}
	s.initPPP()
	return s, bus
}

// A PPP policy format that expands to empty must fail auth: no AAA request is
// published, the drop counter increments, a PAP-NAK is emitted, and the pending
// auth fields are cleared so a retried Auth-Request cannot double-fire.
func TestPublishAAARequestEmptyUsernameFailsAuth(t *testing.T) {
	s, bus := pppEmptyUsernameSession(t, "$remote-id$")

	before := aaa.UsernameEmptyDrops.WithLabelValues("p1", "grp", "pppoe").Value()
	s.publishAAARequest(map[string]string{})

	if bus.aaaReqs != 0 {
		t.Fatalf("expected no AAA request published, got %d", bus.aaaReqs)
	}
	if got := aaa.UsernameEmptyDrops.WithLabelValues("p1", "grp", "pppoe").Value(); got != before+1 {
		t.Fatalf("drop counter: want %d, got %d", before+1, got)
	}
	if bus.egress == 0 {
		t.Fatalf("expected a PAP-NAK to be emitted on auth failure")
	}
	if s.pendingAuthType != "" {
		t.Fatalf("pendingAuthType must be cleared, got %q", s.pendingAuthType)
	}
	if s.pendingAuthRequestID != "" {
		t.Fatalf("pendingAuthRequestID must be cleared, got %q", s.pendingAuthRequestID)
	}
}

// A resolvable PPP format publishes the AAA request and does not increment the
// drop counter.
func TestPublishAAARequestResolvableUsernamePublishes(t *testing.T) {
	s, bus := pppEmptyUsernameSession(t, "$mac-address$")

	before := aaa.UsernameEmptyDrops.WithLabelValues("p1", "grp", "pppoe").Value()
	s.publishAAARequest(map[string]string{})

	if bus.aaaReqs != 1 {
		t.Fatalf("expected exactly one AAA request published, got %d", bus.aaaReqs)
	}
	if got := aaa.UsernameEmptyDrops.WithLabelValues("p1", "grp", "pppoe").Value(); got != before {
		t.Fatalf("drop counter must not change on resolvable username: want %d, got %d", before, got)
	}
}
