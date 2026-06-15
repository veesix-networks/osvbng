// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeCommander captures invocations and returns scripted responses.
type fakeCommander struct {
	mu      sync.Mutex
	calls   []fakeCall
	scripts []fakeResp
}

type fakeCall struct {
	name string
	args []string
}

type fakeResp struct {
	matchName string
	matchArgs []string // empty means match any args
	out       string
	err       error
}

func (f *fakeCommander) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{name: name, args: append([]string(nil), args...)})

	for _, r := range f.scripts {
		if r.matchName != "" && r.matchName != name {
			continue
		}
		if len(r.matchArgs) > 0 && !sliceHasPrefix(args, r.matchArgs) {
			continue
		}
		return []byte(r.out), r.err
	}
	return nil, nil
}

func sliceHasPrefix(slice, prefix []string) bool {
	if len(slice) < len(prefix) {
		return false
	}
	for i := range prefix {
		if slice[i] != prefix[i] {
			return false
		}
	}
	return true
}

func newSupervisorWithFake(t *testing.T) (*Supervisor, *fakeCommander) {
	t.Helper()
	fake := &fakeCommander{}
	sv := &Supervisor{
		Unit:       "osvbng.service",
		DropInRoot: t.TempDir(),
		Cmd:        fake,
	}
	return sv, fake
}

func TestSupervisorShowParsesActiveState(t *testing.T) {
	sv, fake := newSupervisorWithFake(t)
	fake.scripts = []fakeResp{{matchName: "systemctl", out: "ActiveState=active\nSubState=running\nResult=success\n"}}

	st, err := sv.Show(context.Background())
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if !st.IsActive() {
		t.Fatalf("IsActive() = false, want true (ActiveState=%q)", st.ActiveState)
	}
	if st.SubState != "running" {
		t.Fatalf("SubState = %q, want running", st.SubState)
	}
	if st.Result != "success" {
		t.Fatalf("Result = %q, want success", st.Result)
	}
}

func TestSupervisorShowDetectsFailed(t *testing.T) {
	sv, fake := newSupervisorWithFake(t)
	fake.scripts = []fakeResp{{matchName: "systemctl", out: "ActiveState=failed\nSubState=failed\nResult=exit-code\n"}}

	st, err := sv.Show(context.Background())
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if !st.IsFailed() {
		t.Fatalf("IsFailed() = false on ActiveState=failed")
	}
	if st.IsActive() {
		t.Fatalf("IsActive() = true on failed unit")
	}
}

func TestSupervisorShowDetectsActivating(t *testing.T) {
	sv, fake := newSupervisorWithFake(t)
	fake.scripts = []fakeResp{{matchName: "systemctl", out: "ActiveState=activating\nSubState=start\nResult=success\n"}}

	st, err := sv.Show(context.Background())
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if !st.IsActivating() {
		t.Fatalf("IsActivating() = false (ActiveState=%q)", st.ActiveState)
	}
}

func TestSupervisorShowSurfacesCommandError(t *testing.T) {
	sv, fake := newSupervisorWithFake(t)
	fake.scripts = []fakeResp{{
		matchName: "systemctl",
		out:       "Unit not found.",
		err:       errors.New("exit status 1"),
	}}

	_, err := sv.Show(context.Background())
	if err == nil {
		t.Fatal("Show on bad unit: err = nil, want error")
	}
	if !strings.Contains(err.Error(), "Unit not found") {
		t.Fatalf("error did not include systemctl output: %v", err)
	}
}

func TestSupervisorSuspendAndRestoreAutoRestart(t *testing.T) {
	sv, fake := newSupervisorWithFake(t)
	ctx := context.Background()

	if err := sv.SuspendAutoRestart(ctx); err != nil {
		t.Fatalf("SuspendAutoRestart: %v", err)
	}

	confPath := filepath.Join(sv.DropInRoot, "osvbng.service.d", "upgrade.conf")
	body, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("read drop-in: %v", err)
	}
	if !strings.Contains(string(body), "Restart=no") {
		t.Fatalf("drop-in content = %q, want Restart=no", string(body))
	}
	if !strings.Contains(string(body), "RuntimeDirectoryPreserve=yes") {
		t.Fatalf("drop-in content = %q, want RuntimeDirectoryPreserve=yes", string(body))
	}

	// Verify daemon-reload was invoked.
	gotReload := false
	for _, c := range fake.calls {
		if c.name == "systemctl" && len(c.args) > 0 && c.args[0] == "daemon-reload" {
			gotReload = true
		}
	}
	if !gotReload {
		t.Fatalf("daemon-reload was not invoked during Suspend")
	}

	if err := sv.RestoreAutoRestart(ctx); err != nil {
		t.Fatalf("RestoreAutoRestart: %v", err)
	}
	if _, err := os.Stat(confPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("drop-in still present after Restore: %v", err)
	}
	// Drop-in dir should also be cleaned up since it was empty.
	if _, err := os.Stat(filepath.Dir(confPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("drop-in dir not cleaned: %v", err)
	}
}

func TestSupervisorRestoreIsIdempotent(t *testing.T) {
	sv, _ := newSupervisorWithFake(t)
	ctx := context.Background()
	// Never called Suspend. Restore must succeed regardless.
	if err := sv.RestoreAutoRestart(ctx); err != nil {
		t.Fatalf("RestoreAutoRestart on never-suspended: %v", err)
	}
}

func TestSupervisorStartStopInvokesSystemctl(t *testing.T) {
	sv, fake := newSupervisorWithFake(t)
	ctx := context.Background()

	if err := sv.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := sv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	var gotStop, gotStart bool
	for _, c := range fake.calls {
		if c.name == "systemctl" && len(c.args) >= 2 && c.args[0] == "stop" && c.args[1] == "osvbng.service" {
			gotStop = true
		}
		if c.name == "systemctl" && len(c.args) >= 2 && c.args[0] == "start" && c.args[1] == "osvbng.service" {
			gotStart = true
		}
	}
	if !gotStop {
		t.Fatalf("systemctl stop osvbng.service was not invoked")
	}
	if !gotStart {
		t.Fatalf("systemctl start osvbng.service was not invoked")
	}
}

func TestSupervisorParseSystemctlShowTolerantOfExtraFields(t *testing.T) {
	sv, fake := newSupervisorWithFake(t)
	fake.scripts = []fakeResp{{
		matchName: "systemctl",
		out: "ActiveState=active\nSubState=running\nResult=success\n" +
			"ExtraField=ignored\nAnotherField=also-ignored\n",
	}}

	st, err := sv.Show(context.Background())
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if !st.IsActive() {
		t.Fatalf("extra fields broke parsing: %#v", st)
	}
}
