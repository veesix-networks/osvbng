// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package system

import (
	"context"
	"errors"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper/paths"
)

type stubConfigReloader struct {
	calls int
	err   error
}

func (s *stubConfigReloader) ReloadFRR() error {
	s.calls++
	return s.err
}

func TestSystemReloadHappyPath(t *testing.T) {
	stub := &stubConfigReloader{}
	h := &SystemReloadHandler{deps: &deps.OperDeps{ConfigReloader: stub}}

	got, err := h.Execute(context.Background(), &oper.Request{Path: paths.SystemReload.String()})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if stub.calls != 1 {
		t.Fatalf("ReloadFRR call count = %d, want 1", stub.calls)
	}
	resp, ok := got.(*SystemReloadResponse)
	if !ok {
		t.Fatalf("response type = %T, want *SystemReloadResponse", got)
	}
	if len(resp.Errors) != 0 {
		t.Fatalf("Errors = %v, want empty on happy path", resp.Errors)
	}
	if !containsAll(resp.ComponentsTouched, "frr", "dataplane:not-handled") {
		t.Fatalf("ComponentsTouched = %v, want frr + dataplane:not-handled", resp.ComponentsTouched)
	}
}

func TestSystemReloadSurfacesFRRError(t *testing.T) {
	stub := &stubConfigReloader{err: errors.New("frr-reload exited 1")}
	h := &SystemReloadHandler{deps: &deps.OperDeps{ConfigReloader: stub}}

	got, err := h.Execute(context.Background(), &oper.Request{Path: paths.SystemReload.String()})
	if err == nil {
		t.Fatal("Execute returned nil error despite FRR reload failure")
	}
	resp, ok := got.(*SystemReloadResponse)
	if !ok {
		t.Fatalf("response type = %T, want *SystemReloadResponse (even on partial failure)", got)
	}
	if len(resp.Errors) != 1 {
		t.Fatalf("Errors = %v, want 1 entry naming the FRR failure", resp.Errors)
	}
	if containsAny(resp.ComponentsTouched, "frr") {
		t.Fatalf("ComponentsTouched = %v, must not include frr after a failed reload", resp.ComponentsTouched)
	}
	if !containsAny(resp.ComponentsTouched, "dataplane:not-handled") {
		t.Fatalf("ComponentsTouched = %v, must still include dataplane:not-handled", resp.ComponentsTouched)
	}
}

func TestSystemReloadRefusesWhenConfigReloaderUnwired(t *testing.T) {
	h := &SystemReloadHandler{deps: &deps.OperDeps{}}

	_, err := h.Execute(context.Background(), &oper.Request{Path: paths.SystemReload.String()})
	if err == nil {
		t.Fatal("Execute accepted a nil ConfigReloader")
	}
}

func TestSystemReloadPathPattern(t *testing.T) {
	h := &SystemReloadHandler{}
	if got := h.PathPattern(); got != paths.SystemReload {
		t.Fatalf("PathPattern() = %q, want %q", got, paths.SystemReload)
	}
}

func containsAll(haystack []string, needles ...string) bool {
	for _, n := range needles {
		if !containsAny(haystack, n) {
			return false
		}
	}
	return true
}

func containsAny(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
