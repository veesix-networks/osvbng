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

func TestNeighborDetail_Parse(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "neighbor-detail.json")

	var raw map[string]NeighborDetail
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal neighbor-detail: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected at least one neighbor in fixture")
	}
	for ip, d := range raw {
		if d.PeerId == "" {
			t.Errorf("neighbor %s: PeerId unset", ip)
		}
		if d.TCPLocalPort == 0 {
			t.Errorf("neighbor %s: TCPLocalPort zero", ip)
		}
		if d.State == "" {
			t.Errorf("neighbor %s: State unset", ip)
		}
		if len(d.SentMessages) == 0 {
			t.Errorf("neighbor %s: SentMessages raw payload empty", ip)
		}
	}
}

func TestCapabilities_Parse(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "capabilities.json")

	var wrapper struct {
		Capabilities []CapabilityTLV `json:"capabilities"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		t.Fatalf("unmarshal capabilities: %v", err)
	}
	if len(wrapper.Capabilities) == 0 {
		t.Fatal("expected at least one capability TLV")
	}
	for i, c := range wrapper.Capabilities {
		if c.Description == "" {
			t.Errorf("tlv %d: Description unset", i)
		}
		if c.TLVType == "" {
			t.Errorf("tlv %d: TLVType unset", i)
		}
	}
}

func TestNeighborCapabilities_Parse(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "neighbor-capabilities.json")

	var raw map[string]NeighborCapabilities
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal neighbor-capabilities: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected at least one neighbor in fixture")
	}
	for ip, caps := range raw {
		if len(caps.SentCapabilities) == 0 && len(caps.ReceivedCapabilities) == 0 {
			t.Errorf("neighbor %s: both sent and received capabilities empty", ip)
		}
	}
}

func TestIGPSync_Parse(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "igp-sync.json")

	var raw map[string]IGPSync
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal igp-sync: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected at least one interface in fixture")
	}
	for iface, s := range raw {
		if iface == "" {
			t.Error("interface key empty")
		}
		if s.State == "" {
			t.Errorf("interface %s: State unset", iface)
		}
	}
}

func TestInterface_Parse(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "interface.json")

	var raw map[string]Interface
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal interface: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected at least one interface in fixture")
	}
	for key, iface := range raw {
		if key == "" {
			t.Error("composite key empty")
		}
		if iface.State == "" {
			t.Errorf("entry %s: State unset", key)
		}
		if iface.HelloInterval == 0 {
			t.Errorf("entry %s: HelloInterval zero", key)
		}
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
