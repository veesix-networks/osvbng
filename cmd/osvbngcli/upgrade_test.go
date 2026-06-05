// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/upgrade"
)

// fakeUpgradeRunner records which method was invoked with which
// arguments so the dispatch tests can assert routing correctness
// without depending on the real Runner's filesystem side effects.
type fakeUpgradeRunner struct {
	planCalls     atomic.Int32
	applyCalls    atomic.Int32
	rollbackCalls atomic.Int32
	statusCalls   atomic.Int32

	lastPlanArg   string
	lastApplyArg  string
	lastApplyOpts upgrade.ApplyOptions

	planErr     error
	applyErr    error
	rollbackErr error
	statusErr   error
}

func (f *fakeUpgradeRunner) Plan(_ context.Context, tarballPath string) (*upgrade.PlanResult, error) {
	f.planCalls.Add(1)
	f.lastPlanArg = tarballPath
	if f.planErr != nil {
		return nil, f.planErr
	}
	return &upgrade.PlanResult{From: "0.13.0", To: "0.13.1", Tier: "A"}, nil
}

func (f *fakeUpgradeRunner) Apply(_ context.Context, tarballPath string) (*upgrade.ApplyResult, error) {
	f.applyCalls.Add(1)
	f.lastApplyArg = tarballPath
	f.lastApplyOpts = upgrade.ApplyOptions{}
	if f.applyErr != nil {
		return nil, f.applyErr
	}
	return &upgrade.ApplyResult{From: "0.13.0", To: "0.13.1", JournalEndPhase: "completed"}, nil
}

func (f *fakeUpgradeRunner) ApplyOne(_ context.Context, tarballPath string, opts upgrade.ApplyOptions) (*upgrade.ApplyResult, error) {
	f.applyCalls.Add(1)
	f.lastApplyArg = tarballPath
	f.lastApplyOpts = opts
	if f.applyErr != nil {
		return nil, f.applyErr
	}
	return &upgrade.ApplyResult{From: "0.13.0", To: "0.13.1", JournalEndPhase: "completed"}, nil
}

func (f *fakeUpgradeRunner) Rollback(_ context.Context) (*upgrade.RollbackResult, error) {
	f.rollbackCalls.Add(1)
	if f.rollbackErr != nil {
		return nil, f.rollbackErr
	}
	return &upgrade.RollbackResult{From: "0.13.1", To: "0.13.0", HealthOutcome: "ok"}, nil
}

func (f *fakeUpgradeRunner) Status(_ context.Context) (*upgrade.StatusResult, error) {
	f.statusCalls.Add(1)
	if f.statusErr != nil {
		return nil, f.statusErr
	}
	return &upgrade.StatusResult{CurrentVersion: "0.13.1"}, nil
}

// withSuppressedStdout swallows stdout for the duration of fn so the
// render helpers (which fmt.Println to stdout) don't pollute the test
// output stream.
func withSuppressedStdout(t *testing.T, fn func()) {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, r)
		close(done)
	}()
	defer func() {
		w.Close()
		<-done
		os.Stdout = orig
	}()
	fn()
}

func TestRunUpgradeActionRoutesPlan(t *testing.T) {
	fake := &fakeUpgradeRunner{}
	withSuppressedStdout(t, func() {
		if err := runUpgradeAction(context.Background(), fake, "plan", []string{"/tmp/foo.tar.gz"}); err != nil {
			t.Fatalf("plan: %v", err)
		}
	})
	if fake.planCalls.Load() != 1 {
		t.Fatalf("Plan calls = %d, want 1", fake.planCalls.Load())
	}
	if fake.lastPlanArg != "/tmp/foo.tar.gz" {
		t.Fatalf("lastPlanArg = %q, want /tmp/foo.tar.gz", fake.lastPlanArg)
	}
}

func TestRunUpgradeActionRoutesApply(t *testing.T) {
	fake := &fakeUpgradeRunner{}
	withSuppressedStdout(t, func() {
		if err := runUpgradeAction(context.Background(), fake, "apply", []string{"/tmp/foo.tar.gz"}); err != nil {
			t.Fatalf("apply: %v", err)
		}
	})
	if fake.applyCalls.Load() != 1 {
		t.Fatalf("Apply calls = %d, want 1", fake.applyCalls.Load())
	}
}

func TestRunUpgradeActionRoutesRollback(t *testing.T) {
	fake := &fakeUpgradeRunner{}
	withSuppressedStdout(t, func() {
		if err := runUpgradeAction(context.Background(), fake, "rollback", nil); err != nil {
			t.Fatalf("rollback: %v", err)
		}
	})
	if fake.rollbackCalls.Load() != 1 {
		t.Fatalf("Rollback calls = %d, want 1", fake.rollbackCalls.Load())
	}
}

func TestRunUpgradeActionRoutesStatus(t *testing.T) {
	fake := &fakeUpgradeRunner{}
	withSuppressedStdout(t, func() {
		if err := runUpgradeAction(context.Background(), fake, "status", nil); err != nil {
			t.Fatalf("status: %v", err)
		}
	})
	if fake.statusCalls.Load() != 1 {
		t.Fatalf("Status calls = %d, want 1", fake.statusCalls.Load())
	}
}

func TestRunUpgradeActionRejectsUnknownSubAction(t *testing.T) {
	fake := &fakeUpgradeRunner{}
	err := runUpgradeAction(context.Background(), fake, "wibble", nil)
	if err == nil {
		t.Fatal("runUpgradeAction accepted unknown sub-action")
	}
	if !strings.Contains(err.Error(), "wibble") {
		t.Fatalf("error did not name the bad sub-action: %v", err)
	}
}

func TestRunUpgradeActionRejectsBadArgCounts(t *testing.T) {
	fake := &fakeUpgradeRunner{}

	if err := runUpgradeAction(context.Background(), fake, "plan", nil); err == nil {
		t.Fatal("plan with no args was accepted")
	}
	if err := runUpgradeAction(context.Background(), fake, "apply", []string{"a", "b"}); err == nil {
		t.Fatal("apply with extra args was accepted")
	}
	if err := runUpgradeAction(context.Background(), fake, "rollback", []string{"unexpected"}); err == nil {
		t.Fatal("rollback with extra args was accepted")
	}
	if err := runUpgradeAction(context.Background(), fake, "status", []string{"unexpected"}); err == nil {
		t.Fatal("status with extra args was accepted")
	}
}

func TestRunUpgradeActionPropagatesRunnerErrors(t *testing.T) {
	fake := &fakeUpgradeRunner{applyErr: errors.New("signature verification failed")}
	err := runUpgradeAction(context.Background(), fake, "apply", []string{"/tmp/bad.tar.gz"})
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if !strings.Contains(err.Error(), "signature") {
		t.Fatalf("error did not preserve runner detail: %v", err)
	}
}

func TestInterruptActiveUpgradeWithoutActiveReturnsFalse(t *testing.T) {
	// Sanity: no upgrade is in flight in this test process.
	if interruptActiveUpgrade() {
		t.Fatal("interruptActiveUpgrade returned true with no active upgrade")
	}
}

func TestSetUpgradeCancelInstallsAndClears(t *testing.T) {
	var called atomic.Bool
	cleanup := setUpgradeCancel(func() { called.Store(true) })

	if !interruptActiveUpgrade() {
		t.Fatal("interruptActiveUpgrade returned false while cancel was registered")
	}
	if !called.Load() {
		t.Fatal("registered cancel was not invoked")
	}

	cleanup()
	if interruptActiveUpgrade() {
		t.Fatal("interruptActiveUpgrade returned true after cleanup")
	}
}

func TestParseApplyArgsTarballOnly(t *testing.T) {
	path, opts, err := parseApplyArgs([]string{"/tmp/x.tar.gz"})
	if err != nil {
		t.Fatalf("parseApplyArgs: %v", err)
	}
	if path != "/tmp/x.tar.gz" {
		t.Fatalf("path = %q, want /tmp/x.tar.gz", path)
	}
	if opts.FirstBoot || opts.ForceRetry {
		t.Fatalf("opts = %+v, want both flags false", opts)
	}
}

func TestParseApplyArgsFirstBoot(t *testing.T) {
	path, opts, err := parseApplyArgs([]string{"--first-boot", "/tmp/x.tar.gz"})
	if err != nil {
		t.Fatalf("parseApplyArgs: %v", err)
	}
	if path != "/tmp/x.tar.gz" {
		t.Fatalf("path = %q, want /tmp/x.tar.gz", path)
	}
	if !opts.FirstBoot {
		t.Fatal("FirstBoot not set")
	}
	if opts.ForceRetry {
		t.Fatal("ForceRetry should not be set")
	}
}

func TestParseApplyArgsForceRetry(t *testing.T) {
	_, opts, err := parseApplyArgs([]string{"/tmp/x.tar.gz", "--force-retry"})
	if err != nil {
		t.Fatalf("parseApplyArgs: %v", err)
	}
	if !opts.ForceRetry {
		t.Fatal("ForceRetry not set")
	}
}

func TestParseApplyArgsBothFlags(t *testing.T) {
	_, opts, err := parseApplyArgs([]string{"--first-boot", "--force-retry", "/tmp/x.tar.gz"})
	if err != nil {
		t.Fatalf("parseApplyArgs: %v", err)
	}
	if !opts.FirstBoot || !opts.ForceRetry {
		t.Fatalf("opts = %+v, want both true", opts)
	}
}

func TestParseApplyArgsRejectsUnknownFlag(t *testing.T) {
	if _, _, err := parseApplyArgs([]string{"--bogus", "/tmp/x.tar.gz"}); err == nil {
		t.Fatal("parseApplyArgs accepted unknown flag")
	}
}

func TestParseApplyArgsRejectsZeroPositional(t *testing.T) {
	if _, _, err := parseApplyArgs([]string{"--first-boot"}); err == nil {
		t.Fatal("parseApplyArgs accepted zero positional args")
	}
}

func TestParseApplyArgsRejectsMultiplePositional(t *testing.T) {
	if _, _, err := parseApplyArgs([]string{"/tmp/a.tar.gz", "/tmp/b.tar.gz"}); err == nil {
		t.Fatal("parseApplyArgs accepted two positional args")
	}
}

func TestRunUpgradeActionApplyForwardsFirstBoot(t *testing.T) {
	fake := &fakeUpgradeRunner{}
	withSuppressedStdout(t, func() {
		err := runUpgradeAction(context.Background(), fake, "apply",
			[]string{"--first-boot", "/tmp/x.tar.gz"})
		if err != nil {
			t.Fatalf("runUpgradeAction: %v", err)
		}
	})
	if fake.applyCalls.Load() != 1 {
		t.Fatalf("ApplyOne call count = %d, want 1", fake.applyCalls.Load())
	}
	if !fake.lastApplyOpts.FirstBoot {
		t.Fatal("ApplyOne was not invoked with FirstBoot=true")
	}
}
