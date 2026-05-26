// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

// ArtifactKindRegular and friends classify the on-disk type recorded
// when an artifact is snapshotted. Symlinks and regular files are the
// only supported kinds for Tier A; any other type (device, FIFO,
// socket) is rejected at snapshot time because rolling it back safely
// would require type-specific knowledge the upgrade flow doesn't have.
const (
	ArtifactKindRegular = "regular"
	ArtifactKindSymlink = "symlink"
)

// SnapshotEntry is the recorded per-artifact metadata. The rollback
// flow reads these to restore not just the byte content but the
// uid/gid/mode/type the file had before the upgrade.
type SnapshotEntry struct {
	Path          string `yaml:"path"`
	Kind          string `yaml:"kind"`
	UID           int    `yaml:"uid"`
	GID           int    `yaml:"gid"`
	Mode          uint32 `yaml:"mode"`
	Size          int64  `yaml:"size"`
	SHA256        string `yaml:"sha256,omitempty"`
	SymlinkTarget string `yaml:"symlink_target,omitempty"`
	BackupRelpath string `yaml:"backup_relpath"`
	Present       bool   `yaml:"present"`
}

// SnapshotMetadata is the manifest of a single rollback snapshot.
// Saved as metadata.yaml at the snapshot root. Newer fields gained
// during evolution append with the schema-version field acting as a
// hard backstop against forward-incompatible reads.
type SnapshotMetadata struct {
	SchemaVersion int             `yaml:"schema_version"`
	CreatedAt     time.Time       `yaml:"created_at"`
	FromVersion   string          `yaml:"from_version"`
	ToVersion     string          `yaml:"to_version"`
	Entries       []SnapshotEntry `yaml:"entries"`
}

// SnapshotSchemaVersion is the current schema version of metadata.yaml.
const SnapshotSchemaVersion = 1

// Snapshot copies the current on-disk state of every artifact in
// manifest into a per-from-version directory under rollbackRoot, and
// writes a metadata.yaml capturing uid/gid/mode/type/sha256/symlink_target
// for each entry.
//
// rollbackRoot:   parent dir for snapshots (typically /var/opt/osvbng/rollback)
// fromVersion:    current installed version; snapshot lands at rollbackRoot/<fromVersion>
// toVersion:      the version we are upgrading TO (for forensics; recorded
//                  in metadata)
// manifest:       lists which artifact paths to snapshot
//
// The artifacts in the manifest are the FUTURE state. We snapshot the
// CURRENT on-disk state at each artifact.Path. An artifact that is
// absent on disk (fresh add) is recorded with Present=false so rollback
// knows to remove the newly-installed file rather than try to restore
// content that never existed.
func Snapshot(rollbackRoot, fromVersion, toVersion string, manifest *Manifest) (string, *SnapshotMetadata, error) {
	if manifest == nil {
		return "", nil, errors.New("snapshot: manifest is nil")
	}
	if fromVersion == "" {
		return "", nil, errors.New("snapshot: fromVersion is empty")
	}

	snapDir := filepath.Join(rollbackRoot, fromVersion)
	if err := os.MkdirAll(snapDir, 0o750); err != nil {
		return "", nil, fmt.Errorf("mkdir snapshot dir %s: %w", snapDir, err)
	}

	meta := &SnapshotMetadata{
		SchemaVersion: SnapshotSchemaVersion,
		CreatedAt:     time.Now().UTC(),
		FromVersion:   fromVersion,
		ToVersion:     toVersion,
	}

	for _, art := range manifest.Artifacts {
		entry, err := snapshotOne(snapDir, art.Path)
		if err != nil {
			return snapDir, nil, fmt.Errorf("snapshot %s: %w", art.Path, err)
		}
		meta.Entries = append(meta.Entries, entry)
	}

	if err := writeSnapshotMetadata(snapDir, meta); err != nil {
		return snapDir, nil, err
	}

	return snapDir, meta, nil
}

// LoadSnapshotMetadata reads a previously-written metadata.yaml from a
// snapshot directory. Used by the rollback flow to discover what to
// restore.
func LoadSnapshotMetadata(snapDir string) (*SnapshotMetadata, error) {
	path := filepath.Join(snapDir, "metadata.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read snapshot metadata %s: %w", path, err)
	}
	var meta SnapshotMetadata
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("decode snapshot metadata %s: %w", path, err)
	}
	if meta.SchemaVersion == 0 {
		return nil, fmt.Errorf("snapshot metadata %s has no schema_version", path)
	}
	if meta.SchemaVersion > SnapshotSchemaVersion {
		return nil, fmt.Errorf("snapshot metadata %s has schema_version %d (this build understands up to %d)",
			path, meta.SchemaVersion, SnapshotSchemaVersion)
	}
	return &meta, nil
}

// PruneSnapshots removes snapshot directories under rollbackRoot beyond
// the most recent `keep` entries (sorted by metadata CreatedAt). Used
// after a successful apply to bound rollback storage to N-1 by default.
//
// Directories whose metadata.yaml cannot be read are kept rather than
// pruned, on the principle that mystery state is safer left alone than
// deleted.
func PruneSnapshots(rollbackRoot string, keep int) error {
	if keep < 1 {
		return fmt.Errorf("prune: keep must be >= 1, got %d", keep)
	}

	entries, err := os.ReadDir(rollbackRoot)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("readdir %s: %w", rollbackRoot, err)
	}

	type snapInfo struct {
		dir       string
		createdAt time.Time
	}
	var snaps []snapInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(rollbackRoot, e.Name())
		meta, err := LoadSnapshotMetadata(dir)
		if err != nil {
			continue
		}
		snaps = append(snaps, snapInfo{dir: dir, createdAt: meta.CreatedAt})
	}
	if len(snaps) <= keep {
		return nil
	}
	sort.Slice(snaps, func(i, j int) bool {
		return snaps[i].createdAt.After(snaps[j].createdAt)
	})

	for _, s := range snaps[keep:] {
		if err := os.RemoveAll(s.dir); err != nil {
			return fmt.Errorf("remove snapshot %s: %w", s.dir, err)
		}
	}
	return nil
}

func snapshotOne(snapDir, targetPath string) (SnapshotEntry, error) {
	info, err := os.Lstat(targetPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return SnapshotEntry{
				Path:    targetPath,
				Present: false,
			}, nil
		}
		return SnapshotEntry{}, fmt.Errorf("lstat: %w", err)
	}

	mode := info.Mode()
	entry := SnapshotEntry{
		Path:    targetPath,
		Mode:    uint32(mode.Perm()),
		Size:    info.Size(),
		Present: true,
	}

	if sys, ok := info.Sys().(*syscall.Stat_t); ok {
		entry.UID = int(sys.Uid)
		entry.GID = int(sys.Gid)
	}

	switch {
	case mode&os.ModeSymlink != 0:
		entry.Kind = ArtifactKindSymlink
		target, err := os.Readlink(targetPath)
		if err != nil {
			return SnapshotEntry{}, fmt.Errorf("readlink: %w", err)
		}
		entry.SymlinkTarget = target
		entry.BackupRelpath = "" // symlinks are reconstructed from metadata, no bytes stored
	case mode.IsRegular():
		entry.Kind = ArtifactKindRegular
		rel := mangleArtifactRelpath(targetPath)
		backupPath := filepath.Join(snapDir, rel)
		if err := os.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
			return SnapshotEntry{}, fmt.Errorf("mkdir backup parent: %w", err)
		}
		hash, err := copyAndHash(targetPath, backupPath)
		if err != nil {
			return SnapshotEntry{}, fmt.Errorf("copy: %w", err)
		}
		entry.SHA256 = hash
		entry.BackupRelpath = rel
	default:
		return SnapshotEntry{}, fmt.Errorf("unsupported file kind for %s: mode=%v", targetPath, mode)
	}

	return entry, nil
}

// mangleArtifactRelpath converts an absolute artifact path into a
// relative path under the snapshot dir. Leading / is dropped; the rest
// of the path structure is preserved so the backup dir mirrors the
// production layout (e.g. /usr/local/bin/osvbngd -> usr/local/bin/osvbngd).
func mangleArtifactRelpath(absPath string) string {
	return strings.TrimPrefix(absPath, string(filepath.Separator))
}

func copyAndHash(src, dst string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return "", err
	}
	defer out.Close()

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, h), in); err != nil {
		return "", err
	}
	if err := out.Sync(); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeSnapshotMetadata(snapDir string, meta *SnapshotMetadata) error {
	out, err := yaml.Marshal(meta)
	if err != nil {
		return fmt.Errorf("encode snapshot metadata: %w", err)
	}
	target := filepath.Join(snapDir, "metadata.yaml")
	tmp, err := os.CreateTemp(snapDir, ".metadata-*.yaml")
	if err != nil {
		return fmt.Errorf("temp metadata file: %w", err)
	}
	cleanup := func() { _ = os.Remove(tmp.Name()) }
	if _, err := tmp.Write(out); err != nil {
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
	return os.Rename(tmp.Name(), target)
}
