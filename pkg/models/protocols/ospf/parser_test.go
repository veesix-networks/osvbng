// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf

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
	if inst.AttachedAreaCounter != 1 {
		t.Errorf("AttachedAreaCounter = %d, want 1", inst.AttachedAreaCounter)
	}
	if inst.MaximumPaths != 256 {
		t.Errorf("MaximumPaths = %d, want 256", inst.MaximumPaths)
	}
	area, ok := inst.Areas["0.0.0.0"]
	if !ok {
		t.Fatalf("Areas missing 0.0.0.0; got %v", keys(inst.Areas))
	}
	if !area.Backbone {
		t.Error("Areas[0.0.0.0].Backbone = false, want true")
	}
	if area.LsaRouterNumber != 2 {
		t.Errorf("Areas[0.0.0.0].LsaRouterNumber = %d, want 2", area.LsaRouterNumber)
	}
	if area.NbrFullAdjacentCounter != 1 {
		t.Errorf("Areas[0.0.0.0].NbrFullAdjacentCounter = %d, want 1", area.NbrFullAdjacentCounter)
	}
}

func TestInstance_VRFAllParse(t *testing.T) {
	t.Parallel()
	out := map[string]Instance{}
	if err := json.Unmarshal(loadFixture(t, "instance-vrf-all.json"), &out); err != nil {
		t.Fatalf("unmarshal instance-vrf-all: %v", err)
	}
	def, ok := out["default"]
	if !ok {
		t.Fatalf("missing 'default' vrf entry; got %v", keys(out))
	}
	if def.VRFName != "default" {
		t.Errorf("VRFName = %q, want default", def.VRFName)
	}
	if def.RouterID != "10.254.0.1" {
		t.Errorf("RouterID = %q, want 10.254.0.1", def.RouterID)
	}
	if len(def.Areas) == 0 {
		t.Error("Areas empty under vrf=default")
	}
}

func TestInterfaceMap_Parse(t *testing.T) {
	t.Parallel()
	var im InterfaceMap
	if err := json.Unmarshal(loadFixture(t, "interface.json"), &im); err != nil {
		t.Fatalf("unmarshal interface: %v", err)
	}
	if len(im.Interfaces) == 0 {
		t.Fatal("Interfaces empty")
	}
	eth2, ok := im.Interfaces["eth2"]
	if !ok {
		t.Fatalf("eth2 missing; got %v", keys(im.Interfaces))
	}
	if eth2.Area != "0.0.0.0" {
		t.Errorf("eth2.Area = %q, want 0.0.0.0", eth2.Area)
	}
	if eth2.Cost != 10 {
		t.Errorf("eth2.Cost = %d, want 10", eth2.Cost)
	}
	if eth2.TimerMsecs != 10000 {
		t.Errorf("eth2.TimerMsecs = %d, want 10000", eth2.TimerMsecs)
	}
	if !eth2.IfUp || !eth2.OspfEnabled {
		t.Errorf("eth2 IfUp=%v OspfEnabled=%v, want both true", eth2.IfUp, eth2.OspfEnabled)
	}
}

func TestInterfaceMap_VRFAllParse(t *testing.T) {
	t.Parallel()
	out := map[string]InterfaceMap{}
	if err := json.Unmarshal(loadFixture(t, "interface-vrf-all.json"), &out); err != nil {
		t.Fatalf("unmarshal interface-vrf-all: %v", err)
	}
	def, ok := out["default"]
	if !ok {
		t.Fatalf("missing 'default' vrf entry; got %v", keys(out))
	}
	if def.VRFName != "default" {
		t.Errorf("VRFName = %q, want default", def.VRFName)
	}
	if _, ok := def.Interfaces["eth2"]; !ok {
		t.Errorf("default vrf missing eth2; got %v", keys(def.Interfaces))
	}
}

func TestNeighborDetail_Parse(t *testing.T) {
	t.Parallel()
	var ndm NeighborDetailMap
	if err := json.Unmarshal(loadFixture(t, "neighbor-detail.json"), &ndm); err != nil {
		t.Fatalf("unmarshal neighbor-detail: %v", err)
	}
	entries, ok := ndm.Neighbors["10.254.0.2"]
	if !ok {
		t.Fatalf("router-id 10.254.0.2 missing; got %v", keys(ndm.Neighbors))
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry for 10.254.0.2, got %d", len(entries))
	}
	n := entries[0]
	if n.AreaID != "0.0.0.0" {
		t.Errorf("AreaID = %q, want 0.0.0.0", n.AreaID)
	}
	if n.IfaceName != "eth2" {
		t.Errorf("IfaceName = %q, want eth2", n.IfaceName)
	}
	if n.StateChangeCounter == 0 {
		t.Error("StateChangeCounter zero; fixture has stateChangeCounter>0")
	}
	if n.NbrState != "Full/-" {
		t.Errorf("NbrState = %q, want Full/-", n.NbrState)
	}
}

func TestNeighborDetail_VRFAllParse(t *testing.T) {
	t.Parallel()
	out := map[string]NeighborDetailMap{}
	if err := json.Unmarshal(loadFixture(t, "neighbor-detail-vrf-all.json"), &out); err != nil {
		t.Fatalf("unmarshal neighbor-detail-vrf-all: %v", err)
	}
	def, ok := out["default"]
	if !ok {
		t.Fatalf("missing 'default' vrf entry; got %v", keys(out))
	}
	if def.VRFName != "default" {
		t.Errorf("VRFName = %q, want default", def.VRFName)
	}
	if _, ok := def.Neighbors["10.254.0.2"]; !ok {
		t.Errorf("default vrf missing neighbor 10.254.0.2; got %v", keys(def.Neighbors))
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
	if gr.StrictLsaCheck != "Enabled" {
		t.Errorf("StrictLsaCheck = %q, want Enabled", gr.StrictLsaCheck)
	}
}

func TestGRHelper_VRFAllParse(t *testing.T) {
	t.Parallel()
	out := map[string]GRHelper{}
	if err := json.Unmarshal(loadFixture(t, "gr-helper-vrf-all.json"), &out); err != nil {
		t.Fatalf("unmarshal gr-helper-vrf-all: %v", err)
	}
	def, ok := out["default"]
	if !ok {
		t.Fatalf("missing 'default' vrf entry; got %v", keys(out))
	}
	if def.VRFName != "default" {
		t.Errorf("VRFName = %q, want default", def.VRFName)
	}
	if def.SupportedGracePeriod != 1800 {
		t.Errorf("SupportedGracePeriod = %d, want 1800", def.SupportedGracePeriod)
	}
}

func TestRoute_Parse(t *testing.T) {
	t.Parallel()
	out := map[string]Route{}
	if err := json.Unmarshal(loadFixture(t, "route.json"), &out); err != nil {
		t.Fatalf("unmarshal route: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("route map empty")
	}
	r, ok := out["10.254.0.2/32"]
	if !ok {
		t.Fatalf("prefix 10.254.0.2/32 missing; got %v", keys(out))
	}
	if r.Cost != 10 {
		t.Errorf("10.254.0.2/32 Cost = %d, want 10", r.Cost)
	}
	if r.Area != "0.0.0.0" {
		t.Errorf("10.254.0.2/32 Area = %q, want 0.0.0.0", r.Area)
	}
	if len(r.Nexthops) != 1 {
		t.Fatalf("10.254.0.2/32 Nexthops = %d, want 1", len(r.Nexthops))
	}
	if r.Nexthops[0].Via != "eth2" {
		t.Errorf("10.254.0.2/32 Nexthops[0].Via = %q, want eth2", r.Nexthops[0].Via)
	}
	if r.Nexthops[0].AdvertisedRouter != "10.254.0.2" {
		t.Errorf("10.254.0.2/32 Nexthops[0].AdvertisedRouter = %q, want 10.254.0.2", r.Nexthops[0].AdvertisedRouter)
	}
}

func TestPlainTextResponseRejectsJSONUnmarshal(t *testing.T) {
	t.Parallel()
	var sink any
	if err := json.Unmarshal(loadFixture(t, "mpls-te-database.json"), &sink); err == nil {
		t.Fatal("expected json.Unmarshal to fail on plain-text FRR response; got nil error")
	}
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
