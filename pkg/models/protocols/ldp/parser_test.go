// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ldp

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

func TestNeighbors_Parse(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "neighbor.json")

	var wrapper struct {
		Neighbors []Neighbor `json:"neighbors"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		t.Fatalf("unmarshal neighbor: %v", err)
	}
	if len(wrapper.Neighbors) == 0 {
		t.Fatal("expected at least one neighbor in fixture")
	}
	for i, n := range wrapper.Neighbors {
		if n.NeighborId == "" {
			t.Errorf("neighbor %d: NeighborId unset", i)
		}
		if n.State == "" {
			t.Errorf("neighbor %d: State unset", i)
		}
		if n.AddressFamily == "" {
			t.Errorf("neighbor %d: AddressFamily unset", i)
		}
		if n.UpTime == "" {
			t.Errorf("neighbor %d: UpTime unset", i)
		}
	}
}

func TestBindings_Parse(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "binding.json")

	var wrapper struct {
		Bindings []Binding `json:"bindings"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		t.Fatalf("unmarshal binding: %v", err)
	}
	if len(wrapper.Bindings) == 0 {
		t.Fatal("expected at least one binding in fixture")
	}

	var sawInUse bool
	for i, b := range wrapper.Bindings {
		if b.Prefix == "" {
			t.Errorf("binding %d: Prefix unset", i)
		}
		if b.AddressFamily == "" {
			t.Errorf("binding %d: AddressFamily unset", i)
		}
		if b.InUse == 1 {
			sawInUse = true
		}
	}
	if !sawInUse {
		t.Log("note: no bindings had InUse=1 in fixture; that's OK if topology has nothing forwarded")
	}
}

func TestDiscovery_Parse(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "discovery.json")

	var wrapper struct {
		Adjacencies []Discovery `json:"adjacencies"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		t.Fatalf("unmarshal discovery: %v", err)
	}
	if len(wrapper.Adjacencies) == 0 {
		t.Fatal("expected at least one adjacency in fixture")
	}
	for i, d := range wrapper.Adjacencies {
		if d.NeighborId == "" {
			t.Errorf("adjacency %d: NeighborId unset", i)
		}
		if d.Interface == "" {
			t.Errorf("adjacency %d: Interface unset", i)
		}
		if d.HelloHoldtime == 0 {
			t.Errorf("adjacency %d: HelloHoldtime zero", i)
		}
	}
}

func TestEmptyResponses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
	}{
		{"empty neighbors array", `{"neighbors":[]}`},
		{"missing neighbors key", `{}`},
		{"empty bindings array", `{"bindings":[]}`},
		{"missing bindings key", `{}`},
		{"empty adjacencies array", `{"adjacencies":[]}`},
		{"missing adjacencies key", `{}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var w struct {
				Neighbors   []Neighbor  `json:"neighbors"`
				Bindings    []Binding   `json:"bindings"`
				Adjacencies []Discovery `json:"adjacencies"`
			}
			if err := json.Unmarshal([]byte(tc.body), &w); err != nil {
				t.Fatalf("unmarshal %s: %v", tc.name, err)
			}
			// All three slices should be safe to range over.
			for range w.Neighbors {
			}
			for range w.Bindings {
			}
			for range w.Adjacencies {
			}
		})
	}
}

func TestUnmodeledKeysAreDroppedSilently(t *testing.T) {
	t.Parallel()
	junked := []byte(`{
		"addressFamily": "ipv4",
		"neighborId": "10.0.0.1",
		"state": "OPERATIONAL",
		"junkField1": "ignored",
		"junkField2": {"nested": 42}
	}`)
	var n Neighbor
	if err := json.Unmarshal(junked, &n); err != nil {
		t.Fatalf("unmarshal junked neighbor: %v", err)
	}
	if n.AddressFamily != "ipv4" || n.NeighborId != "10.0.0.1" || n.State != "OPERATIONAL" {
		t.Errorf("modeled fields wrong: %+v", n)
	}
}
