// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type tarEntry struct {
	name     string
	body     []byte
	mode     int64
	typeflag byte
}

func writeTarball(t *testing.T, dir, name string, entries []tarEntry) string {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typeflag := e.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		hdr := &tar.Header{
			Name:     e.name,
			Mode:     e.mode,
			Size:     int64(len(e.body)),
			Typeflag: typeflag,
		}
		if hdr.Mode == 0 {
			hdr.Mode = 0o644
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader %s: %v", e.name, err)
		}
		if _, err := tw.Write(e.body); err != nil {
			t.Fatalf("Write body %s: %v", e.name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tw.Close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gz.Close: %v", err)
	}
	return path
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func goodManifest(artifactBody []byte) []byte {
	return []byte(`
schema_version: 2
osvbng_version: 0.14.0
min_compatible_version: 0.13.1
type: A
build_commit: abc1234
artifacts:
  - path: /usr/local/bin/osvbngd
    source: osvbngd
    sha256: ` + sha256Hex(artifactBody) + `
    mode: "0755"
    requires_restart: osvbngd
`)
}

func TestExtractTarballHappyPathStagingIsSiblingOfTarball(t *testing.T) {
	tarDir := t.TempDir()
	body := []byte("fake-osvbngd-binary")
	tarballPath := writeTarball(t, tarDir, "osvbng-v0.13.1.tar.gz", []tarEntry{
		{name: "manifest.yaml", body: goodManifest(body)},
		{name: "osvbngd", body: body, mode: 0o755},
	})

	staging, err := ExtractTarball(tarballPath)
	if err != nil {
		t.Fatalf("ExtractTarball: %v", err)
	}
	defer staging.Cleanup()

	if filepath.Dir(staging.Dir) != tarDir {
		t.Fatalf("staging dir %s is not under tarball parent %s", staging.Dir, tarDir)
	}
	if staging.Manifest == nil {
		t.Fatal("staging.Manifest is nil")
	}
	if staging.Hashes["osvbngd"] != sha256Hex(body) {
		t.Fatalf("artifact hash mismatch: got %s want %s",
			staging.Hashes["osvbngd"], sha256Hex(body))
	}

	mismatches, err := staging.CrossCheckArtifacts()
	if err != nil {
		t.Fatalf("CrossCheckArtifacts: %v", err)
	}
	if len(mismatches) != 0 {
		t.Fatalf("expected zero mismatches, got %v", mismatches)
	}
}

func TestExtractTarballCleanupRemovesStagingDir(t *testing.T) {
	tarDir := t.TempDir()
	body := []byte("x")
	tarballPath := writeTarball(t, tarDir, "u.tar.gz", []tarEntry{
		{name: "manifest.yaml", body: goodManifest(body)},
		{name: "osvbngd", body: body},
	})

	staging, err := ExtractTarball(tarballPath)
	if err != nil {
		t.Fatalf("ExtractTarball: %v", err)
	}
	if _, err := os.Stat(staging.Dir); err != nil {
		t.Fatalf("staging dir does not exist after extract: %v", err)
	}
	if err := staging.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if _, err := os.Stat(staging.Dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging dir still present after Cleanup: %v", err)
	}
}

func TestExtractTarballRefusesPathTraversal(t *testing.T) {
	tarDir := t.TempDir()
	tarballPath := writeTarball(t, tarDir, "evil.tar.gz", []tarEntry{
		{name: "../escape", body: []byte("nope")},
	})

	_, err := ExtractTarball(tarballPath)
	if err == nil {
		t.Fatal("ExtractTarball accepted ../escape entry")
	}
	if !strings.Contains(err.Error(), "escape") {
		t.Fatalf("error did not mention escape: %v", err)
	}
}

func TestExtractTarballRefusesAbsolutePath(t *testing.T) {
	tarDir := t.TempDir()
	tarballPath := writeTarball(t, tarDir, "abs.tar.gz", []tarEntry{
		{name: "/etc/passwd", body: []byte("nope")},
	})

	_, err := ExtractTarball(tarballPath)
	if err == nil {
		t.Fatal("ExtractTarball accepted absolute-path entry")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("error did not mention absolute: %v", err)
	}
}

func TestExtractTarballRefusesSymlinkEntries(t *testing.T) {
	tarDir := t.TempDir()
	path := filepath.Join(tarDir, "symlink.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	hdr := &tar.Header{
		Name:     "evil-link",
		Linkname: "/etc/passwd",
		Typeflag: tar.TypeSymlink,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}
	tw.Close()
	gz.Close()

	_, err = ExtractTarball(path)
	if err == nil {
		t.Fatal("ExtractTarball accepted symlink entry")
	}
	if !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("error did not flag symlink as unsupported: %v", err)
	}
}

func TestExtractTarballRefusesOversizeEntry(t *testing.T) {
	tarDir := t.TempDir()
	path := filepath.Join(tarDir, "big.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	hdr := &tar.Header{
		Name:     "huge",
		Size:     MaxArtifactBytes + 1,
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}
	if _, err := tw.Write(bytes.Repeat([]byte("x"), 1024)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	tw.Close()
	gz.Close()

	_, err = ExtractTarball(path)
	if err == nil {
		t.Fatal("ExtractTarball accepted oversize entry")
	}
	if !strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("error did not mention size limit: %v", err)
	}
}

func TestCrossCheckArtifactsDetectsTamperedBytes(t *testing.T) {
	tarDir := t.TempDir()
	declared := []byte("original-bytes")
	tampered := []byte("TAMPERED-bytes")
	tarballPath := writeTarball(t, tarDir, "u.tar.gz", []tarEntry{
		{name: "manifest.yaml", body: goodManifest(declared)},
		{name: "osvbngd", body: tampered},
	})

	staging, err := ExtractTarball(tarballPath)
	if err != nil {
		t.Fatalf("ExtractTarball: %v", err)
	}
	defer staging.Cleanup()

	mismatches, err := staging.CrossCheckArtifacts()
	if err != nil {
		t.Fatalf("CrossCheckArtifacts: %v", err)
	}
	if len(mismatches) != 1 || mismatches[0] != "/usr/local/bin/osvbngd" {
		t.Fatalf("expected mismatch for /usr/local/bin/osvbngd, got %v", mismatches)
	}
}

func TestExtractTarballRefusesUnsupportedTarType(t *testing.T) {
	tarDir := t.TempDir()
	path := filepath.Join(tarDir, "fifo.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	hdr := &tar.Header{
		Name:     "named-pipe",
		Typeflag: tar.TypeFifo,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}
	tw.Close()
	gz.Close()

	_, err = ExtractTarball(path)
	if err == nil {
		t.Fatal("ExtractTarball accepted FIFO entry")
	}
}

func TestQuarantineCopiesTarballAndWritesReason(t *testing.T) {
	tarDir := t.TempDir()
	tarballPath := filepath.Join(tarDir, "bad.tar.gz")
	body := []byte("contents-here")
	if err := os.WriteFile(tarballPath, body, 0o644); err != nil {
		t.Fatalf("write tarball: %v", err)
	}

	qroot := t.TempDir()
	target, err := Quarantine(qroot, tarballPath, "test-rejection-reason")
	if err != nil {
		t.Fatalf("Quarantine: %v", err)
	}

	if filepath.Dir(target) != qroot {
		t.Fatalf("quarantine target %s not under root %s", target, qroot)
	}

	copied := filepath.Join(target, "bad.tar.gz")
	got, err := os.ReadFile(copied)
	if err != nil {
		t.Fatalf("read copied tarball: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("copied tarball bytes differ from original")
	}

	reason, err := os.ReadFile(copied + ".reason")
	if err != nil {
		t.Fatalf("read reason sidecar: %v", err)
	}
	if !bytes.Contains(reason, []byte("test-rejection-reason")) {
		t.Fatalf("reason sidecar missing reason text: %q", string(reason))
	}
	if !bytes.Contains(reason, []byte(sha256Hex(body))) {
		t.Fatalf("reason sidecar missing sha256: %q", string(reason))
	}
}

func TestStagingCleanupIdempotent(t *testing.T) {
	s := &Staging{Dir: ""}
	if err := s.Cleanup(); err != nil {
		t.Fatalf("Cleanup on empty Staging: %v", err)
	}

	dir := t.TempDir()
	subdir := filepath.Join(dir, "stage")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	s = &Staging{Dir: subdir}
	if err := s.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if err := s.Cleanup(); err != nil {
		t.Fatalf("second Cleanup: %v", err)
	}
}

