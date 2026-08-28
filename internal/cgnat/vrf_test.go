// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"fmt"
	"net"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/cgnat"
	"github.com/veesix-networks/osvbng/pkg/config/servicegroup"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/events/local"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
)

// strictVRF answers only for the names it knows, as vrfmgr.Manager does.
type strictVRF struct{ tableID map[string]uint32 }

func (s *strictVRF) ResolveVRF(name string) (uint32, bool, bool, error) {
	if name == "" {
		return 0, false, false, nil
	}
	id, ok := s.tableID[name]
	if !ok {
		return 0, false, false, fmt.Errorf("VRF %q not found", name)
	}
	return id, true, true, nil
}

type mappingCall struct {
	swIfIndex uint32
	insideIP  net.IP
	vrfID     uint32
	outsideIP net.IP
	portStart uint16
	isAdd     bool
}

type bypassCall struct {
	vrfID uint32
	isAdd bool
}

// vrfDP records the VRF each southbound call carries and completes
// mapping calls inline.
type vrfDP struct {
	stubDP
	mappings []mappingCall
	bypasses []bypassCall
}

func (d *vrfDP) CGNATAddDelSubscriberMappingAsync(poolID, swIfIndex uint32, insideIP net.IP, insideVRFID uint32, outsideIP net.IP, portStart, portEnd uint16, enableFeature, isAdd bool, callback func(error)) {
	d.mappings = append(d.mappings, mappingCall{swIfIndex: swIfIndex, insideIP: insideIP, vrfID: insideVRFID, outsideIP: outsideIP, portStart: portStart, isAdd: isAdd})
	callback(nil)
}

func (d *vrfDP) CGNATAddDelBypass(prefix net.IPNet, vrfID uint32, isAdd bool) error {
	d.bypasses = append(d.bypasses, bypassCall{vrfID: vrfID, isAdd: isAdd})
	return nil
}

func newVRFComponent(t *testing.T, dp *vrfDP) *Component {
	t.Helper()
	cfg := &config.Config{
		CGNAT: &cgnat.Config{
			Pools: map[string]*cgnat.Pool{
				"p1": {
					Mode:                   "pba",
					BlockSize:              64,
					MaxBlocksPerSubscriber: 4,
					PortRange:              "1024-65535",
					AddressPooling:         "paired",
					OutsideAddresses:       []string{"203.0.113.0/30"},
				},
			},
		},
		ServiceGroups: map[string]*servicegroup.Config{
			"nat":    {CGNAT: &servicegroup.CGNATConfig{Policy: "p1"}},
			"bypass": {CGNAT: &servicegroup.CGNATConfig{Bypass: true}},
		},
	}
	pm := NewPoolManager()
	if err := pm.ConfigurePool("p1", 1, cfg.CGNAT.Pools["p1"]); err != nil {
		t.Fatalf("configure pool: %v", err)
	}
	return &Component{
		Base:           component.NewBase("cgnat"),
		logger:         logger.Get("cgnat-test"),
		eventBus:       local.NewBus(),
		dataplane:      dp,
		cfgMgr:         &fakeCfg{cfg: cfg},
		vrfMgr:         &strictVRF{tableID: map[string]uint32{"CUSTOMER-A": 100, "CUSTOMER-B": 200}},
		pools:          pm,
		reverse:        NewReverseIndex(),
		bypass:         NewBypassManager(),
		blacklist:      NewBlacklistManager(),
		poolIDMap:      map[string]uint32{"p1": 1},
		sessionPoolMap: map[string]string{},
		activations:    map[string]struct{}{},
	}
}

func ipoeEvent(id, ip, vrf, serviceGroup string, state models.SessionState) *events.SessionLifecycleEvent {
	return &events.SessionLifecycleEvent{
		AccessType: models.AccessTypeIPoE,
		SessionID:  id,
		State:      state,
		Session: &models.IPoESession{
			SessionID:    id,
			IPv4Address:  net.ParseIP(ip).To4(),
			VRF:          vrf,
			ServiceGroup: serviceGroup,
			IfIndex:      7,
		},
	}
}

func TestActivate_KeysMappingBySessionVRF(t *testing.T) {
	dp := &vrfDP{}
	c := newVRFComponent(t, dp)

	c.handleSessionActivate(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateActive))

	if len(dp.mappings) != 1 || !dp.mappings[0].isAdd {
		t.Fatalf("expected one mapping add, got %+v", dp.mappings)
	}
	if dp.mappings[0].vrfID != 100 {
		t.Fatalf("mapping programmed with vrf %d, want 100", dp.mappings[0].vrfID)
	}
	ip := net.ParseIP("100.64.0.2").To4()
	if got := c.pools.GetMappings("p1", ip, 100); len(got) != 1 || got[0].InsideVRFID != 100 {
		t.Fatalf("allocator not keyed by (100, %s): %+v", ip, got)
	}
	if got := c.pools.GetMappings("p1", ip, 0); len(got) != 0 {
		t.Fatalf("subscriber must not be held under table 0: %+v", got)
	}
}

func TestActivate_SameInsideIPInTwoVRFsGetsDisjointBlocks(t *testing.T) {
	dp := &vrfDP{}
	c := newVRFComponent(t, dp)

	c.handleSessionActivate(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateActive))
	c.handleSessionActivate(ipoeEvent("b1", "100.64.0.2", "CUSTOMER-B", "nat", models.SessionStateActive))

	if len(dp.mappings) != 2 {
		t.Fatalf("expected a dataplane add per VRF, got %+v", dp.mappings)
	}
	a, b := dp.mappings[0], dp.mappings[1]
	if a.vrfID != 100 || b.vrfID != 200 {
		t.Fatalf("vrf ids: got (%d, %d), want (100, 200)", a.vrfID, b.vrfID)
	}
	if a.outsideIP.Equal(b.outsideIP) && a.portStart == b.portStart {
		t.Fatalf("both VRFs were given block %s:%d", a.outsideIP, a.portStart)
	}
	if c.sessionPoolMap["a1"] != "p1" || c.sessionPoolMap["b1"] != "p1" {
		t.Fatalf("both sessions must commit, got %v", c.sessionPoolMap)
	}
}

func TestRelease_UsesSessionVRF(t *testing.T) {
	dp := &vrfDP{}
	c := newVRFComponent(t, dp)
	c.handleSessionActivate(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateActive))
	c.handleSessionActivate(ipoeEvent("b1", "100.64.0.2", "CUSTOMER-B", "nat", models.SessionStateActive))

	c.handleSessionRelease(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateReleased))

	last := dp.mappings[len(dp.mappings)-1]
	if last.isAdd || last.vrfID != 100 {
		t.Fatalf("expected a delete under vrf 100, got %+v", last)
	}
	ip := net.ParseIP("100.64.0.2").To4()
	if got := c.pools.GetMappings("p1", ip, 100); len(got) != 0 {
		t.Fatalf("CUSTOMER-A block still held after release: %+v", got)
	}
	if got := c.pools.GetMappings("p1", ip, 200); len(got) != 1 {
		t.Fatalf("CUSTOMER-B block must survive the other VRF's release: %+v", got)
	}
	if _, ok := c.sessionPoolMap["a1"]; ok {
		t.Fatalf("released session still committed")
	}
}

func TestActivate_UnknownVRFIsRefused(t *testing.T) {
	dp := &vrfDP{}
	c := newVRFComponent(t, dp)

	c.handleSessionActivate(ipoeEvent("x1", "100.64.0.2", "NOPE", "nat", models.SessionStateActive))

	if len(dp.mappings) != 0 {
		t.Fatalf("nothing may be programmed for an unresolvable VRF: %+v", dp.mappings)
	}
	if got := c.pools.GetAllMappings(); len(got) != 0 {
		t.Fatalf("nothing may be allocated for an unresolvable VRF: %+v", got)
	}
	if _, ok := c.sessionPoolMap["x1"]; ok {
		t.Fatalf("refused session must not commit")
	}
	if ok, _ := c.beginActivation("x1"); !ok {
		t.Fatalf("refused activation must release its slot")
	}
}

func TestBypass_CarriesSessionVRF(t *testing.T) {
	dp := &vrfDP{}
	c := newVRFComponent(t, dp)
	ip := net.ParseIP("100.64.0.2").To4()

	c.handleSessionActivate(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "bypass", models.SessionStateActive))

	if len(dp.bypasses) != 1 || !dp.bypasses[0].isAdd || dp.bypasses[0].vrfID != 100 {
		t.Fatalf("expected a bypass add under vrf 100, got %+v", dp.bypasses)
	}
	if !c.bypass.IsBypassed(ip, 100) || c.bypass.IsBypassed(ip, 0) {
		t.Fatalf("bypass must be recorded under the session's VRF only")
	}

	c.handleSessionRelease(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "bypass", models.SessionStateReleased))

	if len(dp.bypasses) != 2 || dp.bypasses[1].isAdd || dp.bypasses[1].vrfID != 100 {
		t.Fatalf("expected a bypass delete under vrf 100, got %+v", dp.bypasses)
	}
	if c.bypass.IsBypassed(ip, 100) {
		t.Fatalf("bypass still recorded after release")
	}
}

func TestActivate_ReusedBlockIsProgrammedForTheNewInterface(t *testing.T) {
	dp := &vrfDP{}
	c := newVRFComponent(t, dp)

	// a1 leaves without a release reaching CGNAT; a2 takes the same
	// address in the same VRF on a new session interface.
	c.handleSessionActivate(ipoeEvent("a1", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateActive))
	ev := ipoeEvent("a2", "100.64.0.2", "CUSTOMER-A", "nat", models.SessionStateActive)
	ev.Session.(*models.IPoESession).IfIndex = 9
	c.handleSessionActivate(ev)

	if len(dp.mappings) != 2 {
		t.Fatalf("the reused block must be programmed for the new interface, got %+v", dp.mappings)
	}
	if dp.mappings[1].swIfIndex != 9 || dp.mappings[1].vrfID != 100 || !dp.mappings[1].isAdd {
		t.Fatalf("second add must carry the new interface under the session VRF, got %+v", dp.mappings[1])
	}
	if dp.mappings[0].portStart != dp.mappings[1].portStart {
		t.Fatalf("the same subscriber identity must keep its block, got %d and %d", dp.mappings[0].portStart, dp.mappings[1].portStart)
	}
	if c.sessionPoolMap["a2"] != "p1" {
		t.Fatalf("a2 must commit, got %v", c.sessionPoolMap)
	}
}
