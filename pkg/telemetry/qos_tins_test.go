// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"reflect"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/southbound"
)

// Regression: SchedulerTinState once had no tin label, so all eight tins of
// a scheduler flattened into one series - gauges kept the last tin's value
// and counters accumulated every tin's delta. This pins the real southbound
// type, not a lookalike, so removing the label breaks the build here.
func TestQoSSchedulerTins_EmitPerTinSeries(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	sm := bindShowType(reg, reflect.TypeOf(southbound.SchedulerState{}))

	state := southbound.SchedulerState{
		SwIfIndex:     7,
		InterfaceName: "ipoe0",
		TinMode:       "diffserv4",
		Tins: []southbound.SchedulerTinState{
			{Tin: 0, Drops: 3, SparseFlows: 2},
			{Tin: 1, Drops: 11, SparseFlows: 9},
		},
	}
	sm.emit(reflect.ValueOf(state), nil, nil)

	got := reg.AppendSnapshot(nil, SnapshotOptions{})
	want := map[string]float64{
		`qos.scheduler.tin.drops{sw_if_index=7,interface=ipoe0,tin_mode=diffserv4,tin=0}`:        3,
		`qos.scheduler.tin.drops{sw_if_index=7,interface=ipoe0,tin_mode=diffserv4,tin=1}`:        11,
		`qos.scheduler.tin.sparse_flows{sw_if_index=7,interface=ipoe0,tin_mode=diffserv4,tin=0}`: 2,
		`qos.scheduler.tin.sparse_flows{sw_if_index=7,interface=ipoe0,tin_mode=diffserv4,tin=1}`: 9,
	}
	if !samplesMatch(got, want) {
		t.Fatalf("unexpected samples:\n%v", samplesDump(got))
	}
}
