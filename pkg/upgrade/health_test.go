// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// virtualClock is an injectable time source for deterministic health
// poll tests. Now() returns the current value; Sleep() advances it.
type virtualClock struct {
	now atomic.Int64
}

func newClock(start time.Time) *virtualClock {
	c := &virtualClock{}
	c.now.Store(start.UnixNano())
	return c
}

func (c *virtualClock) Now() time.Time {
	return time.Unix(0, c.now.Load()).UTC()
}

func (c *virtualClock) Advance(d time.Duration) {
	c.now.Add(int64(d))
}

func writeStateFile(t *testing.T, path, state string, sequence uint64) {
	t.Helper()
	payload, err := json.Marshal(struct {
		State     string    `json:"state"`
		Sequence  uint64    `json:"sequence"`
		UpdatedAt time.Time `json:"updated_at"`
	}{State: state, Sequence: sequence, UpdatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatalf("write state file: %v", err)
	}
}

func TestWaitHealthyHappyPath(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state")
	writeStateFile(t, statePath, "ready", 5)

	fake := &fakeCommander{scripts: []fakeResp{
		{matchName: "systemctl", out: "ActiveState=active\nSubState=running\nResult=success\n"},
	}}
	sv := &Supervisor{Unit: "osvbng.service", DropInRoot: t.TempDir(), Cmd: fake}

	hc := &HealthChecker{
		Supervisor:    sv,
		StateFilePath: statePath,
	}
	res, msg, err := hc.WaitHealthy(context.Background())
	if err != nil {
		t.Fatalf("WaitHealthy: %v", err)
	}
	if res != HealthOK {
		t.Fatalf("res = %v (%s), want HealthOK", res, msg)
	}
}

func TestWaitHealthyFailedShortCircuits(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state")

	fake := &fakeCommander{scripts: []fakeResp{
		{matchName: "systemctl", out: "ActiveState=failed\nSubState=failed\nResult=exit-code\n"},
	}}
	sv := &Supervisor{Unit: "osvbng.service", DropInRoot: t.TempDir(), Cmd: fake}

	hc := &HealthChecker{
		Supervisor:     sv,
		StateFilePath:  statePath,
		OverallTimeout: 30 * time.Second,
	}
	res, msg, _ := hc.WaitHealthy(context.Background())
	if res != HealthFailed {
		t.Fatalf("res = %v (%s), want HealthFailed", res, msg)
	}
}

func TestWaitHealthyDegradedStateReturnsHealthDegraded(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state")
	writeStateFile(t, statePath, "degraded", 7)

	fake := &fakeCommander{scripts: []fakeResp{
		{matchName: "systemctl", out: "ActiveState=active\nSubState=running\nResult=success\n"},
	}}
	sv := &Supervisor{Unit: "osvbng.service", DropInRoot: t.TempDir(), Cmd: fake}

	hc := &HealthChecker{Supervisor: sv, StateFilePath: statePath}
	res, msg, _ := hc.WaitHealthy(context.Background())
	if res != HealthDegraded {
		t.Fatalf("res = %v (%s), want HealthDegraded", res, msg)
	}
}

func TestWaitHealthyInvalidStateReturnsHealthInvalidState(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state")
	writeStateFile(t, statePath, "wat", 1)

	fake := &fakeCommander{scripts: []fakeResp{
		{matchName: "systemctl", out: "ActiveState=active\nSubState=running\nResult=success\n"},
	}}
	sv := &Supervisor{Unit: "osvbng.service", DropInRoot: t.TempDir(), Cmd: fake}

	hc := &HealthChecker{Supervisor: sv, StateFilePath: statePath}
	res, _, _ := hc.WaitHealthy(context.Background())
	if res != HealthInvalidState {
		t.Fatalf("res = %v, want HealthInvalidState", res)
	}
}

func TestWaitHealthyActiveWithoutStateFileWaitsForGrace(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state")

	fake := &fakeCommander{scripts: []fakeResp{
		{matchName: "systemctl", out: "ActiveState=active\nSubState=running\nResult=success\n"},
	}}
	sv := &Supervisor{Unit: "osvbng.service", DropInRoot: t.TempDir(), Cmd: fake}

	clk := newClock(time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC))
	hc := &HealthChecker{
		Supervisor:     sv,
		StateFilePath:  statePath,
		OverallTimeout: 30 * time.Second,
		StateFileGrace: 5 * time.Second,
		PollInterval:   1 * time.Second,
		Now:            clk.Now,
		Sleep: func(d time.Duration) {
			clk.Advance(d)
			// Spawn the state file at 3s into the grace window so the
			// next loop iteration finds it and reports ready.
			if clk.Now().Sub(time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)) >= 3*time.Second {
				if _, err := os.Stat(statePath); os.IsNotExist(err) {
					writeStateFile(t, statePath, "ready", 1)
				}
			}
		},
	}

	res, msg, _ := hc.WaitHealthy(context.Background())
	if res != HealthOK {
		t.Fatalf("res = %v (%s), want HealthOK after grace", res, msg)
	}
}

func TestWaitHealthyActiveWithoutStateFilePastGraceTimesOut(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state")

	fake := &fakeCommander{scripts: []fakeResp{
		{matchName: "systemctl", out: "ActiveState=active\nSubState=running\nResult=success\n"},
	}}
	sv := &Supervisor{Unit: "osvbng.service", DropInRoot: t.TempDir(), Cmd: fake}

	clk := newClock(time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC))
	hc := &HealthChecker{
		Supervisor:     sv,
		StateFilePath:  statePath,
		OverallTimeout: 30 * time.Second,
		StateFileGrace: 5 * time.Second,
		PollInterval:   1 * time.Second,
		Now:            clk.Now,
		Sleep:          func(d time.Duration) { clk.Advance(d) },
	}

	res, msg, _ := hc.WaitHealthy(context.Background())
	if res != HealthTimeout {
		t.Fatalf("res = %v (%s), want HealthTimeout (state file never appeared)", res, msg)
	}
}

func TestWaitHealthyStallDetectionDuringActivating(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state")
	writeStateFile(t, statePath, "restoring", 2)

	fake := &fakeCommander{scripts: []fakeResp{
		{matchName: "systemctl", out: "ActiveState=activating\nSubState=start\nResult=success\n"},
	}}
	sv := &Supervisor{Unit: "osvbng.service", DropInRoot: t.TempDir(), Cmd: fake}

	clk := newClock(time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC))
	hc := &HealthChecker{
		Supervisor:     sv,
		StateFilePath:  statePath,
		OverallTimeout: 5 * time.Minute,
		StallLimit:     10 * time.Second,
		PollInterval:   1 * time.Second,
		Now:            clk.Now,
		Sleep:          func(d time.Duration) { clk.Advance(d) },
	}

	res, msg, _ := hc.WaitHealthy(context.Background())
	if res != HealthStalled {
		t.Fatalf("res = %v (%s), want HealthStalled", res, msg)
	}
}

func TestWaitHealthyProgressResetsStall(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state")
	writeStateFile(t, statePath, "restoring", 1)

	fake := &fakeCommander{scripts: []fakeResp{
		{matchName: "systemctl", out: "ActiveState=activating\nSubState=start\nResult=success\n"},
	}}
	sv := &Supervisor{Unit: "osvbng.service", DropInRoot: t.TempDir(), Cmd: fake}

	clk := newClock(time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC))
	seq := uint64(1)
	hc := &HealthChecker{
		Supervisor:     sv,
		StateFilePath:  statePath,
		OverallTimeout: 60 * time.Second,
		StallLimit:     10 * time.Second,
		PollInterval:   1 * time.Second,
		Now:            clk.Now,
		Sleep: func(d time.Duration) {
			clk.Advance(d)
			// Advance the sequence every poll cycle so the stall
			// detector keeps resetting. After 30 seconds switch to
			// ready so the loop terminates.
			elapsed := clk.Now().Sub(time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC))
			seq++
			state := "restoring"
			active := "ActiveState=activating\nSubState=start\nResult=success\n"
			if elapsed >= 30*time.Second {
				state = "ready"
				active = "ActiveState=active\nSubState=running\nResult=success\n"
			}
			writeStateFile(t, statePath, state, seq)
			fake.mu.Lock()
			fake.scripts = []fakeResp{{matchName: "systemctl", out: active}}
			fake.mu.Unlock()
		},
	}

	res, msg, _ := hc.WaitHealthy(context.Background())
	if res != HealthOK {
		t.Fatalf("res = %v (%s), want HealthOK after progressing through restoring -> ready", res, msg)
	}
}

func TestHealthResultStringHasNoUnknownForKnownValues(t *testing.T) {
	for _, res := range []HealthResult{HealthOK, HealthFailed, HealthStalled, HealthTimeout, HealthDegraded, HealthInvalidState} {
		s := res.String()
		if s == "" || s == "unknown" {
			t.Fatalf("HealthResult(%d).String() = %q", res, s)
		}
	}
}

