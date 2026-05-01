// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"reflect"
	"strings"
	"testing"
)

type ospfNeighbor struct {
	Area   string `json:"area"     metric:"label,map_key"`
	PeerID string `json:"peer_id"  metric:"label"`
	UpSecs uint64 `json:"up_secs"  metric:"name=ospf.neighbor.up_seconds,type=gauge,help=..."`
}

func TestMapKey_StringSlice(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	sm := bindShowType(reg, reflect.TypeOf(ospfNeighbor{}))

	src := map[string][]ospfNeighbor{
		"0.0.0.0": {{PeerID: "1.1.1.1", UpSecs: 10}, {PeerID: "2.2.2.2", UpSecs: 20}},
		"0.0.0.1": {{PeerID: "3.3.3.3", UpSecs: 30}},
	}
	sm.walk(reflect.ValueOf(src), nil)

	got := reg.AppendSnapshot(nil, SnapshotOptions{})
	want := map[string]float64{
		`ospf.neighbor.up_seconds{area=0.0.0.0,peer_id=1.1.1.1}`: 10,
		`ospf.neighbor.up_seconds{area=0.0.0.0,peer_id=2.2.2.2}`: 20,
		`ospf.neighbor.up_seconds{area=0.0.0.1,peer_id=3.3.3.3}`: 30,
	}
	if !samplesMatch(got, want) {
		t.Fatalf("unexpected samples:\n%v", samplesDump(onlyApp(got)))
	}
}

type intKeyed struct {
	ID    uint32 `metric:"label,map_key"`
	Calls uint64 `metric:"name=int.keyed.calls,type=counter,help=..."`
}

func TestMapKey_UintKey_StrconvFormatting(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	sm := bindShowType(reg, reflect.TypeOf(intKeyed{}))

	src := map[uint32]intKeyed{42: {Calls: 5}, 100: {Calls: 12}}
	sm.walk(reflect.ValueOf(src), nil)

	got := reg.AppendSnapshot(nil, SnapshotOptions{})
	want := map[string]float64{
		`int.keyed.calls{id=42}`:  5,
		`int.keyed.calls{id=100}`: 12,
	}
	if !samplesMatch(got, want) {
		t.Fatalf("unexpected samples:\n%v", samplesDump(onlyApp(got)))
	}
}

type signedKeyed struct {
	N uint64 `metric:"name=signed.n,type=gauge,help=..."`
	K int64  `metric:"label,map_key"`
}

func TestMapKey_SignedIntKey(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	sm := bindShowType(reg, reflect.TypeOf(signedKeyed{}))

	src := map[int64]signedKeyed{-1: {N: 1}, 5: {N: 7}}
	sm.walk(reflect.ValueOf(src), nil)

	got := reg.AppendSnapshot(nil, SnapshotOptions{})
	want := map[string]float64{
		`signed.n{k=-1}`: 1,
		`signed.n{k=5}`:  7,
	}
	if !samplesMatch(got, want) {
		t.Fatalf("unexpected samples:\n%v", samplesDump(onlyApp(got)))
	}
}

type boolKeyed struct {
	Enabled bool   `metric:"label,map_key"`
	Hits    uint64 `metric:"name=bool.hits,type=counter,help=..."`
}

func TestMapKey_BoolKey(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	sm := bindShowType(reg, reflect.TypeOf(boolKeyed{}))

	src := map[bool]boolKeyed{true: {Hits: 9}, false: {Hits: 3}}
	sm.walk(reflect.ValueOf(src), nil)

	got := reg.AppendSnapshot(nil, SnapshotOptions{})
	want := map[string]float64{
		`bool.hits{enabled=true}`:  9,
		`bool.hits{enabled=false}`: 3,
	}
	if !samplesMatch(got, want) {
		t.Fatalf("unexpected samples:\n%v", samplesDump(onlyApp(got)))
	}
}

type pointerValue struct {
	Tag string `metric:"label,map_key"`
	V   uint64 `metric:"name=ptrval.v,type=gauge,help=..."`
}

func TestMapKey_PointerValue_NilEntries(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	sm := bindShowType(reg, reflect.TypeOf(pointerValue{}))

	src := map[string]*pointerValue{
		"a": {V: 1},
		"b": nil,
	}
	sm.walk(reflect.ValueOf(src), nil)

	got := reg.AppendSnapshot(nil, SnapshotOptions{})
	want := map[string]float64{`ptrval.v{tag=a}`: 1}
	if !samplesMatch(got, want) {
		t.Fatalf("unexpected samples:\n%v", samplesDump(onlyApp(got)))
	}
}

type noMapKey struct {
	V uint64 `metric:"name=nokey.v,type=gauge,help=..."`
}

func TestMapKey_MissingOnT_DropsKeyLabel(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	sm := bindShowType(reg, reflect.TypeOf(noMapKey{}))

	src := map[string]noMapKey{"x": {V: 4}, "y": {V: 5}}
	sm.walk(reflect.ValueOf(src), nil)

	got := reg.AppendSnapshot(nil, SnapshotOptions{})
	// Both map entries collapse onto the same labelless series; the
	// gauge's Set replaces, so we just assert that *some* value was
	// written and the metric is registered with no labels.
	app := onlyApp(got)
	if len(app) != 1 {
		t.Fatalf("expected exactly one labelless sample, got %v", samplesDump(app))
	}
	if app[0].Name != "nokey.v" || len(app[0].Labels) != 0 {
		t.Fatalf("expected nokey.v with no labels, got %v", samplesDump(app))
	}
}

type dualMapKey struct {
	A string `metric:"label,map_key"`
	B string `metric:"label,map_key"`
	V uint64 `metric:"name=dual.v,type=gauge,help=..."`
}

func TestMapKey_RejectDualMapKey(t *testing.T) {
	t.Parallel()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on two map_key fields")
		}
		if !strings.Contains(pa(r), "more than one map_key") {
			t.Fatalf("unexpected panic: %q", pa(r))
		}
	}()
	bindShowType(NewRegistry(), reflect.TypeOf(dualMapKey{}))
}

type structKeyMap struct {
	Inner struct{ X int } `metric:"label,map_key"`
	V     uint64          `metric:"name=structkey.v,type=gauge,help=..."`
}

func TestMapKey_RejectUnsupportedKind(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on map_key with unsupported field kind")
		}
	}()
	bindShowType(NewRegistry(), reflect.TypeOf(structKeyMap{}))
}

type nestedMapInner struct {
	K string `metric:"label,map_key"`
	V uint64 `metric:"name=nested.v,type=gauge,help=..."`
}
type nestedMapParent struct {
	Pools map[string]map[string]nestedMapInner `metric:"flatten"`
}

func TestMapKey_RejectNestedMapInsideFlatten(t *testing.T) {
	t.Parallel()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on nested map inside flatten")
		}
		if !strings.Contains(pa(r), "map") {
			t.Fatalf("unexpected panic: %q", pa(r))
		}
	}()
	bindShowType(NewRegistry(), reflect.TypeOf(nestedMapParent{}))
}

type readOnlyKeyed struct {
	K string `metric:"label,map_key"`
	V uint64 `metric:"name=readonly.v,type=gauge,help=..."`
}

func TestMapKey_DoesNotMutateSourceMap(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	sm := bindShowType(reg, reflect.TypeOf(readOnlyKeyed{}))

	src := map[string]readOnlyKeyed{
		"a": {V: 1},
		"b": {V: 2},
	}
	before := make(map[string]readOnlyKeyed, len(src))
	for k, v := range src {
		before[k] = v
	}

	sm.walk(reflect.ValueOf(src), nil)

	if !reflect.DeepEqual(src, before) {
		t.Fatalf("walker mutated source map: before=%v after=%v", before, src)
	}
}
