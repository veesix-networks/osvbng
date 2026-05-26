// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectDriftSkipsArtifactsWithoutExpectedHash(t *testing.T) {
	dir := t.TempDir()
	artifact := filepath.Join(dir, "osvbngd")
	if err := os.WriteFile(artifact, []byte("anything"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}

	manifest := &Manifest{
		Artifacts: []ManifestArtifact{
			{Path: artifact, Source: "osvbngd", SHA256: "x"}, // no expected_current_sha256
		},
	}

	findings, err := DetectDrift(manifest)
	if err != nil {
		t.Fatalf("DetectDrift: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %v, want 0", findings)
	}
}

func TestDetectDriftReportsMismatch(t *testing.T) {
	dir := t.TempDir()
	artifact := filepath.Join(dir, "osvbngd")
	body := []byte("operator hand-edit")
	if err := os.WriteFile(artifact, body, 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}

	manifest := &Manifest{
		Artifacts: []ManifestArtifact{
			{
				Path:                  artifact,
				Source:                "osvbngd",
				SHA256:                "x",
				ExpectedCurrentSHA256: "deadbeef00000000000000000000000000000000000000000000000000000000",
			},
		},
	}

	findings, err := DetectDrift(manifest)
	if err != nil {
		t.Fatalf("DetectDrift: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %v, want 1", findings)
	}
	if findings[0].OnDiskAbsent {
		t.Fatalf("OnDiskAbsent = true for present file")
	}
	if findings[0].OnDiskSHA256 == "" {
		t.Fatalf("OnDiskSHA256 is empty")
	}

	msg := findings[0].String()
	if !strings.Contains(msg, "operator-modified") {
		t.Fatalf("string did not mention operator-modified: %q", msg)
	}
	if !strings.Contains(msg, "rollback snapshot will preserve") {
		t.Fatalf("string did not reassure about rollback: %q", msg)
	}
}

func TestDetectDriftReportsAbsence(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "absent-artifact")

	manifest := &Manifest{
		Artifacts: []ManifestArtifact{
			{
				Path:                  missing,
				Source:                "x",
				SHA256:                "x",
				ExpectedCurrentSHA256: "deadbeef00000000000000000000000000000000000000000000000000000000",
			},
		},
	}

	findings, err := DetectDrift(manifest)
	if err != nil {
		t.Fatalf("DetectDrift: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings = %v, want 1", findings)
	}
	if !findings[0].OnDiskAbsent {
		t.Fatalf("OnDiskAbsent = false for missing file")
	}
	if !strings.Contains(findings[0].String(), "absent") {
		t.Fatalf("string did not mention absent: %q", findings[0].String())
	}
}

func TestDetectDriftNoFindingsOnMatch(t *testing.T) {
	dir := t.TempDir()
	body := []byte("matching-bytes")
	artifact := filepath.Join(dir, "osvbngd")
	if err := os.WriteFile(artifact, body, 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	hash, err := sha256File(artifact)
	if err != nil {
		t.Fatalf("sha256File: %v", err)
	}

	manifest := &Manifest{
		Artifacts: []ManifestArtifact{
			{
				Path:                  artifact,
				Source:                "osvbngd",
				SHA256:                "x",
				ExpectedCurrentSHA256: hash,
			},
		},
	}

	findings, err := DetectDrift(manifest)
	if err != nil {
		t.Fatalf("DetectDrift: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %v, want 0", findings)
	}
}

func TestDetectDriftIgnoresSymlinks(t *testing.T) {
	dir := t.TempDir()
	realFile := filepath.Join(dir, "real")
	if err := os.WriteFile(realFile, []byte("real-content"), 0o644); err != nil {
		t.Fatalf("write real: %v", err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(realFile, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	manifest := &Manifest{
		Artifacts: []ManifestArtifact{
			{
				Path:                  link,
				Source:                "x",
				SHA256:                "x",
				ExpectedCurrentSHA256: "deadbeef00000000000000000000000000000000000000000000000000000000",
			},
		},
	}

	findings, err := DetectDrift(manifest)
	if err != nil {
		t.Fatalf("DetectDrift: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings for symlink = %v, want 0 (drift ignored for symlinks)", findings)
	}
}
