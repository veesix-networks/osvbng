// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/veesix-networks/osvbng/cmd/osvbngcli/orderedjson"
)

// The fixtures and goldens are shared with tests/qos_checks.py selftest, so
// the robot-suite assertions and these whitespace-exact goldens are pinned
// to identical material.
var updateGoldens = flag.Bool("update", false, "rewrite golden files under tests/fixtures/qos")

const fixtureDir = "../../tests/fixtures/qos"

func loadEnvelopeData(t *testing.T, name string) (string, any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(fixtureDir, name))
	if err != nil {
		t.Fatal(err)
	}
	var env showResponseEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	payload, err := orderedjson.Decode(env.Data)
	if err != nil {
		t.Fatal(err)
	}
	return env.Path, payload
}

func checkGolden(t *testing.T, goldenName, got string) {
	t.Helper()
	path := filepath.Join(fixtureDir, goldenName)
	if *updateGoldens {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden missing (run with -update): %v", err)
	}
	if string(want) != got {
		t.Fatalf("output differs from %s (run with -update after intentional changes):\n--- got ---\n%s--- want ---\n%s", goldenName, got, want)
	}
}

var uuidRe = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

func TestRenderSchedulerTable_Golden(t *testing.T) {
	path, payload := loadEnvelopeData(t, "scheduler_api.json")
	out, ok := renderCompact(path, payload)
	if !ok {
		t.Fatal("renderer refused the scheduler fixture")
	}
	checkGolden(t, "scheduler_cli.txt", out)

	// Session UUIDs must never be truncated, and no cell may fall back to
	// a JSON blob.
	if got := len(uuidRe.FindAllString(out, -1)); got != 5 {
		t.Fatalf("expected 5 full-length session UUIDs, found %d", got)
	}
	if strings.Contains(out, "{") {
		t.Fatal("compact table contains a JSON blob")
	}
	// The v1-fallback row renders placeholders, not zeros pretending to be
	// state.
	if !strings.Contains(out, "(none)") {
		t.Fatal("v1-fallback row lost its (none) parent marker")
	}
}

func TestRenderAggregateTree_Golden(t *testing.T) {
	path, payload := loadEnvelopeData(t, "aggregate_api.json")
	out, ok := renderCompact(path, payload)
	if !ok {
		t.Fatal("renderer refused the aggregate fixture")
	}
	checkGolden(t, "aggregate_cli.txt", out)

	if !strings.Contains(out, "svlan 400-499") {
		t.Fatal("range svlan lost its range rendering")
	}
	if strings.Contains(out, "svlan 100-100") {
		t.Fatal("single-tag svlan rendered as a range")
	}
	if !strings.Contains(out, "(no svlan aggregates)") {
		t.Fatal("bare port lost its no-children marker")
	}
	// Sorted: svlan 100 must precede svlan 300 even though the fixture
	// lists 300 first.
	if strings.Index(out, "svlan 100") > strings.Index(out, "svlan 300") {
		t.Fatal("svlans not sorted by tag")
	}
	if strings.Contains(out, "{") {
		t.Fatal("tree contains a JSON blob")
	}
}

func TestRenderCompact_FallsBackOnUnknownShape(t *testing.T) {
	if _, ok := renderCompact("qos.scheduler", "not a list"); ok {
		t.Fatal("renderer accepted a non-list payload")
	}
	if _, ok := renderCompact("qos.scheduler", []any{"scalar"}); ok {
		t.Fatal("renderer accepted a list of scalars")
	}
	if _, ok := renderCompact("system.threads", []any{}); ok {
		t.Fatal("registry matched an unrelated path")
	}
}

func TestRenderCompact_EmptyList(t *testing.T) {
	out, ok := renderCompact("qos.scheduler", []any{})
	if !ok || out != "No data\n" {
		t.Fatalf("empty list: got %q ok=%v", out, ok)
	}
}

func TestRenderAggregateTree_SvlanWithoutPort(t *testing.T) {
	payload, err := orderedjson.Decode([]byte(`[
		{"sw_if_index": 1, "interface": "eth1", "level": "svlan", "svlan_id": 100, "svlan_id_end": 100,
		 "rate_kbps": 6000, "weight": 1, "effective_weight": 750000, "buffer_usage": 0, "buffer_limit": 1048576,
		 "active_weight": 0, "active_children": 0, "shaped_pkts": 0, "shaped_bytes": 0, "backpressure": 0,
		 "drr_blocked": 0, "parent_blocked": 0}]`))
	if err != nil {
		t.Fatal(err)
	}
	out, ok := renderCompact("qos.aggregate", payload)
	if !ok {
		t.Fatal("renderer refused svlan-only payload")
	}
	if strings.Contains(out, "├─") || strings.Contains(out, "└─") {
		t.Fatalf("filtered svlan block must render flat, got:\n%s", out)
	}
	if !strings.Contains(out, "eth1  svlan 100") {
		t.Fatalf("flat svlan block missing interface prefix:\n%s", out)
	}
}

func TestHumanize(t *testing.T) {
	cases := []struct {
		got, want string
	}{
		{siCount(json.Number("18234511")), "18.2M"},
		{siCount(json.Number("1204")), "1.2K"},
		{siCount(json.Number("0")), "0"},
		{siCount(json.Number("999")), "999"},
		{siRateKbps(json.Number("50000")), "50M"},
		{siRateKbps(json.Number("2000")), "2M"},
		{siRateKbps(json.Number("500")), "500K"},
		{siBytes(json.Number("25900000000")), "25.9G"},
		{siBytesUnit(json.Number("112400000000")), "112.4 GB"},
		{siBytesUnit(json.Number("122")), "122 B"},
		{siBytesUnit(json.Number("1200000")), "1.2 MB"},
		{pct(json.Number("212992"), json.Number("4194304")), "5%"},
		{pct(json.Number("10"), json.Number("0")), "0%"},
		{pct(nil, json.Number("100")), "0%"},
		{siCount("not-a-number"), "not-a-number"},
	}
	for i, c := range cases {
		if c.got != c.want {
			t.Errorf("case %d: got %q want %q", i, c.got, c.want)
		}
	}
}
