// Copyright 2025 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"fmt"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/events/local"
)

func newTestSM(name string, priority uint32, preempt bool, nodeID string) *SRGStateMachine {
	sm, _ := NewSRGStateMachine(name, &config.SRGConfig{
		VirtualMAC:       "02:ab:cd:00:00:01",
		Priority:         priority,
		Preempt:          preempt,
		SubscriberGroups: []string{"residential", "business"},
	}, nodeID)
	return sm
}

func TestSRGStateMachine_InitToWaiting(t *testing.T) {
	sm := newTestSM("srg1", 100, false, "node-a")

	if sm.State() != SRGStateInit {
		t.Fatalf("expected INIT, got %s", sm.State())
	}

	tr := sm.Start()
	if tr == nil {
		t.Fatal("expected transition")
	}
	if tr.OldState != SRGStateInit || tr.NewState != SRGStateWaiting {
		t.Fatalf("expected INIT->WAITING, got %s->%s", tr.OldState, tr.NewState)
	}
	if sm.State() != SRGStateWaiting {
		t.Fatalf("expected WAITING, got %s", sm.State())
	}
}

func TestSRGStateMachine_DoubleStartNoop(t *testing.T) {
	sm := newTestSM("srg1", 100, false, "node-a")
	sm.Start()
	tr := sm.Start()
	if tr != nil {
		t.Fatal("expected nil transition on double start")
	}
}

func TestSRGStateMachine_PeerDiscoveredMovesToReady(t *testing.T) {
	sm := newTestSM("srg1", 100, false, "node-a")
	sm.Start()

	tr := sm.PeerDiscovered(50, "node-b", SRGStateWaiting)
	if tr == nil {
		t.Fatal("expected transition")
	}
	if tr.NewState != SRGStateReady {
		t.Fatalf("expected READY, got %s", tr.NewState)
	}
}

func TestSRGStateMachine_ElectionHigherPriorityWins(t *testing.T) {
	sm := newTestSM("srg1", 200, false, "node-a")
	sm.Start()
	sm.PeerDiscovered(100, "node-b", SRGStateWaiting)

	tr := sm.Elect("node-b")
	if tr == nil {
		t.Fatal("expected transition")
	}
	if tr.NewState != SRGStateActive {
		t.Fatalf("expected ACTIVE (higher priority), got %s", tr.NewState)
	}
}

func TestSRGStateMachine_ElectionLowerPriorityLoses(t *testing.T) {
	sm := newTestSM("srg1", 50, false, "node-a")
	sm.Start()
	sm.PeerDiscovered(200, "node-b", SRGStateWaiting)

	tr := sm.Elect("node-b")
	if tr == nil {
		t.Fatal("expected transition")
	}
	if tr.NewState != SRGStateStandby {
		t.Fatalf("expected STANDBY (lower priority), got %s", tr.NewState)
	}
}

func TestSRGStateMachine_ElectionTiebreakByNodeID(t *testing.T) {
	sm := newTestSM("srg1", 100, false, "node-a")
	sm.Start()
	sm.PeerDiscovered(100, "node-b", SRGStateWaiting)

	tr := sm.Elect("node-b")
	if tr == nil {
		t.Fatal("expected transition")
	}
	if tr.NewState != SRGStateActive {
		t.Fatalf("expected ACTIVE (lower node_id wins tie), got %s", tr.NewState)
	}

	sm2 := newTestSM("srg1", 100, false, "node-b")
	sm2.Start()
	sm2.PeerDiscovered(100, "node-a", SRGStateWaiting)

	tr2 := sm2.Elect("node-a")
	if tr2 == nil {
		t.Fatal("expected transition")
	}
	if tr2.NewState != SRGStateStandby {
		t.Fatalf("expected STANDBY (higher node_id loses tie), got %s", tr2.NewState)
	}
}

func TestSRGStateMachine_PeerLostActiveToSolo(t *testing.T) {
	sm := newTestSM("srg1", 200, false, "node-a")
	sm.Start()
	sm.PeerDiscovered(100, "node-b", SRGStateWaiting)
	sm.Elect("node-b")

	if sm.State() != SRGStateActive {
		t.Fatalf("expected ACTIVE, got %s", sm.State())
	}

	tr := sm.PeerLost()
	if tr == nil {
		t.Fatal("expected transition")
	}
	if tr.NewState != SRGStateActiveSolo {
		t.Fatalf("expected ACTIVE_SOLO, got %s", tr.NewState)
	}
}

func TestSRGStateMachine_PeerLostStandbyToActive(t *testing.T) {
	sm := newTestSM("srg1", 50, false, "node-a")
	sm.Start()
	sm.PeerDiscovered(200, "node-b", SRGStateWaiting)
	sm.Elect("node-b")

	if sm.State() != SRGStateStandby {
		t.Fatalf("expected STANDBY, got %s", sm.State())
	}

	tr := sm.PeerLost()
	if tr == nil {
		t.Fatal("expected transition")
	}
	if tr.NewState != SRGStateActive {
		t.Fatalf("expected ACTIVE (peer lost while standby), got %s", tr.NewState)
	}
}

func TestSRGStateMachine_SoloBackToReady(t *testing.T) {
	sm := newTestSM("srg1", 200, false, "node-a")
	sm.Start()
	sm.PeerDiscovered(100, "node-b", SRGStateWaiting)
	sm.Elect("node-b")
	sm.PeerLost()

	if sm.State() != SRGStateActiveSolo {
		t.Fatalf("expected ACTIVE_SOLO, got %s", sm.State())
	}

	tr := sm.PeerDiscovered(100, "node-b", SRGStateWaiting)
	if tr == nil {
		t.Fatal("expected transition")
	}
	if tr.NewState != SRGStateReady {
		t.Fatalf("expected READY, got %s", tr.NewState)
	}
}

func TestSRGStateMachine_GracefulSwitchover(t *testing.T) {
	sm := newTestSM("srg1", 200, false, "node-a")
	sm.Start()
	sm.PeerDiscovered(100, "node-b", SRGStateWaiting)
	sm.Elect("node-b")

	if sm.State() != SRGStateActive {
		t.Fatalf("expected ACTIVE, got %s", sm.State())
	}

	tr := sm.Switchover()
	if tr == nil {
		t.Fatal("expected transition")
	}
	if tr.NewState != SRGStateStandby {
		t.Fatalf("expected STANDBY after switchover, got %s", tr.NewState)
	}
}

func TestSRGStateMachine_PreemptReelection(t *testing.T) {
	sm := newTestSM("srg1", 200, true, "node-a")
	sm.Start()
	sm.PeerDiscovered(100, "node-b", SRGStateWaiting)
	sm.Elect("node-b")
	sm.Switchover()

	if sm.State() != SRGStateStandby {
		t.Fatalf("expected STANDBY, got %s", sm.State())
	}

	tr := sm.PeerHeartbeatUpdate(100, "node-b", SRGStateActive)
	if tr == nil {
		t.Fatal("expected transition (preempt)")
	}
	if tr.NewState != SRGStateActive {
		t.Fatalf("expected ACTIVE (preempt), got %s", tr.NewState)
	}
}

func TestSRGStateMachine_SplitBrainYield(t *testing.T) {
	sm := newTestSM("srg1", 200, false, "node-a")
	sm.Start()
	sm.PeerDiscovered(100, "node-b", SRGStateWaiting)
	sm.Elect("node-b")

	if sm.State() != SRGStateActive {
		t.Fatalf("expected ACTIVE, got %s", sm.State())
	}

	sm.AdjustPriority(-180)

	tr := sm.PeerHeartbeatUpdate(100, "node-b", SRGStateActive)
	if tr == nil {
		t.Fatal("expected transition (yield to peer in split-brain)")
	}
	if tr.NewState != SRGStateStandby {
		t.Fatalf("expected STANDBY (lower priority yields), got %s", tr.NewState)
	}
}

func TestSRGStateMachine_SplitBrainKeepsActiveIfWins(t *testing.T) {
	sm := newTestSM("srg1", 200, false, "node-a")
	sm.Start()
	sm.PeerDiscovered(100, "node-b", SRGStateWaiting)
	sm.Elect("node-b")

	if sm.State() != SRGStateActive {
		t.Fatalf("expected ACTIVE, got %s", sm.State())
	}

	tr := sm.PeerHeartbeatUpdate(100, "node-b", SRGStateActive)
	if tr != nil {
		t.Fatal("expected no transition (higher priority keeps ACTIVE)")
	}
}

func TestSRGStateMachine_NoPreemptStaysStandby(t *testing.T) {
	sm := newTestSM("srg1", 200, false, "node-a")
	sm.Start()
	sm.PeerDiscovered(100, "node-b", SRGStateWaiting)
	sm.Elect("node-b")
	sm.Switchover()

	tr := sm.PeerHeartbeatUpdate(100, "node-b", SRGStateActive)
	if tr != nil {
		t.Fatal("expected no transition (preempt disabled)")
	}
	if sm.State() != SRGStateStandby {
		t.Fatalf("expected STANDBY, got %s", sm.State())
	}
}

func TestSRGStateMachine_OwnsSubscriberGroup(t *testing.T) {
	sm := newTestSM("srg1", 100, false, "node-a")
	if !sm.OwnsSubscriberGroup("residential") {
		t.Fatal("expected to own group residential")
	}
	if !sm.OwnsSubscriberGroup("business") {
		t.Fatal("expected to own group business")
	}
	if sm.OwnsSubscriberGroup("wholesale") {
		t.Fatal("expected not to own group wholesale")
	}
}

func TestSRGStateMachine_IsActive(t *testing.T) {
	sm := newTestSM("srg1", 200, false, "node-a")

	if sm.IsActive() {
		t.Fatal("should not be active in INIT")
	}

	sm.Start()
	sm.PeerDiscovered(100, "node-b", SRGStateWaiting)
	sm.Elect("node-b")

	if !sm.IsActive() {
		t.Fatal("should be active after election win")
	}

	sm.PeerLost()
	if !sm.IsActive() {
		t.Fatal("should be active in ACTIVE_SOLO")
	}
}

func TestSRGStateMachine_VirtualMAC(t *testing.T) {
	sm := newTestSM("srg1", 100, false, "node-a")
	vmac := sm.VirtualMAC()
	if vmac == nil {
		t.Fatal("expected virtual MAC")
	}
	if vmac.String() != "02:ab:cd:00:00:01" {
		t.Fatalf("expected 02:ab:cd:00:00:01, got %s", vmac.String())
	}
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	cfg := &config.HAConfig{
		Enabled: true,
		NodeID:  "node-a",
		SRGs: map[string]*config.SRGConfig{
			"srg1": {
				VirtualMAC:       "02:ab:cd:00:00:01",
				Priority:         100,
				SubscriberGroups: []string{"residential", "business"},
			},
			"srg2": {
				VirtualMAC:       "02:ab:cd:00:00:02",
				Priority:         50,
				SubscriberGroups: []string{"wholesale"},
			},
		},
	}
	m, err := NewManager(cfg, local.NewBus())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m
}

func TestManager_GetSRGForGroup(t *testing.T) {
	m := newTestManager(t)

	if name := m.GetSRGForGroup("residential"); name != "srg1" {
		t.Fatalf("expected srg1, got %q", name)
	}
	if name := m.GetSRGForGroup("wholesale"); name != "srg2" {
		t.Fatalf("expected srg2, got %q", name)
	}
	if name := m.GetSRGForGroup("unknown"); name != "" {
		t.Fatalf("expected empty, got %q", name)
	}
	if name := m.GetSRGForGroup(""); name != "" {
		t.Fatalf("expected empty for empty input, got %q", name)
	}
}

func TestManager_GetVirtualMAC(t *testing.T) {
	m := newTestManager(t)

	vmac := m.GetVirtualMAC("srg1")
	if vmac == nil || vmac.String() != "02:ab:cd:00:00:01" {
		t.Fatalf("expected 02:ab:cd:00:00:01, got %v", vmac)
	}

	vmac = m.GetVirtualMAC("srg2")
	if vmac == nil || vmac.String() != "02:ab:cd:00:00:02" {
		t.Fatalf("expected 02:ab:cd:00:00:02, got %v", vmac)
	}

	if vmac := m.GetVirtualMAC("nonexistent"); vmac != nil {
		t.Fatalf("expected nil for unknown SRG, got %v", vmac)
	}

	if vmac := m.GetVirtualMAC(""); vmac != nil {
		t.Fatalf("expected nil for empty SRG name, got %v", vmac)
	}
}

func TestManager_IsActive(t *testing.T) {
	m := newTestManager(t)

	if !m.IsActive("") {
		t.Fatal("empty SRG name should return true")
	}

	if !m.IsActive("nonexistent") {
		t.Fatal("unknown SRG should return true")
	}

	for _, sm := range m.srgs {
		sm.Start()
		sm.PeerDiscovered(200, "node-b", SRGStateWaiting)
		sm.Elect("node-b")
	}

	if m.IsActive("srg1") {
		t.Fatal("srg1 should be standby (lower priority)")
	}

	if m.IsActive("srg2") {
		t.Fatal("srg2 should be standby (lower priority)")
	}
}

func TestSRGStateMachine_AdjustPriority(t *testing.T) {
	sm := newTestSM("srg1", 200, false, "node-a")

	if sm.Priority() != 200 {
		t.Fatalf("expected initial priority 200, got %d", sm.Priority())
	}

	sm.AdjustPriority(-50)
	if sm.Priority() != 150 {
		t.Fatalf("expected priority 150 after -50 delta, got %d", sm.Priority())
	}

	sm.AdjustPriority(-100)
	if sm.Priority() != 100 {
		t.Fatalf("expected priority 100 after -100 delta, got %d", sm.Priority())
	}

	sm.AdjustPriority(0)
	if sm.Priority() != 200 {
		t.Fatalf("expected priority restored to 200 after zero delta, got %d", sm.Priority())
	}
}

func TestSRGStateMachine_AdjustPriorityClampsToZero(t *testing.T) {
	sm := newTestSM("srg1", 50, false, "node-a")

	sm.AdjustPriority(-100)
	if sm.Priority() != 0 {
		t.Fatalf("expected priority clamped to 0, got %d", sm.Priority())
	}

	sm.AdjustPriority(-999)
	if sm.Priority() != 0 {
		t.Fatalf("expected priority still 0 with large negative delta, got %d", sm.Priority())
	}
}

func TestSRGStateMachine_BasePriority(t *testing.T) {
	sm := newTestSM("srg1", 200, false, "node-a")

	if sm.BasePriority() != 200 {
		t.Fatalf("expected base priority 200, got %d", sm.BasePriority())
	}

	sm.AdjustPriority(-150)
	if sm.BasePriority() != 200 {
		t.Fatalf("expected base priority unchanged at 200, got %d", sm.BasePriority())
	}
	if sm.Priority() != 50 {
		t.Fatalf("expected effective priority 50, got %d", sm.Priority())
	}
}

func TestSRGStateMachine_ElectionUsesEffectivePriority(t *testing.T) {
	sm := newTestSM("srg1", 200, false, "node-a")
	sm.AdjustPriority(-180)

	sm.Start()
	sm.PeerDiscovered(100, "node-b", SRGStateWaiting)

	tr := sm.Elect("node-b")
	if tr == nil {
		t.Fatal("expected transition")
	}
	if tr.NewState != SRGStateStandby {
		t.Fatalf("expected STANDBY (effective 20 < peer 100), got %s", tr.NewState)
	}
}

func TestSRGStateMachine_PreemptWithReducedPriority(t *testing.T) {
	sm := newTestSM("srg1", 200, true, "node-a")
	sm.Start()
	sm.PeerDiscovered(100, "node-b", SRGStateWaiting)
	sm.Elect("node-b")

	if sm.State() != SRGStateActive {
		t.Fatalf("expected ACTIVE, got %s", sm.State())
	}

	sm.Switchover()
	if sm.State() != SRGStateStandby {
		t.Fatalf("expected STANDBY after switchover, got %s", sm.State())
	}

	sm.AdjustPriority(-150)

	tr := sm.PeerHeartbeatUpdate(100, "node-b", SRGStateActive)
	if tr != nil {
		t.Fatal("expected no preempt (effective 50 < peer 100)")
	}
	if sm.State() != SRGStateStandby {
		t.Fatalf("expected STANDBY, got %s", sm.State())
	}
}

func newTestManagerWithTracking(t *testing.T) *Manager {
	t.Helper()
	cfg := &config.HAConfig{
		Enabled: true,
		NodeID:  "node-a",
		SRGs: map[string]*config.SRGConfig{
			"srg1": {
				VirtualMAC:             "02:ab:cd:00:00:01",
				Priority:               200,
				Preempt:                true,
				SubscriberGroups:       []string{"residential"},
				Interfaces:             []string{"GigE0/0/0", "GigE0/0/1"},
				TrackPriorityDecrement: 50,
			},
		},
	}
	ifIndices := map[string]uint32{
		"GigE0/0/0": 10,
		"GigE0/0/1": 11,
	}
	m, err := NewManager(cfg, local.NewBus(),
		WithInterfaceResolver(func(name string) (uint32, error) {
			idx, ok := ifIndices[name]
			if !ok {
				return 0, fmt.Errorf("interface %q not found", name)
			}
			return idx, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	m.buildInterfaceMap()
	return m
}

func TestManager_BuildInterfaceMap(t *testing.T) {
	m := newTestManagerWithTracking(t)

	if len(m.ifToSRG) != 2 {
		t.Fatalf("expected 2 tracked interfaces, got %d", len(m.ifToSRG))
	}
	if m.ifToSRG[10] != "srg1" {
		t.Fatalf("expected ifToSRG[10]=srg1, got %q", m.ifToSRG[10])
	}
	if m.ifToSRG[11] != "srg1" {
		t.Fatalf("expected ifToSRG[11]=srg1, got %q", m.ifToSRG[11])
	}
}

func TestManager_BuildInterfaceMapSkipsZeroDecrement(t *testing.T) {
	cfg := &config.HAConfig{
		Enabled: true,
		NodeID:  "node-a",
		SRGs: map[string]*config.SRGConfig{
			"srg1": {
				VirtualMAC:             "02:ab:cd:00:00:01",
				Priority:               200,
				SubscriberGroups:       []string{"residential"},
				Interfaces:             []string{"GigE0/0/0"},
				TrackPriorityDecrement: 0,
			},
		},
	}
	m, err := NewManager(cfg, local.NewBus(),
		WithInterfaceResolver(func(name string) (uint32, error) {
			return 10, nil
		}),
	)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	m.buildInterfaceMap()

	if len(m.ifToSRG) != 0 {
		t.Fatalf("expected 0 tracked interfaces for zero decrement, got %d", len(m.ifToSRG))
	}
}

func TestManager_BuildInterfaceMapCallsWatchCallback(t *testing.T) {
	var watchedIndices []uint32
	cfg := &config.HAConfig{
		Enabled: true,
		NodeID:  "node-a",
		SRGs: map[string]*config.SRGConfig{
			"srg1": {
				VirtualMAC:             "02:ab:cd:00:00:01",
				Priority:               200,
				SubscriberGroups:       []string{"residential"},
				Interfaces:             []string{"GigE0/0/0"},
				TrackPriorityDecrement: 50,
			},
		},
	}
	m, err := NewManager(cfg, local.NewBus(),
		WithInterfaceResolver(func(name string) (uint32, error) {
			return 10, nil
		}),
		WithInterfaceWatchCallback(func(idx uint32) {
			watchedIndices = append(watchedIndices, idx)
		}),
	)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	m.buildInterfaceMap()

	if len(watchedIndices) != 1 || watchedIndices[0] != 10 {
		t.Fatalf("expected watch callback called with [10], got %v", watchedIndices)
	}
}

func TestManager_HandleInterfaceEventLinkDown(t *testing.T) {
	m := newTestManagerWithTracking(t)

	m.handleInterfaceEvent(events.Event{
		Data: events.InterfaceStateEvent{
			SwIfIndex: 10,
			Name:      "GigE0/0/0",
			LinkUp:    false,
		},
	})

	sm := m.srgs["srg1"]
	if sm.Priority() != 150 {
		t.Fatalf("expected priority 150 after one interface down, got %d", sm.Priority())
	}
}

func TestManager_HandleInterfaceEventLinkUp(t *testing.T) {
	m := newTestManagerWithTracking(t)

	m.handleInterfaceEvent(events.Event{
		Data: events.InterfaceStateEvent{
			SwIfIndex: 10,
			Name:      "GigE0/0/0",
			LinkUp:    false,
		},
	})

	m.handleInterfaceEvent(events.Event{
		Data: events.InterfaceStateEvent{
			SwIfIndex: 10,
			Name:      "GigE0/0/0",
			LinkUp:    true,
		},
	})

	sm := m.srgs["srg1"]
	if sm.Priority() != 200 {
		t.Fatalf("expected priority restored to 200, got %d", sm.Priority())
	}
}

func TestManager_HandleInterfaceEventUntrackedIgnored(t *testing.T) {
	m := newTestManagerWithTracking(t)

	m.handleInterfaceEvent(events.Event{
		Data: events.InterfaceStateEvent{
			SwIfIndex: 999,
			Name:      "Unknown0",
			LinkUp:    false,
		},
	})

	sm := m.srgs["srg1"]
	if sm.Priority() != 200 {
		t.Fatalf("expected priority unchanged at 200 for untracked interface, got %d", sm.Priority())
	}
}

func TestManager_HandleInterfaceEventMultipleDown(t *testing.T) {
	m := newTestManagerWithTracking(t)

	m.handleInterfaceEvent(events.Event{
		Data: events.InterfaceStateEvent{
			SwIfIndex: 10,
			Name:      "GigE0/0/0",
			LinkUp:    false,
		},
	})
	m.handleInterfaceEvent(events.Event{
		Data: events.InterfaceStateEvent{
			SwIfIndex: 11,
			Name:      "GigE0/0/1",
			LinkUp:    false,
		},
	})

	sm := m.srgs["srg1"]
	if sm.Priority() != 100 {
		t.Fatalf("expected priority 100 after two interfaces down (200 - 2*50), got %d", sm.Priority())
	}

	m.handleInterfaceEvent(events.Event{
		Data: events.InterfaceStateEvent{
			SwIfIndex: 10,
			Name:      "GigE0/0/0",
			LinkUp:    true,
		},
	})

	if sm.Priority() != 150 {
		t.Fatalf("expected priority 150 after one restored (200 - 1*50), got %d", sm.Priority())
	}
}
