// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"reflect"
	"testing"
)

type nestedLeaf struct {
	Peer  string `metric:"label=peer,map_key"`
	Bytes uint64 `metric:"name=nested.bytes,type=counter,help=..."`
}

type nestedGroup struct {
	VRF   string                `metric:"label=vrf,map_key"`
	Peers map[string]nestedLeaf `metric:"flatten"`
}

type nestedRoot struct {
	Groups map[string]nestedGroup `metric:"flatten"`
}

// TestNestedMapKey_TwoLevels exercises map-flatten inside map-flatten:
// outer map keyed by VRF, value-struct contains an inner flatten map keyed by
// peer. Both keys must project into the emitted label tuple.
func TestNestedMapKey_TwoLevels(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	sm := bindShowType(reg, reflect.TypeOf(nestedRoot{}))

	src := nestedRoot{
		Groups: map[string]nestedGroup{
			"default": {Peers: map[string]nestedLeaf{
				"10.0.0.1": {Bytes: 100},
				"10.0.0.2": {Bytes: 200},
			}},
			"CUSTOMER-A": {Peers: map[string]nestedLeaf{
				"10.1.0.1": {Bytes: 300},
			}},
		},
	}
	sm.walk(reflect.ValueOf(src), nil)

	got := reg.AppendSnapshot(nil, SnapshotOptions{})
	want := map[string]float64{
		`nested.bytes{vrf=default,peer=10.0.0.1}`:    100,
		`nested.bytes{vrf=default,peer=10.0.0.2}`:    200,
		`nested.bytes{vrf=CUSTOMER-A,peer=10.1.0.1}`: 300,
	}
	if !samplesMatch(got, want) {
		t.Fatalf("unexpected samples:\n%v", samplesDump(onlyApp(got)))
	}
}
