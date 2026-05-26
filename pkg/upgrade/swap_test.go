// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestSwapArtifactReplacesAtomically(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "osvbngd")
	if err := os.WriteFile(target, []byte("OLD"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	oldInode := mustInode(t, target)

	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("NEW"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	if err := SwapArtifact(src, target, -1, -1, "0755"); err != nil {
		t.Fatalf("SwapArtifact: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "NEW" {
		t.Fatalf("target bytes = %q, want NEW", string(got))
	}
	if mustInode(t, target) == oldInode {
		t.Fatalf("target inode did not change (expected rename(2) to replace)")
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %o, want 0755", info.Mode().Perm())
	}
}

func TestSwapArtifactCleansStagingFileOnSourceMissing(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "osvbngd")
	if err := os.WriteFile(target, []byte("OLD"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	err := SwapArtifact(filepath.Join(dir, "missing"), target, -1, -1, "0755")
	if err == nil {
		t.Fatal("SwapArtifact accepted missing source")
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") && strings.HasSuffix(e.Name(), ".new") {
			t.Fatalf("leftover staging file after failed swap: %s", e.Name())
		}
	}

	// Target must still hold the original bytes.
	got, _ := os.ReadFile(target)
	if string(got) != "OLD" {
		t.Fatalf("target was disturbed by failed swap: got %q", string(got))
	}
}

func TestSwapArtifactInvalidMode(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "osvbngd")
	if err := os.WriteFile(target, []byte("OLD"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("NEW"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	err := SwapArtifact(src, target, -1, -1, "notamode")
	if err == nil {
		t.Fatal("SwapArtifact accepted invalid mode string")
	}
}

func TestSwapArtifactEmptyModeSkipsChmod(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "osvbngd")
	if err := os.WriteFile(target, []byte("OLD"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("NEW"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	if err := SwapArtifact(src, target, -1, -1, ""); err != nil {
		t.Fatalf("SwapArtifact: %v", err)
	}

	info, _ := os.Stat(target)
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %o, want default 0644 from temp file", info.Mode().Perm())
	}
}

func TestParseOctalModeVariants(t *testing.T) {
	cases := map[string]os.FileMode{
		"0755": 0o755,
		"755":  0o755,
		"0o755": 0o755,
		"0644": 0o644,
		"0o644": 0o644,
	}
	for input, want := range cases {
		got, err := parseOctalMode(input)
		if err != nil {
			t.Fatalf("parseOctalMode(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("parseOctalMode(%q) = %o, want %o", input, got, want)
		}
	}

	if _, err := parseOctalMode(""); err == nil {
		t.Fatal("parseOctalMode(\"\") = nil, want error")
	}
	if _, err := parseOctalMode("hello"); err == nil {
		t.Fatal("parseOctalMode(\"hello\") = nil, want error")
	}
}

func mustInode(t *testing.T, path string) uint64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	sys, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("stat sys is not *syscall.Stat_t")
	}
	return sys.Ino
}
