// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// makeArtifact creates a regular file in tmpdir mimicking a production
// installed artifact at the given absolute-looking path under tmpdir,
// returns the absolute path written.
func makeArtifact(t *testing.T, tmpdir, relPath string, body []byte, mode os.FileMode) string {
	t.Helper()
	target := filepath.Join(tmpdir, relPath)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(target, body, mode); err != nil {
		t.Fatalf("write: %v", err)
	}
	return target
}

func TestSnapshotRegularFilePreservesMetadata(t *testing.T) {
	root := t.TempDir()
	rbRoot := filepath.Join(root, "rollback")

	body := []byte("osvbngd-binary-contents")
	artifact := makeArtifact(t, root, "usr/local/bin/osvbngd", body, 0o755)

	manifest := &Manifest{
		OsvbngVersion: "0.13.1",
		Artifacts: []ManifestArtifact{
			{Path: artifact, Source: "osvbngd", SHA256: "x", Mode: "0755"},
		},
	}

	snapDir, meta, err := Snapshot(rbRoot, "0.13.0", "0.13.1", manifest)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if !strings.HasSuffix(snapDir, "0.13.0") {
		t.Fatalf("snapDir %q does not end with from-version", snapDir)
	}
	if len(meta.Entries) != 1 {
		t.Fatalf("entries: got %d, want 1", len(meta.Entries))
	}

	entry := meta.Entries[0]
	if !entry.Present {
		t.Fatalf("entry.Present = false, want true")
	}
	if entry.Kind != ArtifactKindRegular {
		t.Fatalf("entry.Kind = %q, want %q", entry.Kind, ArtifactKindRegular)
	}
	if entry.Mode != 0o755 {
		t.Fatalf("entry.Mode = %o, want 0755", entry.Mode)
	}
	if entry.Size != int64(len(body)) {
		t.Fatalf("entry.Size = %d, want %d", entry.Size, len(body))
	}
	wantHash := sha256.Sum256(body)
	if entry.SHA256 != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("entry.SHA256 = %q, want %q", entry.SHA256, hex.EncodeToString(wantHash[:]))
	}

	// Verify backup file actually exists with the right content.
	backup := filepath.Join(snapDir, entry.BackupRelpath)
	got, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(got) != string(body) {
		t.Fatalf("backup content differs from original")
	}
}

func TestSnapshotPreservesSymlink(t *testing.T) {
	root := t.TempDir()
	rbRoot := filepath.Join(root, "rollback")

	// Create a real target then a symlink to it.
	target := makeArtifact(t, root, "real/binary", []byte("real-body"), 0o755)
	linkPath := filepath.Join(root, "usr/local/bin/osvbngd-link")
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	manifest := &Manifest{
		OsvbngVersion: "0.13.1",
		Artifacts: []ManifestArtifact{
			{Path: linkPath, Source: "osvbngd", SHA256: "x"},
		},
	}

	_, meta, err := Snapshot(rbRoot, "0.13.0", "0.13.1", manifest)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	entry := meta.Entries[0]
	if entry.Kind != ArtifactKindSymlink {
		t.Fatalf("entry.Kind = %q, want %q", entry.Kind, ArtifactKindSymlink)
	}
	if entry.SymlinkTarget != target {
		t.Fatalf("SymlinkTarget = %q, want %q", entry.SymlinkTarget, target)
	}
	if entry.BackupRelpath != "" {
		t.Fatalf("BackupRelpath for symlink = %q, want empty", entry.BackupRelpath)
	}
}

func TestSnapshotMarksAbsentArtifactAsNotPresent(t *testing.T) {
	root := t.TempDir()
	rbRoot := filepath.Join(root, "rollback")

	missing := filepath.Join(root, "usr/local/bin/new-artifact")

	manifest := &Manifest{
		OsvbngVersion: "0.13.1",
		Artifacts: []ManifestArtifact{
			{Path: missing, Source: "new-artifact", SHA256: "x"},
		},
	}

	_, meta, err := Snapshot(rbRoot, "0.13.0", "0.13.1", manifest)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	entry := meta.Entries[0]
	if entry.Present {
		t.Fatalf("absent artifact recorded as Present=true")
	}
}

func TestLoadSnapshotMetadataRoundtrip(t *testing.T) {
	root := t.TempDir()
	rbRoot := filepath.Join(root, "rollback")

	artifact := makeArtifact(t, root, "usr/local/bin/osvbngd", []byte("xx"), 0o755)
	manifest := &Manifest{
		Artifacts: []ManifestArtifact{
			{Path: artifact, Source: "osvbngd", SHA256: "x"},
		},
	}

	snapDir, original, err := Snapshot(rbRoot, "0.13.0", "0.13.1", manifest)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	loaded, err := LoadSnapshotMetadata(snapDir)
	if err != nil {
		t.Fatalf("LoadSnapshotMetadata: %v", err)
	}
	if loaded.FromVersion != original.FromVersion {
		t.Fatalf("FromVersion: %q vs %q", loaded.FromVersion, original.FromVersion)
	}
	if loaded.ToVersion != original.ToVersion {
		t.Fatalf("ToVersion: %q vs %q", loaded.ToVersion, original.ToVersion)
	}
	if len(loaded.Entries) != len(original.Entries) {
		t.Fatalf("entries len: %d vs %d", len(loaded.Entries), len(original.Entries))
	}
}

func TestPruneSnapshotsKeepsMostRecentN(t *testing.T) {
	root := t.TempDir()
	rbRoot := filepath.Join(root, "rollback")

	artifact := makeArtifact(t, root, "usr/local/bin/osvbngd", []byte("x"), 0o755)
	manifest := &Manifest{
		Artifacts: []ManifestArtifact{
			{Path: artifact, Source: "osvbngd", SHA256: "x"},
		},
	}

	versions := []string{"0.12.0", "0.12.1", "0.12.2", "0.13.0"}
	for i, v := range versions {
		snapDir, _, err := Snapshot(rbRoot, v, "0.99.0", manifest)
		if err != nil {
			t.Fatalf("Snapshot %s: %v", v, err)
		}
		// Hand-stamp distinct CreatedAt so prune ordering is deterministic.
		meta, err := LoadSnapshotMetadata(snapDir)
		if err != nil {
			t.Fatalf("LoadSnapshotMetadata: %v", err)
		}
		meta.CreatedAt = time.Date(2026, 1, 1+i, 0, 0, 0, 0, time.UTC)
		if err := writeSnapshotMetadata(snapDir, meta); err != nil {
			t.Fatalf("rewrite metadata: %v", err)
		}
	}

	if err := PruneSnapshots(rbRoot, 2); err != nil {
		t.Fatalf("PruneSnapshots: %v", err)
	}

	for i, v := range versions {
		dir := filepath.Join(rbRoot, v)
		_, err := os.Stat(dir)
		shouldExist := i >= 2
		if shouldExist && err != nil {
			t.Fatalf("snapshot %s should exist after prune: %v", v, err)
		}
		if !shouldExist && err == nil {
			t.Fatalf("snapshot %s should have been pruned but still exists", v)
		}
	}
}

func TestPruneSnapshotsTolerantOfMissingRollbackRoot(t *testing.T) {
	root := t.TempDir()
	rbRoot := filepath.Join(root, "rollback-never-created")
	if err := PruneSnapshots(rbRoot, 1); err != nil {
		t.Fatalf("PruneSnapshots on missing root: %v", err)
	}
}

func TestSnapshotRefusesInvalidKeep(t *testing.T) {
	root := t.TempDir()
	if err := PruneSnapshots(root, 0); err == nil {
		t.Fatal("PruneSnapshots accepted keep=0")
	}
}
