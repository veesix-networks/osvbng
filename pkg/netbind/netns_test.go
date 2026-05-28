// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package netbind

import (
	"errors"
	"testing"
)

func TestWithNetNS_ZeroBindingNoOp(t *testing.T) {
	SetLCPNetNs("")

	called := false
	err := withNetNS(Binding{}, func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("withNetNS: %v", err)
	}
	if !called {
		t.Fatal("fn not invoked")
	}
}

func TestWithNetNS_VRFButNoNetnsConfigured(t *testing.T) {
	SetLCPNetNs("")

	called := false
	err := withNetNS(Binding{VRF: "mgmt"}, func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("withNetNS: %v", err)
	}
	if !called {
		t.Fatal("fn not invoked")
	}
}

func TestWithNetNS_PropagatesFnError(t *testing.T) {
	SetLCPNetNs("")

	want := errors.New("dial failed")
	got := withNetNS(Binding{VRF: "mgmt"}, func() error {
		return want
	})
	if !errors.Is(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestWithNetNS_NonexistentNetns(t *testing.T) {
	SetLCPNetNs("netbind-test-does-not-exist")
	defer SetLCPNetNs("")

	err := withNetNS(Binding{VRF: "mgmt"}, func() error {
		t.Fatal("fn should not run when netns lookup fails")
		return nil
	})
	if err == nil {
		t.Fatal("expected error from missing netns")
	}
}
