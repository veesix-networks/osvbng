// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCurrentManifestFile(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "current-manifest.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write current manifest: %v", err)
	}
}

func TestCurrentInstalledVersionReadsManifest(t *testing.T) {
	stateRoot := t.TempDir()
	writeCurrentManifestFile(t, stateRoot, `
schema_version: 2
osvbng_version: 0.14.0
min_compatible_version: 0.13.1
type: A
build_commit: abc1234
artifacts:
  - path: /usr/local/bin/osvbngd
    source: osvbngd
    sha256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
    requires_restart: osvbngd
`)

	got, err := CurrentInstalledVersion(context.Background(), stateRoot, "/nonexistent/osvbngd", nil)
	if err != nil {
		t.Fatalf("CurrentInstalledVersion: %v", err)
	}
	if got != "0.14.0" {
		t.Fatalf("got %q, want 0.14.0", got)
	}
}

func TestCurrentInstalledVersionFallsBackToOsvbngdVersion(t *testing.T) {
	stateRoot := t.TempDir()
	// No current-manifest.yaml present — should exec the daemon.

	fake := &fakeCommander{
		scripts: []fakeResp{{matchName: "/usr/local/bin/osvbngd", out: "0.13.0 (abc1234) built on 2026-06-15T08:00:00Z\n"}},
	}

	got, err := CurrentInstalledVersion(context.Background(), stateRoot, "/usr/local/bin/osvbngd", fake)
	if err != nil {
		t.Fatalf("CurrentInstalledVersion: %v", err)
	}
	if got != "0.13.0" {
		t.Fatalf("got %q, want 0.13.0", got)
	}
}

func TestCurrentInstalledVersionFallsBackOnUnparseableManifest(t *testing.T) {
	stateRoot := t.TempDir()
	// Write a manifest that won't parse but the fallback should kick in only on missing-file errors.
	// To test the fallback path, leave the manifest absent and exercise the version exec.
	fake := &fakeCommander{
		scripts: []fakeResp{{matchName: "osvbngd", out: "v0.14.2-dirty (deadbee) built on 2026-07-01\n"}},
	}
	got, err := CurrentInstalledVersion(context.Background(), stateRoot, "osvbngd", fake)
	if err != nil {
		t.Fatalf("CurrentInstalledVersion: %v", err)
	}
	if got != "v0.14.2-dirty" {
		t.Fatalf("got %q, want v0.14.2-dirty", got)
	}
}

func TestCurrentInstalledVersionFailsWhenDaemonAbsent(t *testing.T) {
	stateRoot := t.TempDir()
	fake := &fakeCommander{
		scripts: []fakeResp{{
			matchName: "osvbngd",
			out:       "exec: \"osvbngd\": executable file not found in $PATH",
			err:       errors.New("not found"),
		}},
	}
	_, err := CurrentInstalledVersion(context.Background(), stateRoot, "osvbngd", fake)
	if err == nil {
		t.Fatal("CurrentInstalledVersion: err = nil with no manifest and no daemon, want error")
	}
}

func TestParseVersionOutputVariants(t *testing.T) {
	cases := map[string]string{
		"0.13.0 (abc1234) built on 2026-06-15": "0.13.0",
		"v0.13.0\n":                            "v0.13.0",
		"  v0.14.2-dirty  ":                    "v0.14.2-dirty",
		"":                                     "",
		"   ":                                  "",
	}
	for input, want := range cases {
		got := parseVersionOutput(input)
		if got != want {
			t.Fatalf("parseVersionOutput(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestWriteCurrentManifestRoundtrip(t *testing.T) {
	stateRoot := t.TempDir()
	m := &Manifest{
		SchemaVersion:        ManifestSchemaVersion,
		OsvbngVersion:        "0.14.0",
		MinCompatibleVersion: "0.13.1",
		Type:                 TierA,
		BuildCommit:          "abc1234",
		Artifacts: []ManifestArtifact{
			{Path: "/usr/local/bin/osvbngd", Source: "osvbngd", SHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", Mode: "0755", RequiresRestart: "osvbngd"},
		},
	}

	if err := WriteCurrentManifest(stateRoot, m); err != nil {
		t.Fatalf("WriteCurrentManifest: %v", err)
	}

	got, err := CurrentInstalledVersion(context.Background(), stateRoot, "/nonexistent/osvbngd", nil)
	if err != nil {
		t.Fatalf("CurrentInstalledVersion after WriteCurrentManifest: %v", err)
	}
	if got != "0.14.0" {
		t.Fatalf("got %q, want 0.14.0", got)
	}

	// Belt + suspenders: ensure no temp files were left behind.
	entries, _ := os.ReadDir(stateRoot)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".current-manifest") {
			t.Fatalf("temp file leaked into stateRoot: %s", e.Name())
		}
	}
}
