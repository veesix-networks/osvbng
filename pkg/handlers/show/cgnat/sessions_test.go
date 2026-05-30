// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"reflect"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/handlers/pagination"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/models"
)

func TestParseSessionFilter(t *testing.T) {
	req := &show.Request{Options: map[string]string{
		"inside-ip":   "100.64.0.2",
		"remote-port": "443",
		"proto":       "tcp",
		"pool-id":     "42",
		"cursor":      "9",
		"limit":       "100",
	}}
	f, err := parseSessionFilter(req)
	if err != nil {
		t.Fatalf("parseSessionFilter: %v", err)
	}
	if f.InsideIP.String() != "100.64.0.2" {
		t.Errorf("InsideIP = %v", f.InsideIP)
	}
	if f.RemotePort != 443 || f.Proto != 6 || f.PoolID != 42 || f.Cursor != 9 || f.Limit != 100 {
		t.Errorf("parsed = %+v", f)
	}
}

func TestParseSessionFilterICMPRejectsRemotePort(t *testing.T) {
	req := &show.Request{Options: map[string]string{"proto": "icmp", "remote-port": "8"}}
	if _, err := parseSessionFilter(req); err == nil {
		t.Fatal("expected error for proto=icmp with remote-port, got nil")
	}
}

func TestParseSessionFilterICMPAllowsInsidePort(t *testing.T) {
	req := &show.Request{Options: map[string]string{"proto": "icmp", "inside-port": "1234"}}
	f, err := parseSessionFilter(req)
	if err != nil {
		t.Fatalf("inside-port with icmp should be allowed: %v", err)
	}
	if f.Proto != 1 || f.InsidePort != 1234 {
		t.Errorf("parsed = %+v", f)
	}
}

func TestParseSessionFilterRejectsBadInput(t *testing.T) {
	cases := []map[string]string{
		{"inside-ip": "not-an-ip"},
		{"inside-ip": "2001:db8::1"}, // IPv6 not valid for NAT44
		{"proto": "sctp"},
		{"limit": "abc"},
	}
	for _, opts := range cases {
		if _, err := parseSessionFilter(&show.Request{Options: opts}); err == nil {
			t.Errorf("expected error for %v", opts)
		}
	}
}

// The session page is a struct, not a slice, so the generic presentation-layer
// pagination must pass it through untouched (no double-pagination).
func TestSessionPageBypassesPagination(t *testing.T) {
	page := models.CGNATSessionPage{Sessions: []models.CGNATSession{{PoolID: 1}}, Total: 9}
	out, err := pagination.Paginate(page, pagination.Request{Limit: 100}, "")
	if err != nil {
		t.Fatalf("Paginate: %v", err)
	}
	if out.Paginated {
		t.Error("CGNATSessionPage was paginated; expected pass-through")
	}
}

func TestSessionsHandlerOutputIsStruct(t *testing.T) {
	h := &SessionsHandler{}
	if k := reflect.TypeOf(h.OutputType()).Kind(); k != reflect.Struct {
		t.Errorf("OutputType kind = %v, want struct (so the handler is not paginated)", k)
	}
	if _, ok := interface{}(h).(show.ShowSortHandler); ok {
		t.Error("SessionsHandler must not implement ShowSortHandler (would trigger generic pagination)")
	}
}
