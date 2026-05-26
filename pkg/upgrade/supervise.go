// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Commander runs an external command and returns its combined stdout+stderr.
// Production uses execCommander (os/exec); tests inject a fake.
type Commander interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// execCommander is the default Commander. Captures combined output so
// systemctl error messages survive into operator-visible error text.
type execCommander struct{}

func (execCommander) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.Bytes(), err
}

// NewExecCommander returns a Commander backed by os/exec. Use this in
// production; tests inject their own fake.
func NewExecCommander() Commander { return execCommander{} }

// Supervisor wraps systemd unit operations the upgrade flow needs.
// Targets a single unit (typically `osvbng.service`). Uses the drop-in
// approach for transient Restart= overrides rather than `systemctl
// set-property --runtime`, because Restart= is not a runtime-mutable
// property on systemd 252 and earlier.
type Supervisor struct {
	Unit       string    // unit name, e.g. "osvbng.service"
	DropInRoot string    // "/run/systemd/system" parent (overridable for tests)
	Cmd        Commander // injectable command runner
}

// NewSupervisor returns a Supervisor with production defaults.
func NewSupervisor(unit string) *Supervisor {
	return &Supervisor{
		Unit:       unit,
		DropInRoot: "/run/systemd/system",
		Cmd:        execCommander{},
	}
}

// UnitState is the parsed result of `systemctl show <unit>
// --property=ActiveState,SubState,Result`. Fields preserve the raw
// systemd vocabulary so callers can match the documented state machine
// directly (active / activating / inactive / deactivating / failed /
// reloading / maintenance).
type UnitState struct {
	ActiveState string
	SubState    string
	Result      string
}

// IsFailed returns true when systemd reports the unit as failed. The
// upgrade health-poll fast-paths on this to abort early without waiting
// the full overall timeout.
func (s UnitState) IsFailed() bool { return s.ActiveState == "failed" }

// IsActive returns true when ActiveState == "active". Distinct from the
// "is the daemon ready" check which also requires the state file to
// report ready and /readyz to return 200.
func (s UnitState) IsActive() bool { return s.ActiveState == "active" }

// IsActivating returns true when systemd reports the unit as
// activating. The health-poll uses this to know it's making forward
// progress and should keep polling.
func (s UnitState) IsActivating() bool { return s.ActiveState == "activating" }

// Show queries systemd for the unit's current state via `systemctl show
// --property=ActiveState,SubState,Result`.
func (s *Supervisor) Show(ctx context.Context) (UnitState, error) {
	out, err := s.Cmd.Run(ctx, "systemctl", "show", s.Unit,
		"--property=ActiveState",
		"--property=SubState",
		"--property=Result")
	if err != nil {
		return UnitState{}, fmt.Errorf("systemctl show %s: %w (%s)", s.Unit, err, strings.TrimSpace(string(out)))
	}
	return parseSystemctlShow(out), nil
}

// Stop runs `systemctl stop <unit>` synchronously. systemd returns
// when the unit has reached an inactive or failed state.
func (s *Supervisor) Stop(ctx context.Context) error {
	out, err := s.Cmd.Run(ctx, "systemctl", "stop", s.Unit)
	if err != nil {
		return fmt.Errorf("systemctl stop %s: %w (%s)", s.Unit, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Start runs `systemctl start <unit>`. systemd returns once the unit
// has been told to start; the unit may still be `activating` for some
// time afterwards.
func (s *Supervisor) Start(ctx context.Context) error {
	out, err := s.Cmd.Run(ctx, "systemctl", "start", s.Unit)
	if err != nil {
		return fmt.Errorf("systemctl start %s: %w (%s)", s.Unit, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// SuspendAutoRestart writes a transient drop-in disabling Restart= on
// the unit, then reloads systemd. The drop-in lives under /run so it
// clears automatically on reboot if the upgrade flow crashes before
// RestoreAutoRestart runs.
//
// Restoring the previous Restart= policy is the caller's responsibility
// via RestoreAutoRestart, normally registered as a defer at apply
// entry so it runs on success, failure, and signal-driven abort.
func (s *Supervisor) SuspendAutoRestart(ctx context.Context) error {
	dir := s.dropInDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	conf := []byte("[Service]\nRestart=no\n")
	if err := os.WriteFile(filepath.Join(dir, "upgrade.conf"), conf, 0o644); err != nil {
		return fmt.Errorf("write drop-in: %w", err)
	}
	return s.daemonReload(ctx)
}

// RestoreAutoRestart removes the transient drop-in and reloads systemd.
// Idempotent: a missing drop-in is not an error so this is safe to call
// even when SuspendAutoRestart was not called or already cleaned up.
func (s *Supervisor) RestoreAutoRestart(ctx context.Context) error {
	dir := s.dropInDir()
	confPath := filepath.Join(dir, "upgrade.conf")
	if err := os.Remove(confPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove drop-in: %w", err)
	}
	// Drop the dir too if it's empty so we don't leave litter under /run.
	_ = os.Remove(dir)
	return s.daemonReload(ctx)
}

func (s *Supervisor) daemonReload(ctx context.Context) error {
	out, err := s.Cmd.Run(ctx, "systemctl", "daemon-reload")
	if err != nil {
		return fmt.Errorf("systemctl daemon-reload: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *Supervisor) dropInDir() string {
	return filepath.Join(s.DropInRoot, s.Unit+".d")
}

// parseSystemctlShow turns key=value lines (one per line) into a
// UnitState. Tolerates extra properties — systemd-version drift can
// add fields between releases.
func parseSystemctlShow(out []byte) UnitState {
	var st UnitState
	for _, line := range strings.Split(string(out), "\n") {
		i := strings.IndexByte(line, '=')
		if i <= 0 {
			continue
		}
		switch strings.TrimSpace(line[:i]) {
		case "ActiveState":
			st.ActiveState = strings.TrimSpace(line[i+1:])
		case "SubState":
			st.SubState = strings.TrimSpace(line[i+1:])
		case "Result":
			st.Result = strings.TrimSpace(line[i+1:])
		}
	}
	return st
}
