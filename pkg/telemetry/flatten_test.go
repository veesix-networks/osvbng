// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type flatChild struct {
	Pool  string `metric:"label"`
	InUse uint64 `metric:"name=cgnat.pool.in_use,type=gauge,help=..."`
}

type flatParent struct {
	Active uint64      `metric:"name=cgnat.sessions.active,type=gauge,help=..."`
	Pools  []flatChild `metric:"flatten"`
}

func TestFlatten_SingleLevel(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	sm := bindShowType(reg, reflect.TypeOf(flatParent{}))

	resp := flatParent{
		Active: 7,
		Pools: []flatChild{
			{Pool: "p1", InUse: 10},
			{Pool: "p2", InUse: 25},
		},
	}
	sm.emit(reflect.ValueOf(resp), nil, nil)

	got := reg.AppendSnapshot(nil, SnapshotOptions{})
	want := map[string]float64{
		`cgnat.sessions.active{}`: 7,
		`cgnat.pool.in_use{pool=p1}`: 10,
		`cgnat.pool.in_use{pool=p2}`: 25,
	}
	if !samplesMatch(got, want) {
		t.Fatalf("unexpected samples:\n%v", samplesDump(got))
	}
}

type twoLevelLeaf struct {
	Iface  string `metric:"label"`
	Weight uint64 `metric:"name=mpls.path.weight,type=gauge,help=..."`
}

type twoLevelMid struct {
	Eos   bool           `metric:"label"`
	Paths []twoLevelLeaf `metric:"flatten"`
}

type twoLevelOuter struct {
	Label  uint32        `metric:"label"`
	Routes []twoLevelMid `metric:"flatten"`
}

func TestFlatten_TwoLevel_LabelPropagation(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	sm := bindShowType(reg, reflect.TypeOf(twoLevelOuter{}))

	val := twoLevelOuter{
		Label: 100,
		Routes: []twoLevelMid{
			{Eos: true, Paths: []twoLevelLeaf{{Iface: "eth0", Weight: 1}}},
		},
	}
	sm.emit(reflect.ValueOf(val), nil, nil)

	got := reg.AppendSnapshot(nil, SnapshotOptions{})
	want := map[string]float64{
		`mpls.path.weight{label=100,eos=true,iface=eth0}`: 1,
	}
	if !samplesMatch(got, want) {
		t.Fatalf("unexpected samples:\n%v", samplesDump(got))
	}
}

type multiBranchA struct {
	Tag string `metric:"label"`
	V   uint64 `metric:"name=multi.branch_a,type=gauge,help=..."`
}

type multiBranchB struct {
	Tag string `metric:"label"`
	V   uint64 `metric:"name=multi.branch_b,type=gauge,help=..."`
}

type multiBranchParent struct {
	A []multiBranchA `metric:"flatten"`
	B []multiBranchB `metric:"flatten"`
}

func TestFlatten_MultipleSiblingBranches(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	sm := bindShowType(reg, reflect.TypeOf(multiBranchParent{}))

	val := multiBranchParent{
		A: []multiBranchA{{Tag: "x", V: 11}},
		B: []multiBranchB{{Tag: "y", V: 22}},
	}
	sm.emit(reflect.ValueOf(val), nil, nil)

	got := reg.AppendSnapshot(nil, SnapshotOptions{})
	want := map[string]float64{
		`multi.branch_a{tag=x}`: 11,
		`multi.branch_b{tag=y}`: 22,
	}
	if !samplesMatch(got, want) {
		t.Fatalf("unexpected samples:\n%v", samplesDump(got))
	}
}

type pointerFlatChild struct {
	Z uint64 `metric:"name=ptr.flat.z,type=gauge,help=..."`
}
type pointerFlatParent struct {
	Inner *pointerFlatChild `metric:"flatten"`
}

func TestFlatten_PointerUnwrap_NilSilent(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	sm := bindShowType(reg, reflect.TypeOf(pointerFlatParent{}))

	sm.emit(reflect.ValueOf(pointerFlatParent{Inner: nil}), nil, nil)
	got := onlyApp(reg.AppendSnapshot(nil, SnapshotOptions{}))
	if len(got) != 0 {
		t.Fatalf("expected no samples for nil pointer flatten, got %v", samplesDump(got))
	}

	sm.emit(reflect.ValueOf(pointerFlatParent{Inner: &pointerFlatChild{Z: 42}}), nil, nil)
	got = reg.AppendSnapshot(nil, SnapshotOptions{})
	want := map[string]float64{`ptr.flat.z{}`: 42}
	if !samplesMatch(got, want) {
		t.Fatalf("unexpected samples after nonnil pointer:\n%v", samplesDump(got))
	}
}

type emptySliceParent struct {
	Items []flatChild `metric:"flatten"`
}

func TestFlatten_EmptySlice_SilentZero(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	sm := bindShowType(reg, reflect.TypeOf(emptySliceParent{}))

	sm.emit(reflect.ValueOf(emptySliceParent{Items: nil}), nil, nil)
	got := onlyApp(reg.AppendSnapshot(nil, SnapshotOptions{}))
	if len(got) != 0 {
		t.Fatalf("expected no samples for empty slice flatten, got %v", samplesDump(got))
	}
}

type valueAndFlattenSameField struct {
	Bad uint64 `metric:"flatten,name=bad,type=counter,help=x"`
}

func TestFlatten_RejectValueMetricCombination(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when flatten combined with value metric")
		}
	}()
	bindShowType(NewRegistry(), reflect.TypeOf(valueAndFlattenSameField{}))
}

type flattenOnString struct {
	Bad string `metric:"flatten"`
}

func TestFlatten_RejectNonAggregateKind(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when flatten on string field")
		}
	}()
	bindShowType(NewRegistry(), reflect.TypeOf(flattenOnString{}))
}

type cyclicFlatten struct {
	Next *cyclicFlatten `metric:"flatten"`
}

func TestFlatten_RejectDirectCycle(t *testing.T) {
	t.Parallel()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on direct cyclic flatten path")
		}
		msg := pa(r)
		if !strings.Contains(msg, "cyclic") {
			t.Fatalf("expected cycle message, got %q", msg)
		}
	}()
	bindShowType(NewRegistry(), reflect.TypeOf(cyclicFlatten{}))
}

type indirectA struct {
	B *indirectB `metric:"flatten"`
}
type indirectB struct {
	A *indirectA `metric:"flatten"`
}

func TestFlatten_RejectIndirectCycle(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on indirect cyclic flatten path")
		}
	}()
	bindShowType(NewRegistry(), reflect.TypeOf(indirectA{}))
}

type collisionInner struct {
	Area string `metric:"label=area"`
	V    uint64 `metric:"name=collision.v,type=gauge,help=..."`
}

type collisionOuter struct {
	Area  string           `metric:"label=area"`
	Inner []collisionInner `metric:"flatten"`
}

func TestFlatten_RejectLabelNameCollision(t *testing.T) {
	t.Parallel()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on combined label-name collision")
		}
		msg := pa(r)
		if !strings.Contains(msg, "collide") {
			t.Fatalf("expected collision message, got %q", msg)
		}
	}()
	bindShowType(NewRegistry(), reflect.TypeOf(collisionOuter{}))
}

type dupNameLeaf struct {
	K string `metric:"label"`
	V uint64 `metric:"name=dup.shared,type=gauge,help=..."`
}
type dupNameParent struct {
	A []dupNameLeaf `metric:"flatten"`
	B []dupNameLeaf `metric:"flatten"`
}

func TestFlatten_RejectDuplicateMetricNamesAcrossPaths(t *testing.T) {
	t.Parallel()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on duplicate metric name through different flatten paths")
		}
		msg := pa(r)
		if !strings.Contains(msg, "duplicate metric name") {
			t.Fatalf("expected duplicate-name message, got %q", msg)
		}
	}()
	bindShowType(NewRegistry(), reflect.TypeOf(dupNameParent{}))
}

// samplesMatch checks that each entry in want appears in got with the
// expected value. Extra samples in got (e.g. registry-internal metrics)
// are ignored, and samples whose Name is in want must match exactly.
func samplesMatch(got []Sample, want map[string]float64) bool {
	seen := make(map[string]bool, len(want))
	for _, s := range got {
		key := s.Name + "{" + labelsToString(s.Labels) + "}"
		if w, ok := want[key]; ok {
			if w != s.Value {
				return false
			}
			seen[key] = true
		}
	}
	return len(seen) == len(want)
}

func samplesDump(got []Sample) string {
	var b strings.Builder
	for _, s := range got {
		fmt.Fprintf(&b, "%s{%s} = %v\n", s.Name, labelsToString(s.Labels), s.Value)
	}
	return b.String()
}

// onlyApp filters out the SDK's internal observability metrics so callers
// can assert the application surface alone.
func onlyApp(got []Sample) []Sample {
	out := make([]Sample, 0, len(got))
	for _, s := range got {
		if strings.HasPrefix(s.Name, "telemetry.") {
			continue
		}
		out = append(out, s)
	}
	return out
}

func labelsToString(lbls []LabelPair) string {
	parts := make([]string, len(lbls))
	for i, l := range lbls {
		parts[i] = l.Name + "=" + l.Value
	}
	return strings.Join(parts, ",")
}

func pa(r any) string {
	if e, ok := r.(error); ok {
		return e.Error()
	}
	return ""
}
