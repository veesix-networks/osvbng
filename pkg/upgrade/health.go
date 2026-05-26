// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"
)

// HealthResult is the outcome of WaitHealthy.
type HealthResult int

const (
	// HealthOK means systemd reports active and the state file says
	// ready. Apply may declare success.
	HealthOK HealthResult = iota

	// HealthFailed means the unit transitioned to "failed" — the daemon
	// crashed and systemd is not retrying (the upgrade flow has
	// transiently set Restart=no). Caller should trigger rollback.
	HealthFailed

	// HealthStalled means we have not seen state-file progress within
	// StallLimit. The daemon is making no forward progress; caller
	// should trigger rollback.
	HealthStalled

	// HealthTimeout means OverallTimeout elapsed without reaching
	// ready. Caller should trigger rollback.
	HealthTimeout

	// HealthDegraded means the state file says "degraded". The daemon
	// is up but reports component failure.
	HealthDegraded

	// HealthInvalidState means the state file holds a value this build
	// does not recognise. Caller should trigger rollback.
	HealthInvalidState
)

// String renders a HealthResult for logging.
func (h HealthResult) String() string {
	switch h {
	case HealthOK:
		return "ok"
	case HealthFailed:
		return "failed"
	case HealthStalled:
		return "stalled"
	case HealthTimeout:
		return "timeout"
	case HealthDegraded:
		return "degraded"
	case HealthInvalidState:
		return "invalid_state"
	default:
		return "unknown"
	}
}

// HealthChecker queries two signals the upgrade health-poll
// disambiguates between: systemd unit ActiveState, and the osvbngd
// state file at /run/osvbng/state. Both are injectable so tests don't
// need a live daemon or real systemd.
//
// /readyz is intentionally NOT consulted. The northbound API plugin is
// operator-configurable; an operator who has disabled it would see
// every upgrade fail health-check despite the daemon being healthy.
// The state file is osvbngd's own component-orchestrator readiness
// self-assessment and is the authoritative signal.
type HealthChecker struct {
	Supervisor    *Supervisor
	StateFilePath string

	// Tunables. Zero values pick the spec defaults.
	OverallTimeout time.Duration // default 60s
	StallLimit     time.Duration // default 30s
	PollInterval   time.Duration // default 1s
	StateFileGrace time.Duration // default 5s — race window between unit active and first state write

	// Clock is injectable for deterministic tests.
	Now   func() time.Time
	Sleep func(time.Duration)
}

// healthStateFile is the on-disk shape osvbngd writes to /run/osvbng/state.
// Mirrors pkg/component/statefile.go's Payload, redeclared here so
// pkg/upgrade has no inverse import dependency on pkg/component.
type healthStateFile struct {
	State     string    `json:"state"`
	Sequence  uint64    `json:"sequence"`
	UpdatedAt time.Time `json:"updated_at"`
	ModTime   time.Time `json:"-"`
}

// NewHealthChecker returns a HealthChecker with production defaults.
func NewHealthChecker(sv *Supervisor) *HealthChecker {
	return &HealthChecker{
		Supervisor:    sv,
		StateFilePath: "/run/osvbng/state",
	}
}

func (h *HealthChecker) defaults() {
	if h.OverallTimeout == 0 {
		h.OverallTimeout = 60 * time.Second
	}
	if h.StallLimit == 0 {
		h.StallLimit = 30 * time.Second
	}
	if h.PollInterval == 0 {
		h.PollInterval = 1 * time.Second
	}
	if h.StateFileGrace == 0 {
		h.StateFileGrace = 5 * time.Second
	}
	if h.Now == nil {
		h.Now = time.Now
	}
	if h.Sleep == nil {
		h.Sleep = time.Sleep
	}
}

// WaitHealthy polls until the daemon is healthy, has failed, has
// stalled, or has timed out. Returns the categorised outcome plus a
// human-readable message suitable for the operator-facing event log.
//
// Loop summary: systemd ActiveState gates the major branches (failed →
// stop; activating → keep polling and watch for state-file progress;
// active → check state-file content). The state file's Sequence field
// is the primary stall-detection signal.
func (h *HealthChecker) WaitHealthy(ctx context.Context) (HealthResult, string, error) {
	h.defaults()
	start := h.Now()
	deadline := start.Add(h.OverallTimeout)
	graceUntil := start.Add(h.StateFileGrace)
	var lastSeq uint64
	lastSeqAt := start

	for {
		now := h.Now()
		if now.After(deadline) {
			return HealthTimeout, fmt.Sprintf("overall health timeout (%v) reached", h.OverallTimeout), nil
		}
		if ctx.Err() != nil {
			return HealthTimeout, "context cancelled while waiting for daemon to become ready", ctx.Err()
		}

		st, err := h.Supervisor.Show(ctx)
		if err != nil {
			return HealthTimeout, fmt.Sprintf("systemctl show error: %v", err), err
		}

		if st.IsFailed() {
			return HealthFailed, fmt.Sprintf("systemd reports unit failed (SubState=%s, Result=%s)", st.SubState, st.Result), nil
		}

		if st.IsActive() {
			sf, sfErr := h.readStateFile()
			if sfErr != nil {
				if errors.Is(sfErr, fs.ErrNotExist) {
					if h.Now().Before(graceUntil) {
						h.Sleep(h.PollInterval)
						continue
					}
					return HealthTimeout, fmt.Sprintf("unit active but state file %s absent past grace (%v)", h.StateFilePath, h.StateFileGrace), nil
				}
				return HealthTimeout, fmt.Sprintf("read state file %s: %v", h.StateFilePath, sfErr), sfErr
			}

			if sf.Sequence > lastSeq {
				lastSeq = sf.Sequence
				lastSeqAt = h.Now()
			}

			switch sf.State {
			case "ready":
				return HealthOK, "daemon ready", nil
			case "degraded":
				return HealthDegraded, fmt.Sprintf("daemon reports degraded state (sequence=%d)", sf.Sequence), nil
			case "starting", "restoring":
				// keep polling
			default:
				return HealthInvalidState, fmt.Sprintf("state file holds unknown state %q", sf.State), nil
			}

			h.Sleep(h.PollInterval)
			continue
		}

		if st.IsActivating() {
			sf, sfErr := h.readStateFile()
			if sfErr == nil && sf.Sequence > lastSeq {
				lastSeq = sf.Sequence
				lastSeqAt = h.Now()
			}
			if h.Now().Sub(lastSeqAt) > h.StallLimit {
				return HealthStalled, fmt.Sprintf("no state-file progress in %v (last sequence=%d)", h.StallLimit, lastSeq), nil
			}
			h.Sleep(h.PollInterval)
			continue
		}

		h.Sleep(h.PollInterval)
	}
}

func (h *HealthChecker) readStateFile() (*healthStateFile, error) {
	data, err := os.ReadFile(h.StateFilePath)
	if err != nil {
		return nil, err
	}
	var sf healthStateFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("decode state file %s: %w", h.StateFilePath, err)
	}
	info, statErr := os.Stat(h.StateFilePath)
	if statErr == nil {
		sf.ModTime = info.ModTime()
	}
	return &sf, nil
}
