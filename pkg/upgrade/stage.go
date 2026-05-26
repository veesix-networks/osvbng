// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// MaxArtifactBytes caps the size of a single artifact extracted from a
// tarball. Anything larger is refused before any bytes touch disk. Tier-A
// tarballs ship Go binaries and VPP plugins; 200 MiB is comfortably above
// the largest of those while still bounding a malicious or corrupted
// tarball.
const MaxArtifactBytes = 200 * 1024 * 1024

// Staging holds the extracted contents of an upgrade tarball plus the
// metadata needed to cross-check artifacts against the manifest.
type Staging struct {
	Dir         string         // extraction root, sibling of the tarball
	TarballPath string         // original tarball path (for forensics / quarantine)
	Manifest    *Manifest      // parsed manifest from the tarball
	Hashes      map[string]string
}

// Cleanup removes the staging directory. Safe to call multiple times.
// The apply flow defers Cleanup at staging time so all error paths
// (signature mismatch, hash mismatch, abort) leave nothing behind.
func (s *Staging) Cleanup() error {
	if s.Dir == "" {
		return nil
	}
	err := os.RemoveAll(s.Dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// ExtractTarball reads a gzipped tar tarball and writes its contents
// to a freshly-created subdirectory beside the tarball itself (i.e.
// MkdirTemp at filepath.Dir(tarballPath)). Placing the staging dir on
// the same partition as the operator-supplied tarball means a 2 GB
// tarball on /tmp does not silently land on a different (potentially
// full) partition.
//
// Each tar entry is filtered for safety before any byte is written:
//   - Entry names are cleaned and rejected if they escape the staging dir.
//   - Only regular files and (within-staging) directories are accepted;
//     symlinks, hardlinks, devices, FIFOs, and sockets are rejected.
//   - Per-entry size cap (MaxArtifactBytes) is enforced.
//
// The returned Staging has hashes for every regular-file entry under
// the staging dir, keyed by their tarball-relative path; the apply flow
// uses these to cross-check the manifest's per-artifact sha256.
func ExtractTarball(tarballPath string) (*Staging, error) {
	tarballPath = filepath.Clean(tarballPath)
	parent := filepath.Dir(tarballPath)

	dir, err := os.MkdirTemp(parent, "osvbng-upgrade-")
	if err != nil {
		return nil, fmt.Errorf("create staging dir under %s: %w", parent, err)
	}

	staging := &Staging{
		Dir:         dir,
		TarballPath: tarballPath,
		Hashes:      make(map[string]string),
	}

	f, err := os.Open(tarballPath)
	if err != nil {
		_ = staging.Cleanup()
		return nil, fmt.Errorf("open tarball %s: %w", tarballPath, err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		_ = staging.Cleanup()
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			_ = staging.Cleanup()
			return nil, fmt.Errorf("read tar header: %w", err)
		}

		cleanName, err := safeTarEntryPath(hdr.Name, dir)
		if err != nil {
			_ = staging.Cleanup()
			return nil, err
		}

		target := filepath.Join(dir, cleanName)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				_ = staging.Cleanup()
				return nil, fmt.Errorf("mkdir %s: %w", target, err)
			}
		case tar.TypeReg:
			if hdr.Size > MaxArtifactBytes {
				_ = staging.Cleanup()
				return nil, fmt.Errorf("tar entry %q size %d exceeds limit %d",
					hdr.Name, hdr.Size, MaxArtifactBytes)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				_ = staging.Cleanup()
				return nil, fmt.Errorf("mkdir parent of %s: %w", target, err)
			}
			hash, err := writeEntry(tr, target, hdr.Size)
			if err != nil {
				_ = staging.Cleanup()
				return nil, fmt.Errorf("write %s: %w", target, err)
			}
			staging.Hashes[cleanName] = hash
		default:
			_ = staging.Cleanup()
			return nil, fmt.Errorf("tar entry %q has unsupported type %c (only regular files and directories are accepted)",
				hdr.Name, hdr.Typeflag)
		}
	}

	manifestPath := filepath.Join(dir, "manifest.yaml")
	manifest, err := ParseManifestFile(manifestPath)
	if err != nil {
		_ = staging.Cleanup()
		return nil, fmt.Errorf("manifest in tarball: %w", err)
	}
	staging.Manifest = manifest

	return staging, nil
}

// CrossCheckArtifacts verifies that every manifest artifact's declared
// sha256 matches the file actually extracted from the tarball. Called
// after ExtractTarball + signature verify but before any host-mutating
// step, so a "verified signature but tampered artifacts" tarball is
// rejected with the staging dir intact for the caller to quarantine.
//
// Returns a list of artifact paths that failed verification; the caller
// is responsible for cleaning up and quarantining when the list is
// non-empty.
func (s *Staging) CrossCheckArtifacts() ([]string, error) {
	if s.Manifest == nil {
		return nil, errors.New("staging has no manifest; call ExtractTarball first")
	}
	var mismatches []string
	for _, art := range s.Manifest.Artifacts {
		got, ok := s.Hashes[art.Source]
		if !ok {
			return nil, fmt.Errorf("manifest references artifact source %q but no such file was extracted", art.Source)
		}
		if !strings.EqualFold(got, art.SHA256) {
			mismatches = append(mismatches, art.Path)
		}
	}
	return mismatches, nil
}

// safeTarEntryPath cleans a tar entry name and ensures the result, when
// joined with stagingRoot, stays within stagingRoot. Rejects absolute
// paths, parent-traversal segments, and any name that would escape the
// staging directory via symlink-style tricks. Empty names and bare "."
// are accepted as the staging root itself.
func safeTarEntryPath(name, stagingRoot string) (string, error) {
	if name == "" || name == "." {
		return "", nil
	}
	clean := filepath.Clean(name)
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("tar entry %q is absolute", name)
	}
	if clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, `..\`) {
		return "", fmt.Errorf("tar entry %q escapes staging dir", name)
	}

	joined := filepath.Join(stagingRoot, clean)
	rel, err := filepath.Rel(stagingRoot, joined)
	if err != nil {
		return "", fmt.Errorf("compute rel for %q: %w", name, err)
	}
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("tar entry %q escapes staging dir", name)
	}
	return clean, nil
}

// writeEntry copies up to size bytes from tr into a new file at target
// while computing a sha256 of the bytes written. Returns the hex-encoded
// hash. The size cap is honoured even if the tar header lies (io.LimitReader
// stops early; the result is a hash of whatever was actually written and
// the caller's downstream cross-check will fail).
func writeEntry(tr io.Reader, target string, size int64) (string, error) {
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return "", err
	}
	defer out.Close()

	h := sha256.New()
	limited := io.LimitReader(tr, size)
	if _, err := io.Copy(io.MultiWriter(out, h), limited); err != nil {
		return "", err
	}
	if err := out.Sync(); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
