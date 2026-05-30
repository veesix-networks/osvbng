// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package aaa

import (
	"net"
	"testing"
)

func mustMAC(t *testing.T, s string) net.HardwareAddr {
	t.Helper()
	mac, err := net.ParseMAC(s)
	if err != nil {
		t.Fatalf("parse mac %q: %v", s, err)
	}
	return mac
}

func TestExpandPasswordEmpty(t *testing.T) {
	p := &AAAPolicy{}
	ctx := &PolicyContext{MACAddress: mustMAC(t, "aa:bb:cc:dd:ee:ff")}
	if got := p.ExpandPassword(ctx); got != "" {
		t.Fatalf("empty Password should expand to empty string, got %q", got)
	}
}

func TestExpandPasswordStatic(t *testing.T) {
	p := &AAAPolicy{Password: "cisco123"}
	ctx := &PolicyContext{MACAddress: mustMAC(t, "aa:bb:cc:dd:ee:ff")}
	if got := p.ExpandPassword(ctx); got != "cisco123" {
		t.Fatalf("static password: want %q, got %q", "cisco123", got)
	}
}

func TestExpandPasswordTokenRemoteID(t *testing.T) {
	p := &AAAPolicy{Password: "$remote-id$"}
	ctx := &PolicyContext{
		MACAddress: mustMAC(t, "aa:bb:cc:dd:ee:ff"),
		RemoteID:   "ONE6107874",
	}
	if got := p.ExpandPassword(ctx); got != "ONE6107874" {
		t.Fatalf("$remote-id$ expansion: want %q, got %q", "ONE6107874", got)
	}
}

func TestExpandPasswordAllTokens(t *testing.T) {
	p := &AAAPolicy{Password: "$mac-address$|$svlan$|$cvlan$|$remote-id$|$circuit-id$|$agent-circuit-id$|$agent-remote-id$|$agent-relay-id$|$hostname$"}
	ctx := &PolicyContext{
		MACAddress:     mustMAC(t, "aa:bb:cc:dd:ee:ff"),
		SVLAN:          100,
		CVLAN:          200,
		RemoteID:       "rid",
		CircuitID:      "cid",
		AgentCircuitID: "acid",
		AgentRemoteID:  "arid",
		AgentRelayID:   "arelid",
		Hostname:       "host",
	}
	want := "aa:bb:cc:dd:ee:ff|100|200|arid|acid|acid|arid|arelid|host"
	if got := p.ExpandPassword(ctx); got != want {
		t.Fatalf("all-tokens password: want %q, got %q", want, got)
	}
}

func TestExpandPasswordAgentPrecedenceMatchesFormat(t *testing.T) {
	p := &AAAPolicy{Format: "$remote-id$", Password: "$remote-id$"}
	ctx := &PolicyContext{
		MACAddress:    mustMAC(t, "aa:bb:cc:dd:ee:ff"),
		RemoteID:      "raw",
		AgentRemoteID: "agent",
	}
	gotF := p.ExpandFormat(ctx)
	gotP := p.ExpandPassword(ctx)
	if gotF != gotP {
		t.Fatalf("agent-remote-id precedence should match between ExpandFormat (%q) and ExpandPassword (%q)", gotF, gotP)
	}
	if gotP != "agent" {
		t.Fatalf("agent precedence: want %q, got %q", "agent", gotP)
	}
}

func TestExpandFormatStillWorks(t *testing.T) {
	p := &AAAPolicy{Format: "$mac-address$@$svlan$"}
	ctx := &PolicyContext{
		MACAddress: mustMAC(t, "aa:bb:cc:dd:ee:ff"),
		SVLAN:      100,
	}
	want := "aa:bb:cc:dd:ee:ff@100"
	if got := p.ExpandFormat(ctx); got != want {
		t.Fatalf("ExpandFormat regression: want %q, got %q", want, got)
	}
}

func TestExpandFormatChecked(t *testing.T) {
	mac := mustMAC(t, "aa:bb:cc:dd:ee:ff")

	t.Run("unset_format_reports_not_ok", func(t *testing.T) {
		p := &AAAPolicy{}
		got, ok := p.ExpandFormatChecked(&PolicyContext{MACAddress: mac})
		if ok {
			t.Fatalf("unset Format must report ok=false")
		}
		if got != "" {
			t.Fatalf("unset Format must expand to %q, got %q", "", got)
		}
	})

	t.Run("remote_id_token_with_no_remote_id_reports_not_ok", func(t *testing.T) {
		p := &AAAPolicy{Format: "$remote-id$"}
		got, ok := p.ExpandFormatChecked(&PolicyContext{MACAddress: mac})
		if ok {
			t.Fatalf("empty expansion must report ok=false")
		}
		if got != "" {
			t.Fatalf("empty expansion must yield %q, got %q", "", got)
		}
	})

	t.Run("remote_id_token_with_agent_remote_id_reports_ok", func(t *testing.T) {
		p := &AAAPolicy{Format: "$remote-id$"}
		got, ok := p.ExpandFormatChecked(&PolicyContext{MACAddress: mac, AgentRemoteID: "ONE7829589"})
		if !ok {
			t.Fatalf("non-empty expansion must report ok=true")
		}
		if got != "ONE7829589" {
			t.Fatalf("want %q, got %q", "ONE7829589", got)
		}
	})

	t.Run("always_resolvable_token_reports_ok", func(t *testing.T) {
		p := &AAAPolicy{Format: "$mac-address$"}
		got, ok := p.ExpandFormatChecked(&PolicyContext{MACAddress: mac})
		if !ok {
			t.Fatalf("$mac-address$ must always report ok=true")
		}
		if got != "aa:bb:cc:dd:ee:ff" {
			t.Fatalf("want %q, got %q", "aa:bb:cc:dd:ee:ff", got)
		}
	})
}
