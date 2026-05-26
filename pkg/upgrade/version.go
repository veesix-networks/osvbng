// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// CurrentInstalledVersion returns the version of osvbngd actually
// installed on disk. NOT pkg/version.Version — the running osvbngcli
// process may be holding a stale build-time version after an apply
// that replaced the on-disk binary (Linux open-file semantics).
//
// stateRoot:    /var/opt/osvbng/ — where current-manifest.yaml lives
// daemonPath:   /usr/local/bin/osvbngd — fallback when no manifest is
//               recorded yet (fresh install, never upgraded)
// commander:    injectable so tests don't need a real osvbngd binary
//
// Lookup order:
//   1. /var/opt/osvbng/current-manifest.yaml — written by the previous
//      successful apply's commit step. Authoritative for any
//      previously-upgraded box.
//   2. exec daemonPath -version — for never-upgraded boxes where no
//      manifest exists yet. Parses the "<version> (<commit>) built on
//      ..." format pkg/version.Full produces.
func CurrentInstalledVersion(ctx context.Context, stateRoot, daemonPath string, commander Commander) (string, error) {
	manifestPath := filepath.Join(stateRoot, "current-manifest.yaml")
	m, err := ParseManifestFile(manifestPath)
	if err == nil {
		return m.OsvbngVersion, nil
	}
	if !errors.Is(err, fs.ErrNotExist) && !isMissingPath(err) {
		return "", fmt.Errorf("read current manifest: %w", err)
	}

	if commander == nil {
		commander = execCommander{}
	}
	out, err := commander.Run(ctx, daemonPath, "-version")
	if err != nil {
		return "", fmt.Errorf("exec %s -version: %w (%s)", daemonPath, err, strings.TrimSpace(string(out)))
	}
	v := parseVersionOutput(string(out))
	if v == "" {
		return "", fmt.Errorf("could not parse version from %s -version output: %q", daemonPath, string(out))
	}
	return v, nil
}

// isMissingPath returns true when err describes a missing file or
// directory, regardless of whether the standard library wraps it
// behind os.PathError vs the os/exec path. ParseManifestFile wraps
// the underlying os.ReadFile error so errors.Is(err, fs.ErrNotExist)
// works; we keep this helper for resilience against future error
// wrapping changes.
func isMissingPath(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, fs.ErrNotExist) {
		return true
	}
	var pathErr *exec.Error
	if errors.As(err, &pathErr) {
		return errors.Is(pathErr.Err, fs.ErrNotExist)
	}
	return false
}

// parseVersionOutput extracts the version token from pkg/version.Full
// output: "v0.13.0 (abc1234) built on 2026-06-15T08:00:00Z". The first
// whitespace-separated token is the version. Returns empty string on
// empty input.
func parseVersionOutput(out string) string {
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return ""
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// WriteCurrentManifest copies the new manifest into the state root as
// current-manifest.yaml after a successful apply. CurrentInstalledVersion
// reads this on subsequent invocations. Atomic via tmp+rename so a
// crash mid-write never leaves a partial manifest.
func WriteCurrentManifest(stateRoot string, m *Manifest) error {
	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	target := filepath.Join(stateRoot, "current-manifest.yaml")
	tmp, err := os.CreateTemp(stateRoot, ".current-manifest-*.yaml")
	if err != nil {
		return fmt.Errorf("create temp manifest: %w", err)
	}
	cleanup := func() { _ = os.Remove(tmp.Name()) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o640); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		cleanup()
		return fmt.Errorf("rename current manifest: %w", err)
	}
	return nil
}
