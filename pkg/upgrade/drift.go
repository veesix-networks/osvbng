// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// DriftFinding describes a single artifact whose on-disk content does
// not match the manifest's optional expected_current_sha256 field.
// Drift is warn-not-refuse: the upgrade proceeds, and the rollback
// snapshot captures the drifted bytes so the operator's hand-edits are
// preserved as a recoverable side effect.
type DriftFinding struct {
	Path             string
	ExpectedSHA256   string
	OnDiskSHA256     string
	OnDiskAbsent     bool
}

// String renders a DriftFinding into a one-line operator-facing
// warning suitable for routing through a ProgressReporter.Warn call.
func (d DriftFinding) String() string {
	if d.OnDiskAbsent {
		return fmt.Sprintf("expected artifact %s is absent on disk (manifest expected sha256:%s)",
			d.Path, d.ExpectedSHA256)
	}
	return fmt.Sprintf("operator-modified %s will be overwritten by upgrade "+
		"(manifest expects sha256:%s, found sha256:%s); rollback snapshot will "+
		"preserve the modified bytes",
		d.Path, d.ExpectedSHA256, d.OnDiskSHA256)
}

// DetectDrift compares each manifest artifact's expected_current_sha256
// (when set) against the actual on-disk sha256 at artifact.Path.
// Artifacts without expected_current_sha256 are skipped — drift
// detection is opt-in per-artifact from the manifest authoring side.
//
// Returns one DriftFinding per mismatch. The slice is empty (and the
// error nil) when no drift is detected or no artifact opts in.
// Returns an error only on unexpected I/O failures (permissions, etc.);
// a missing artifact with an expected hash is treated as drift, not as
// an error.
func DetectDrift(manifest *Manifest) ([]DriftFinding, error) {
	if manifest == nil {
		return nil, errors.New("drift: manifest is nil")
	}

	var findings []DriftFinding
	for _, art := range manifest.Artifacts {
		if art.ExpectedCurrentSHA256 == "" {
			continue
		}
		hash, err := hashFileAt(art.Path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				findings = append(findings, DriftFinding{
					Path:           art.Path,
					ExpectedSHA256: art.ExpectedCurrentSHA256,
					OnDiskAbsent:   true,
				})
				continue
			}
			return nil, fmt.Errorf("drift hash %s: %w", art.Path, err)
		}
		if hash == "" {
			continue // symlink — no content hash, drift detection N/A
		}
		if !strings.EqualFold(hash, art.ExpectedCurrentSHA256) {
			findings = append(findings, DriftFinding{
				Path:           art.Path,
				ExpectedSHA256: art.ExpectedCurrentSHA256,
				OnDiskSHA256:   hash,
			})
		}
	}
	return findings, nil
}

func hashFileAt(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		// Symlinks have no content hash; the symlink target is the
		// rollback-recovery dimension. Treat as "no expected hash" to
		// avoid spurious drift on symlinked install layouts.
		return "", nil
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("not a regular file: %s (mode %v)", path, info.Mode())
	}
	return sha256File(path)
}
