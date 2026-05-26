// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

// Package upgrade implements the on-host primitives the osvbngcli upgrade
// builtin uses to apply, roll back, and inspect signed osvbng release
// tarballs. Every primitive operates directly on the filesystem and
// process state — none make daemon API calls — so the upgrade flow
// remains usable when osvbngd is stopped or unreachable.
package upgrade

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// JournalVersion is the schema version of the on-disk journal payload.
// Bump only on incompatible field changes; readers must refuse newer
// versions to avoid acting on a state they don't fully understand.
const JournalVersion = 1

// JournalState is the durable record of an in-flight or completed upgrade.
// The journal is updated before AND after every irreversible step so that
// a process interrupted at any point (operator Ctrl-C, crash, power loss)
// leaves enough state on disk for a later `upgrade rollback` to resume
// from a known partial state.
type JournalState struct {
	Version            int       `json:"version"`
	From               string    `json:"from"`
	To                 string    `json:"to"`
	Tarball            string    `json:"tarball,omitempty"`
	StartedAt          time.Time `json:"started_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	Phase              string    `json:"phase"`
	CompletedArtifacts []string  `json:"completed_artifacts,omitempty"`
	PendingArtifacts   []string  `json:"pending_artifacts,omitempty"`
	Message            string    `json:"message,omitempty"`
}

// Journal is an atomic on-disk record of upgrade progress. All writes
// land via a temp file in the same directory followed by rename(2), so
// concurrent readers always see either the old payload or the new — never
// a partial write.
type Journal struct {
	path string
}

// NewJournal binds a Journal to a filesystem path. The parent directory
// is NOT created; callers are responsible for ensuring it exists before
// any Write call (the apply flow does MkdirAll on /var/opt/osvbng/ at
// entry).
func NewJournal(path string) *Journal {
	return &Journal{path: path}
}

// Path returns the underlying file path. Useful for diagnostics and for
// callers that need to reference the journal location in operator
// messages.
func (j *Journal) Path() string {
	return j.path
}

// Read decodes the journal from disk. Returns fs.ErrNotExist (wrapped)
// when no journal has been written yet — callers detect this with
// errors.Is(err, fs.ErrNotExist) and treat it as "no prior upgrade".
func (j *Journal) Read() (*JournalState, error) {
	data, err := os.ReadFile(j.path)
	if err != nil {
		return nil, err
	}
	var state JournalState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("decode journal %s: %w", j.path, err)
	}
	if state.Version > JournalVersion {
		return nil, fmt.Errorf("journal %s has unsupported version %d (this build understands up to %d)",
			j.path, state.Version, JournalVersion)
	}
	return &state, nil
}

// Write atomically replaces the journal with the supplied state.
// UpdatedAt is stamped here so callers never have to remember; Version
// is filled in if zero. The temp file lives in the same directory as
// the target so rename(2) is guaranteed atomic on the same filesystem.
func (j *Journal) Write(state *JournalState) error {
	if state == nil {
		return errors.New("journal: cannot write nil state")
	}
	if state.Version == 0 {
		state.Version = JournalVersion
	}
	state.UpdatedAt = time.Now().UTC()

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode journal: %w", err)
	}

	dir := filepath.Dir(j.path)
	tmp, err := os.CreateTemp(dir, ".upgrade-state-*.json")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	cleanup := func() { _ = os.Remove(tmp.Name()) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write temp journal: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("fsync temp journal: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp journal: %w", err)
	}

	if err := os.Chmod(tmp.Name(), 0o640); err != nil {
		cleanup()
		return fmt.Errorf("chmod temp journal: %w", err)
	}

	if err := os.Rename(tmp.Name(), j.path); err != nil {
		cleanup()
		return fmt.Errorf("rename temp journal to %s: %w", j.path, err)
	}
	return nil
}

// SetPhase loads the current journal, updates only the Phase field
// (preserving every other field), and writes the result atomically.
// Used by the apply flow between irreversible steps to record progress.
//
// SetPhase refuses to create a journal from scratch — if no journal
// exists, the caller is expected to use Write with a fully-populated
// state. This avoids accidentally writing an apply-style journal during
// a rollback resumption.
func (j *Journal) SetPhase(phase string) error {
	state, err := j.Read()
	if err != nil {
		return fmt.Errorf("set phase %q: %w", phase, err)
	}
	state.Phase = phase
	return j.Write(state)
}

// Clear removes the journal file. Used after a successful apply has
// committed (the journal becomes historical) and at the end of a
// successful rollback. Returns nil when the journal is already absent.
func (j *Journal) Clear() error {
	err := os.Remove(j.path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove journal %s: %w", j.path, err)
	}
	return nil
}
