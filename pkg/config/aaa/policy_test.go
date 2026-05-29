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
