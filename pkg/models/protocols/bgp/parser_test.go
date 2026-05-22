// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

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

func TestNeighbors_AggregateParse(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "neighbors.json")

	var raw map[string]Neighbor
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal neighbors: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected at least one neighbor in fixture")
	}

	for ip, n := range raw {
		if n.NeighborAddr == "" {
			t.Errorf("neighbor %s: NeighborAddr unset", ip)
		}
		if n.BgpState == "" {
			t.Errorf("neighbor %s: BgpState unset", ip)
		}
		if n.RemoteAs == 0 {
			t.Errorf("neighbor %s: RemoteAs zero", ip)
		}
		if n.MessageStats.TotalRecv == 0 && n.MessageStats.OpensRecv == 0 {
			t.Errorf("neighbor %s: MessageStats appears unparsed", ip)
		}
	}
}

func TestStatistics_IPv4Parse(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "statistics_v4.json")

	var wrapper struct {
		IPv4Unicast []Statistics `json:"ipv4Unicast"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		t.Fatalf("unmarshal statistics_v4: %v", err)
	}
	if len(wrapper.IPv4Unicast) == 0 {
		t.Fatal("expected at least one ipv4Unicast entry")
	}
	s := wrapper.IPv4Unicast[0]
	if s.Instance == "" {
		t.Error("Instance unset")
	}
	if s.TotalPrefixes == 0 {
		t.Error("TotalPrefixes zero; fixture should have prefixes")
	}
}

func TestStatistics_IPv6Parse(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "statistics_v6.json")

	var wrapper struct {
		IPv6Unicast []Statistics `json:"ipv6Unicast"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		t.Fatalf("unmarshal statistics_v6: %v", err)
	}
	if len(wrapper.IPv6Unicast) == 0 {
		t.Fatal("expected at least one ipv6Unicast entry")
	}
	s := wrapper.IPv6Unicast[0]
	if s.Instance == "" {
		t.Error("Instance unset")
	}
}

func TestVPNRoutes_PopulatedParse(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "vpn_v4.json")

	var routes VPNRoutes
	if err := json.Unmarshal(data, &routes); err != nil {
		t.Fatalf("unmarshal vpn_v4: %v", err)
	}
	if routes.TotalRoutes == 0 {
		t.Error("TotalRoutes zero; fixture should have routes")
	}
	if routes.TotalPaths == 0 {
		t.Error("TotalPaths zero")
	}
	if len(routes.Routes) == 0 {
		t.Error("Routes raw payload empty")
	}
}

func TestVPNRoutes_EmptyParse(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "vpn_v6.json")

	var routes VPNRoutes
	if err := json.Unmarshal(data, &routes); err != nil {
		t.Fatalf("unmarshal vpn_v6 (empty): %v", err)
	}
	if routes.TotalRoutes != 0 || routes.TotalPaths != 0 {
		t.Errorf("expected zero totals on empty fixture, got routes=%d paths=%d",
			routes.TotalRoutes, routes.TotalPaths)
	}
}

func TestVPNSummary_PopulatedParse(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "vpn_v4_summary.json")

	var summary VPNSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("unmarshal vpn_v4_summary: %v", err)
	}
	if summary.PeerCount == 0 {
		t.Error("PeerCount zero; fixture should have peers")
	}
	if len(summary.Peers) == 0 {
		t.Error("Peers map empty")
	}
	for ip, p := range summary.Peers {
		if p.State == "" {
			t.Errorf("peer %s: State unset", ip)
		}
	}
}

func TestVPNSummary_EmptyParse(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "vpn_v6_summary.json")

	var summary VPNSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("unmarshal vpn_v6_summary (empty): %v", err)
	}
	if summary.PeerCount != 0 || len(summary.Peers) != 0 {
		t.Errorf("expected zero peers on empty fixture, got count=%d len=%d",
			summary.PeerCount, len(summary.Peers))
	}
}

func TestUnmodeledKeysAreDroppedSilently(t *testing.T) {
	t.Parallel()
	// Craft a fixture with extra junk keys alongside known fields.
	junked := []byte(`{
		"neighborAddr": "10.0.0.1",
		"bgpState": "Established",
		"remoteAs": 65001,
		"junkKey1": "ignored",
		"junkKey2": {"nested": "also ignored"},
		"messageStats": {"totalRecv": 5}
	}`)
	var n Neighbor
	if err := json.Unmarshal(junked, &n); err != nil {
		t.Fatalf("unmarshal junked neighbor: %v", err)
	}
	if n.NeighborAddr != "10.0.0.1" || n.BgpState != "Established" || n.RemoteAs != 65001 {
		t.Errorf("modeled fields wrong: %+v", n)
	}
	if n.MessageStats.TotalRecv != 5 {
		t.Errorf("nested modeled field wrong: %d", n.MessageStats.TotalRecv)
	}
}

func TestMissingKeyAndEmptyArrayDistinction(t *testing.T) {
	t.Parallel()

	// Missing entire `peers` key.
	missing := []byte(`{"as": 65000, "peerCount": 0}`)
	var s1 VPNSummary
	if err := json.Unmarshal(missing, &s1); err != nil {
		t.Fatalf("unmarshal missing-key: %v", err)
	}
	if s1.Peers != nil {
		t.Errorf("missing key should leave Peers nil, got len=%d", len(s1.Peers))
	}

	// Present but empty map.
	empty := []byte(`{"as": 65000, "peerCount": 0, "peers": {}}`)
	var s2 VPNSummary
	if err := json.Unmarshal(empty, &s2); err != nil {
		t.Fatalf("unmarshal empty-map: %v", err)
	}
	if s2.Peers == nil {
		t.Error("empty map should leave Peers non-nil")
	}
	if len(s2.Peers) != 0 {
		t.Errorf("empty map should have zero entries, got %d", len(s2.Peers))
	}
}

func TestSummaryAFI_Parse(t *testing.T) {
	t.Parallel()
	var s SummaryAFI
	if err := json.Unmarshal(loadFixture(t, "ipv4-unicast-summary.json"), &s); err != nil {
		t.Fatalf("unmarshal ipv4 unicast summary: %v", err)
	}
	if s.RouterID != "10.254.0.1" {
		t.Errorf("RouterID = %q, want 10.254.0.1", s.RouterID)
	}
	if s.AS != 65000 {
		t.Errorf("AS = %d, want 65000", s.AS)
	}
	if s.VRFName != "default" {
		t.Errorf("VRFName = %q, want default", s.VRFName)
	}
	if s.PeerCount != 1 {
		t.Errorf("PeerCount = %d, want 1", s.PeerCount)
	}
	peer, ok := s.Peers["10.254.0.2"]
	if !ok {
		t.Fatalf("missing peer 10.254.0.2; got %v", peerKeys(s.Peers))
	}
	if peer.State != "Established" {
		t.Errorf("Peer.State = %q, want Established", peer.State)
	}
	if peer.RemoteAS != 65000 {
		t.Errorf("Peer.RemoteAS = %d, want 65000", peer.RemoteAS)
	}
}

func peerKeys(m map[string]SummaryPeer) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestNeighborsAll_Parse(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "bgp-neighbors-vrf-all.json")

	var raw map[string]VRFNeighborSummary
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal bgp-neighbors-vrf-all: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected at least one VRF in fixture")
	}
	def, ok := raw["default"]
	if !ok {
		t.Fatal("missing default VRF in fixture")
	}
	if len(def.Neighbors) == 0 {
		t.Fatal("default VRF has no neighbor entries; UnmarshalJSON likely dropped them")
	}
	n, ok := def.Neighbors["10.254.0.2"]
	if !ok {
		t.Fatalf("missing neighbor 10.254.0.2; got %v", neighborSummaryKeys(def.Neighbors))
	}
	if n.NeighborAddr != "10.254.0.2" {
		t.Errorf("NeighborAddr = %q, want 10.254.0.2", n.NeighborAddr)
	}
	if n.State != "Established" {
		t.Errorf("State = %q, want Established", n.State)
	}
	if n.Up != 1 {
		t.Errorf("Up = %d, want 1 (Established → up)", n.Up)
	}
	if n.UptimeMS == 0 {
		t.Error("UptimeMS zero; fixture should have non-zero uptime")
	}
}

func neighborSummaryKeys(m map[string]NeighborSummary) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestVPNStatistics_Parse(t *testing.T) {
	t.Parallel()
	data := loadFixture(t, "vpn-ipv4-statistics.json")

	var wrapper struct {
		IPv4Vpn []VPNStatistics `json:"ipv4Vpn"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		t.Fatalf("unmarshal vpn-ipv4-statistics: %v", err)
	}
	if len(wrapper.IPv4Vpn) == 0 {
		t.Fatal("expected at least one ipv4Vpn entry")
	}
	for i, s := range wrapper.IPv4Vpn {
		if s.Instance == "" {
			t.Errorf("entry %d: Instance unset", i)
		}
	}
	first := wrapper.IPv4Vpn[0]
	if first.Instance != "VRF default" {
		t.Errorf("first.Instance = %q, want %q", first.Instance, "VRF default")
	}
	if first.TotalPrefixes == 0 {
		t.Error("first.TotalPrefixes zero; default VRF fixture should advertise prefixes")
	}
}
