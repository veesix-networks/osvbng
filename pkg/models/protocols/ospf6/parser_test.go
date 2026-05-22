// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf6

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func TestInstance_Parse(t *testing.T) {
	t.Parallel()
	var inst Instance
	if err := json.Unmarshal(loadFixture(t, "instance.json"), &inst); err != nil {
		t.Fatalf("unmarshal instance: %v", err)
	}
	if inst.RouterID != "10.254.0.1" {
		t.Errorf("RouterID = %q, want 10.254.0.1", inst.RouterID)
	}
	if inst.NumberOfAreaInRouter != 1 {
		t.Errorf("NumberOfAreaInRouter = %d, want 1", inst.NumberOfAreaInRouter)
	}
	if inst.MaximumPaths != 256 {
		t.Errorf("MaximumPaths = %d, want 256", inst.MaximumPaths)
	}
	area, ok := inst.Areas["0.0.0.0"]
	if !ok {
		t.Fatalf("Areas missing 0.0.0.0; got %v", keys(inst.Areas))
	}
	if area.NumberOfAreaScopedLsa == 0 {
		t.Errorf("Areas[0.0.0.0].NumberOfAreaScopedLsa zero; fixture has >0")
	}
}

func TestInterface_Parse(t *testing.T) {
	t.Parallel()
	out := map[string]Interface{}
	if err := json.Unmarshal(loadFixture(t, "interface.json"), &out); err != nil {
		t.Fatalf("unmarshal interface: %v", err)
	}
	eth2, ok := out["eth2"]
	if !ok {
		t.Fatalf("eth2 missing; got %v", keys(out))
	}
	if eth2.AreaID != "0.0.0.0" {
		t.Errorf("eth2.AreaID = %q, want 0.0.0.0", eth2.AreaID)
	}
	if eth2.Cost != 10 {
		t.Errorf("eth2.Cost = %d, want 10", eth2.Cost)
	}
	if eth2.TimerIntervalsConfigHello != 10 {
		t.Errorf("eth2.TimerIntervalsConfigHello = %d, want 10", eth2.TimerIntervalsConfigHello)
	}
	if eth2.InterfaceMtu != 1500 {
		t.Errorf("eth2.InterfaceMtu = %d, want 1500", eth2.InterfaceMtu)
	}
	if !eth2.AttachedToArea {
		t.Errorf("eth2.AttachedToArea = false, want true")
	}
}

func TestInterfaceTraffic_Parse(t *testing.T) {
	t.Parallel()
	out := map[string]InterfaceTraffic{}
	if err := json.Unmarshal(loadFixture(t, "interface-traffic.json"), &out); err != nil {
		t.Fatalf("unmarshal interface-traffic: %v", err)
	}
	eth2, ok := out["eth2"]
	if !ok {
		t.Fatalf("eth2 missing; got %v", keys(out))
	}
	if eth2.HelloTx == 0 {
		t.Error("HelloTx zero; fixture has helloTx>0")
	}
	if eth2.HelloRx == 0 {
		t.Error("HelloRx zero; fixture has helloRx>0")
	}
}

func TestGRHelper_Parse(t *testing.T) {
	t.Parallel()
	var gr GRHelper
	if err := json.Unmarshal(loadFixture(t, "gr-helper.json"), &gr); err != nil {
		t.Fatalf("unmarshal gr-helper: %v", err)
	}
	if gr.RouterID != "10.254.0.1" {
		t.Errorf("RouterID = %q, want 10.254.0.1", gr.RouterID)
	}
	if gr.SupportedGracePeriod != 1800 {
		t.Errorf("SupportedGracePeriod = %d, want 1800", gr.SupportedGracePeriod)
	}
	if gr.HelperSupport != "Disabled" {
		t.Errorf("HelperSupport = %q, want Disabled", gr.HelperSupport)
	}
}

func TestNeighbor_Parse(t *testing.T) {
	t.Parallel()
	var resp struct {
		Neighbors []Neighbor `json:"neighbors"`
	}
	if err := json.Unmarshal(loadFixture(t, "neighbor.json"), &resp); err != nil {
		t.Fatalf("unmarshal neighbor: %v", err)
	}
	if len(resp.Neighbors) != 1 {
		t.Fatalf("expected 1 neighbor, got %d", len(resp.Neighbors))
	}
	n := resp.Neighbors[0]
	if n.NeighborID != "10.254.0.2" {
		t.Errorf("NeighborID = %q, want 10.254.0.2", n.NeighborID)
	}
	if n.State != "Full" {
		t.Errorf("State = %q, want Full", n.State)
	}
	if n.InterfaceName != "eth2" {
		t.Errorf("InterfaceName = %q, want eth2", n.InterfaceName)
	}
}

func TestRouteResponse_Parse(t *testing.T) {
	t.Parallel()
	var r RouteResponse
	if err := json.Unmarshal(loadFixture(t, "route.json"), &r); err != nil {
		t.Fatalf("unmarshal route: %v", err)
	}
	if len(r.Routes) == 0 {
		t.Fatal("Routes empty")
	}
}

func TestDatabaseRouter_RawForward(t *testing.T) {
	t.Parallel()
	raw := loadFixture(t, "database-router.json")
	var any map[string]json.RawMessage
	if err := json.Unmarshal(raw, &any); err != nil {
		t.Fatalf("LSDB response should be valid JSON: %v", err)
	}
	if _, ok := any["areaScopedLinkStateDb"]; !ok {
		t.Errorf("database-router.json missing 'areaScopedLinkStateDb' top-level key; got %v", keysRaw(any))
	}
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func keysRaw(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
