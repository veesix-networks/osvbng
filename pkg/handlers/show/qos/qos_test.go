// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package qos

import (
	"context"
	"strings"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

// fakeSouthbound embeds the interface so only the methods these handlers
// touch need bodies; anything else panics, which is the desired test
// behaviour.
type fakeSouthbound struct {
	southbound.Southbound
	ifMgr      *ifmgr.Manager
	schedulers []southbound.SchedulerState
	aggregates []southbound.AggregateState
	byParent   func(parentSwIfIndex uint32, level string, svlanID uint16) ([]southbound.SchedulerState, error)
}

func (f *fakeSouthbound) GetIfMgr() *ifmgr.Manager { return f.ifMgr }

func (f *fakeSouthbound) DumpSchedulers() ([]southbound.SchedulerState, error) {
	return append([]southbound.SchedulerState(nil), f.schedulers...), nil
}

func (f *fakeSouthbound) DumpScheduler(swIfIndex uint32) (*southbound.SchedulerState, error) {
	for i := range f.schedulers {
		if f.schedulers[i].SwIfIndex == swIfIndex {
			s := f.schedulers[i]
			return &s, nil
		}
	}
	return nil, nil
}

func (f *fakeSouthbound) DumpSchedulersByParent(parentSwIfIndex uint32, level string, svlanID uint16) ([]southbound.SchedulerState, error) {
	if f.byParent != nil {
		return f.byParent(parentSwIfIndex, level, svlanID)
	}
	return nil, southbound.ErrSchedV2Unsupported
}

func (f *fakeSouthbound) DumpAggregates() ([]southbound.AggregateState, error) {
	return append([]southbound.AggregateState(nil), f.aggregates...), nil
}

func testDeps(sb *fakeSouthbound) *deps.ShowDeps {
	if sb.ifMgr == nil {
		sb.ifMgr = ifmgr.New()
	}
	return &deps.ShowDeps{Southbound: sb}
}

func req(opts map[string]string) *show.Request {
	if opts == nil {
		opts = map[string]string{}
	}
	return &show.Request{Options: opts}
}

// The telemetry poller calls Collect with an empty request; if empty options
// ever mean "match nothing", every QoS metric silently vanishes.
func TestSchedulerList_EmptyRequestReturnsAll(t *testing.T) {
	sb := &fakeSouthbound{schedulers: []southbound.SchedulerState{
		{SwIfIndex: 7}, {SwIfIndex: 9},
	}}
	h := &SchedulerHandler{deps: testDeps(sb)}

	out, err := h.Collect(context.Background(), req(nil))
	if err != nil {
		t.Fatal(err)
	}
	if got := out.([]southbound.SchedulerState); len(got) != 2 {
		t.Fatalf("expected 2 schedulers, got %d", len(got))
	}
}

func TestSchedulerList_InterfaceFilter(t *testing.T) {
	sb := &fakeSouthbound{
		ifMgr: ifmgr.New(),
		schedulers: []southbound.SchedulerState{
			{SwIfIndex: 7}, {SwIfIndex: 9},
		},
	}
	sb.ifMgr.Add(&ifmgr.Interface{SwIfIndex: 9, Name: "ipoe0"})
	h := &SchedulerHandler{deps: testDeps(sb)}

	for _, opt := range []string{"9", "ipoe0"} {
		out, err := h.Collect(context.Background(), req(map[string]string{"interface": opt}))
		if err != nil {
			t.Fatal(err)
		}
		got := out.([]southbound.SchedulerState)
		if len(got) != 1 || got[0].SwIfIndex != 9 {
			t.Fatalf("filter %q: expected sw_if_index 9, got %+v", opt, got)
		}
	}

	if _, err := h.Collect(context.Background(), req(map[string]string{"interface": "nope"})); err == nil {
		t.Fatal("expected error for unknown interface name")
	}
}

func TestAggregateList_EmptyRequestReturnsAll(t *testing.T) {
	sb := &fakeSouthbound{aggregates: []southbound.AggregateState{
		{SwIfIndex: 1, Level: "port"},
		{SwIfIndex: 1, Level: "svlan", SVLANID: 100, SVLANIDEnd: 199},
	}}
	h := &AggregateHandler{deps: testDeps(sb)}

	out, err := h.Collect(context.Background(), req(nil))
	if err != nil {
		t.Fatal(err)
	}
	if got := out.([]southbound.AggregateState); len(got) != 2 {
		t.Fatalf("expected 2 aggregates, got %d", len(got))
	}
}

// The svlan filter matches any tag an aggregate's range covers, the way the
// dataplane's own svlan_map resolves a tag.
func TestAggregateList_SVLANRangeFilter(t *testing.T) {
	sb := &fakeSouthbound{aggregates: []southbound.AggregateState{
		{SwIfIndex: 1, Level: "port"},
		{SwIfIndex: 1, Level: "svlan", SVLANID: 100, SVLANIDEnd: 199},
		{SwIfIndex: 1, Level: "svlan", SVLANID: 200, SVLANIDEnd: 200},
	}}
	h := &AggregateHandler{deps: testDeps(sb)}

	out, err := h.Collect(context.Background(), req(map[string]string{"svlan": "150"}))
	if err != nil {
		t.Fatal(err)
	}
	got := out.([]southbound.AggregateState)
	if len(got) != 1 || got[0].SVLANID != 100 {
		t.Fatalf("expected the 100-199 aggregate, got %+v", got)
	}
}

func TestAggregateDetail_BuildsHierarchy(t *testing.T) {
	sb := &fakeSouthbound{
		ifMgr: ifmgr.New(),
		aggregates: []southbound.AggregateState{
			{SwIfIndex: 1, Level: "port", InterfaceName: "xe0"},
			{SwIfIndex: 1, Level: "svlan", SVLANID: 100, SVLANIDEnd: 199},
		},
		byParent: func(parent uint32, level string, svlanID uint16) ([]southbound.SchedulerState, error) {
			if level == "svlan" && svlanID == 100 {
				return []southbound.SchedulerState{{SwIfIndex: 7}}, nil
			}
			return nil, nil
		},
	}
	sb.ifMgr.Add(&ifmgr.Interface{SwIfIndex: 1, Name: "xe0"})
	h := &AggregateDetailHandler{deps: testDeps(sb)}

	out, err := h.Collect(context.Background(), req(map[string]string{"interface": "xe0"}))
	if err != nil {
		t.Fatal(err)
	}
	root := out.(*AggregateDetail)
	if root.Aggregate.Level != "port" {
		t.Fatalf("expected port at the root, got %q", root.Aggregate.Level)
	}
	if len(root.Children) != 1 || root.Children[0].Aggregate.SVLANID != 100 {
		t.Fatalf("expected one S-VLAN child 100-199, got %+v", root.Children)
	}
	if len(root.Children[0].Schedulers) != 1 || root.Children[0].Schedulers[0].SwIfIndex != 7 {
		t.Fatalf("expected member scheduler 7 under the S-VLAN, got %+v", root.Children[0].Schedulers)
	}
}

// A v1-only dataplane still renders the aggregate tiers; only membership
// carries a note.
func TestAggregateDetail_V1FallbackNote(t *testing.T) {
	sb := &fakeSouthbound{
		ifMgr: ifmgr.New(),
		aggregates: []southbound.AggregateState{
			{SwIfIndex: 1, Level: "port"},
		},
	}
	sb.ifMgr.Add(&ifmgr.Interface{SwIfIndex: 1, Name: "xe0"})
	h := &AggregateDetailHandler{deps: testDeps(sb)}

	out, err := h.Collect(context.Background(), req(map[string]string{"interface": "xe0"}))
	if err != nil {
		t.Fatal(err)
	}
	root := out.(*AggregateDetail)
	if !strings.Contains(root.Note, "3.1.0") {
		t.Fatalf("expected the dataplane-version note, got %q", root.Note)
	}
}

func TestAggregateDetail_RequiresInterface(t *testing.T) {
	h := &AggregateDetailHandler{deps: testDeps(&fakeSouthbound{})}
	if _, err := h.Collect(context.Background(), req(nil)); err == nil {
		t.Fatal("expected an error without --interface")
	}
}

// A session interface without a scheduler is a view with a note, never an
// error: the session exists, it just has no QoS applied.
func TestSchedulerDetail_NoSchedulerIsNote(t *testing.T) {
	sb := &fakeSouthbound{ifMgr: ifmgr.New()}
	sb.ifMgr.Add(&ifmgr.Interface{SwIfIndex: 7, Name: "ipoe0.100.1"})
	h := &SchedulerDetailHandler{deps: testDeps(sb)}

	out, err := h.Collect(context.Background(), req(map[string]string{"interface": "ipoe0.100.1"}))
	if err != nil {
		t.Fatal(err)
	}
	view := out.(*SchedulerSessionView)
	if view.Scheduler != nil || view.Note == "" {
		t.Fatalf("expected nil scheduler with a note, got %+v", view)
	}
}

func TestSchedulerDetail_ResolvesParents(t *testing.T) {
	sb := &fakeSouthbound{
		ifMgr: ifmgr.New(),
		schedulers: []southbound.SchedulerState{{
			SwIfIndex:       7,
			HasParent:       true,
			ParentLevel:     "svlan",
			ParentSwIfIndex: 1,
			ParentSVLANID:   100,
		}},
		aggregates: []southbound.AggregateState{
			{SwIfIndex: 1, Level: "port"},
			{SwIfIndex: 1, Level: "svlan", SVLANID: 100, SVLANIDEnd: 199},
		},
	}
	h := &SchedulerDetailHandler{deps: testDeps(sb)}

	out, err := h.Collect(context.Background(), req(map[string]string{"interface": "7"}))
	if err != nil {
		t.Fatal(err)
	}
	view := out.(*SchedulerSessionView)
	if view.ParentSVLAN == nil || view.ParentSVLAN.SVLANID != 100 {
		t.Fatalf("expected the S-VLAN parent, got %+v", view.ParentSVLAN)
	}
	if view.ParentPort == nil || view.ParentPort.Level != "port" {
		t.Fatalf("expected the port parent, got %+v", view.ParentPort)
	}
}
