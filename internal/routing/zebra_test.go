// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package routing

import (
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/models/protocols/zebra"
)

func TestZebraRouteSummaryParsesCapturedShape(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "zebra-route-ipv4-summary.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var s zebra.RouteSummary
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if s.RoutesTotal != 4 {
		t.Errorf("RoutesTotal = %d, want 4", s.RoutesTotal)
	}
	if s.RoutesTotalFib != 4 {
		t.Errorf("RoutesTotalFib = %d, want 4", s.RoutesTotalFib)
	}
	if len(s.Routes) != 3 {
		t.Fatalf("len(Routes) = %d, want 3 (kernel, connected, static)", len(s.Routes))
	}
	byType := map[string]zebra.RouteSummaryItem{}
	for _, r := range s.Routes {
		byType[r.Type] = r
	}
	if got := byType["connected"]; got.FIB != 2 || got.RIB != 2 {
		t.Errorf("connected = %+v, want FIB=2 RIB=2", got)
	}
	if got := byType["static"]; got.FIB != 1 || got.RIB != 1 {
		t.Errorf("static = %+v, want FIB=1 RIB=1", got)
	}
	if got := byType["kernel"]; got.FIB != 1 || got.RIB != 1 {
		t.Errorf("kernel = %+v, want FIB=1 RIB=1", got)
	}
}

func TestZebraRoutePerPrefixIsKeyedByPrefix(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "zebra-route-ipv4-prefix.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := m["192.0.2.0/24"]; !ok {
		t.Errorf("expected key 192.0.2.0/24, got %v", keys(m))
	}
}

func TestZebraInterfaceIsKeyedByName(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "zebra-interface.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	wantNames := []string{"CUSTOMER-A", "dummy-custa", "lo"}
	for _, n := range wantNames {
		if _, ok := m[n]; !ok {
			t.Errorf("expected interface %q in response, got %v", n, keys(m))
		}
	}
}

func TestVRFPrefix(t *testing.T) {
	cases := map[string]struct {
		want    string
		wantErr bool
	}{
		"":               {"", false},
		"CUSTOMER-A":     {"vrf CUSTOMER-A ", false},
		"all":            {"vrf all ", false},
		"with space":     {"", true},
		"semi;injection": {"", true},
		"$(evil)":        {"", true},
	}
	for in, want := range cases {
		got, err := vrfPrefix(in)
		if (err != nil) != want.wantErr {
			t.Errorf("vrfPrefix(%q) err=%v wantErr=%v", in, err, want.wantErr)
			continue
		}
		if !want.wantErr && got != want.want {
			t.Errorf("vrfPrefix(%q) = %q, want %q", in, got, want.want)
		}
	}
}

func TestAFIKeyword(t *testing.T) {
	for in, want := range map[string]string{"ip": "ip", "ipv4": "ip", "ipv6": "ipv6"} {
		if got := afiKeyword(in); got != want {
			t.Errorf("afiKeyword(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestVRFDisplayName(t *testing.T) {
	if got := vrfDisplayName(""); got != "default" {
		t.Errorf("vrfDisplayName empty = %q, want default", got)
	}
	if got := vrfDisplayName("CUSTOMER-A"); got != "CUSTOMER-A" {
		t.Errorf("vrfDisplayName named = %q, want CUSTOMER-A", got)
	}
}

func TestValidInterfaceNameRegex(t *testing.T) {
	for _, ok := range []string{"lo", "eth0", "dummy-custa", "GigabitEthernet0.100", "bond0_1"} {
		if !validInterfaceNameRE.MatchString(ok) {
			t.Errorf("expected %q to match validInterfaceNameRE", ok)
		}
	}
	for _, bad := range []string{"eth0; rm -rf /", "with space", "$(evil)"} {
		if validInterfaceNameRE.MatchString(bad) {
			t.Errorf("expected %q to NOT match validInterfaceNameRE", bad)
		}
	}
}

func TestPrefixParseRoundTrip(t *testing.T) {
	for _, in := range []string{"10.0.0.0/24", "192.0.2.0/24", "2001:db8::/32"} {
		p, err := netip.ParsePrefix(in)
		if err != nil {
			t.Errorf("ParsePrefix(%q): %v", in, err)
			continue
		}
		if p.String() != in {
			t.Errorf("ParsePrefix(%q).String() = %q", in, p.String())
		}
	}
}

func keys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
