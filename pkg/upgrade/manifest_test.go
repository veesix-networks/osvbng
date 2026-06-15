// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"strings"
	"testing"
)

const validTierAManifest = `
schema_version: 2
osvbng_version: 0.14.0
min_compatible_version: 0.13.1
type: A
build_date: 2026-06-15T08:00:00Z
build_commit: abc1234
artifacts:
  - path: /usr/local/bin/osvbngd
    source: bin/osvbngd
    sha256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
    mode: "0755"
    requires_restart: osvbngd
hooks:
  pre:
    path: pre.sh
    sha256: 1111111111111111111111111111111111111111111111111111111111111111
estimated_outage_seconds: 30
`

func TestParseManifestHappyPath(t *testing.T) {
	m, err := ParseManifest([]byte(validTierAManifest))
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.Type != TierA {
		t.Fatalf("Type = %q, want %q", m.Type, TierA)
	}
	if m.SchemaVersion != ManifestSchemaVersion {
		t.Fatalf("SchemaVersion = %d, want %d", m.SchemaVersion, ManifestSchemaVersion)
	}
	if m.OsvbngVersion != "0.14.0" {
		t.Fatalf("OsvbngVersion = %q, want 0.14.0", m.OsvbngVersion)
	}
	if len(m.Artifacts) != 1 {
		t.Fatalf("Artifacts len = %d, want 1", len(m.Artifacts))
	}
	if m.Artifacts[0].RequiresRestart != "osvbngd" {
		t.Fatalf("Artifacts[0].RequiresRestart = %q, want osvbngd", m.Artifacts[0].RequiresRestart)
	}
	if m.Hooks.Pre.Path != "pre.sh" {
		t.Fatalf("Hooks.Pre.Path = %q, want pre.sh", m.Hooks.Pre.Path)
	}
	if len(m.Hooks.Pre.SHA256) != 64 {
		t.Fatalf("Hooks.Pre.SHA256 = %q, want 64 hex chars", m.Hooks.Pre.SHA256)
	}
}

func TestParseManifestRefusesTierB(t *testing.T) {
	m := strings.Replace(validTierAManifest, "type: A", "type: B", 1)
	_, err := ParseManifest([]byte(m))
	if err == nil {
		t.Fatal("ParseManifest accepted type: B")
	}
	if !strings.Contains(err.Error(), "tier") || !strings.Contains(err.Error(), `"B"`) {
		t.Fatalf("error did not mention tier mismatch: %v", err)
	}
}

func TestParseManifestRefusesUnknownTopLevelField(t *testing.T) {
	m := validTierAManifest + "\nunknown_field: oops\n"
	_, err := ParseManifest([]byte(m))
	if err == nil {
		t.Fatal("ParseManifest accepted unknown top-level field")
	}
	if !strings.Contains(err.Error(), "unknown_field") {
		t.Fatalf("error did not name the unknown field: %v", err)
	}
}

func TestParseManifestRefusesUnknownArtifactField(t *testing.T) {
	m := strings.Replace(validTierAManifest,
		`requires_restart: osvbngd`,
		`requires_restart: osvbngd
    bogus_artifact_field: oops`, 1)
	_, err := ParseManifest([]byte(m))
	if err == nil {
		t.Fatal("ParseManifest accepted unknown artifact field")
	}
	if !strings.Contains(err.Error(), "bogus_artifact_field") {
		t.Fatalf("error did not name the unknown artifact field: %v", err)
	}
}

func TestParseManifestRefusesDebPackagesEvenWithTypeA(t *testing.T) {
	m := validTierAManifest + `
deb_packages:
  - libfoo3_3.2.1-1_amd64.deb
`
	_, err := ParseManifest([]byte(m))
	if err == nil {
		t.Fatal("ParseManifest accepted Tier-B field deb_packages")
	}
	if !strings.Contains(err.Error(), "Tier-B") || !strings.Contains(err.Error(), "deb_packages") {
		t.Fatalf("error did not flag deb_packages as Tier-B: %v", err)
	}
}

func TestParseManifestRefusesDpkgBaseline(t *testing.T) {
	m := validTierAManifest + `
dpkg_baseline: /tmp/baseline.list
`
	_, err := ParseManifest([]byte(m))
	if err == nil {
		t.Fatal("ParseManifest accepted Tier-B field dpkg_baseline")
	}
	if !strings.Contains(err.Error(), "dpkg_baseline") {
		t.Fatalf("error did not name dpkg_baseline: %v", err)
	}
}

func TestParseManifestRefusesEmptyArtifacts(t *testing.T) {
	m := `
schema_version: 2
osvbng_version: 0.14.0
min_compatible_version: 0.13.1
type: A
artifacts: []
`
	_, err := ParseManifest([]byte(m))
	if err == nil {
		t.Fatal("ParseManifest accepted empty artifacts")
	}
	if !strings.Contains(err.Error(), "artifacts list is empty") {
		t.Fatalf("error did not flag empty artifacts: %v", err)
	}
}

func TestParseManifestRefusesShortSHA256(t *testing.T) {
	m := strings.Replace(validTierAManifest,
		"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"deadbeef", 1)
	_, err := ParseManifest([]byte(m))
	if err == nil {
		t.Fatal("ParseManifest accepted truncated sha256")
	}
	if !strings.Contains(err.Error(), "sha256") {
		t.Fatalf("error did not flag bad sha256: %v", err)
	}
}

func TestParseManifestRefusesDuplicatePath(t *testing.T) {
	m := `
schema_version: 2
osvbng_version: 0.14.0
min_compatible_version: 0.13.1
type: A
artifacts:
  - path: /usr/local/bin/osvbngd
    source: a
    sha256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
    requires_restart: osvbngd
  - path: /usr/local/bin/osvbngd
    source: b
    sha256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
    requires_restart: osvbngd
`
	_, err := ParseManifest([]byte(m))
	if err == nil {
		t.Fatal("ParseManifest accepted duplicate artifact paths")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("error did not flag duplicate path: %v", err)
	}
}

func TestParseManifestMissingType(t *testing.T) {
	m := strings.Replace(validTierAManifest, "type: A", "", 1)
	_, err := ParseManifest([]byte(m))
	if err == nil {
		t.Fatal("ParseManifest accepted missing type")
	}
	if !strings.Contains(err.Error(), "type") {
		t.Fatalf("error did not flag missing type: %v", err)
	}
}

func TestParseManifestRefusesMissingSchemaVersion(t *testing.T) {
	m := strings.Replace(validTierAManifest, "schema_version: 2\n", "", 1)
	_, err := ParseManifest([]byte(m))
	if err == nil {
		t.Fatal("ParseManifest accepted manifest with no schema_version")
	}
	if !strings.Contains(err.Error(), "schema_version") {
		t.Fatalf("error did not name schema_version: %v", err)
	}
}

func TestParseManifestRefusesFutureSchemaVersion(t *testing.T) {
	m := strings.Replace(validTierAManifest, "schema_version: 2", "schema_version: 3", 1)
	_, err := ParseManifest([]byte(m))
	if err == nil {
		t.Fatal("ParseManifest accepted schema_version: 3")
	}
	if !strings.Contains(err.Error(), "schema_version=3") {
		t.Fatalf("error did not flag the future schema_version: %v", err)
	}
}

func TestParseManifestRefusesZeroSchemaVersion(t *testing.T) {
	m := strings.Replace(validTierAManifest, "schema_version: 2", "schema_version: 0", 1)
	_, err := ParseManifest([]byte(m))
	if err == nil {
		t.Fatal("ParseManifest accepted schema_version: 0")
	}
	if !strings.Contains(err.Error(), "schema_version") {
		t.Fatalf("error did not name schema_version on zero: %v", err)
	}
}

func TestParseManifestRefusesMalformedSchemaVersion(t *testing.T) {
	m := strings.Replace(validTierAManifest, "schema_version: 2", "schema_version: not-a-number", 1)
	_, err := ParseManifest([]byte(m))
	if err == nil {
		t.Fatal("ParseManifest accepted non-integer schema_version")
	}
	if !strings.Contains(err.Error(), "unmarshal") && !strings.Contains(err.Error(), "int") {
		t.Fatalf("error did not flag type mismatch on schema_version: %v", err)
	}
}

func TestParseManifestRequiresRestartEnum(t *testing.T) {
	t.Run("osvbngd", func(t *testing.T) {
		m, err := ParseManifest([]byte(validTierAManifest))
		if err != nil {
			t.Fatalf("ParseManifest osvbngd: %v", err)
		}
		if m.Artifacts[0].RequiresRestart != "osvbngd" {
			t.Fatalf("RequiresRestart = %q, want osvbngd", m.Artifacts[0].RequiresRestart)
		}
	})
	for _, valid := range []string{"vpp", "both", "none"} {
		valid := valid
		t.Run(valid, func(t *testing.T) {
			m := strings.Replace(validTierAManifest, "requires_restart: osvbngd", "requires_restart: "+valid, 1)
			parsed, err := ParseManifest([]byte(m))
			if err != nil {
				t.Fatalf("ParseManifest %s: %v", valid, err)
			}
			if parsed.Artifacts[0].RequiresRestart != valid {
				t.Fatalf("RequiresRestart = %q, want %q", parsed.Artifacts[0].RequiresRestart, valid)
			}
		})
	}
	t.Run("typo", func(t *testing.T) {
		m := strings.Replace(validTierAManifest, "requires_restart: osvbngd", "requires_restart: vp", 1)
		_, err := ParseManifest([]byte(m))
		if err == nil {
			t.Fatal("ParseManifest accepted requires_restart: vp (typo)")
		}
		if !strings.Contains(err.Error(), "requires_restart") || !strings.Contains(err.Error(), `"vp"`) {
			t.Fatalf("error did not flag the typo: %v", err)
		}
	})
	t.Run("empty", func(t *testing.T) {
		m := strings.Replace(validTierAManifest, "    requires_restart: osvbngd\n", "", 1)
		_, err := ParseManifest([]byte(m))
		if err == nil {
			t.Fatal("ParseManifest accepted artifact with no requires_restart")
		}
		if !strings.Contains(err.Error(), "requires_restart") || !strings.Contains(err.Error(), "empty") {
			t.Fatalf("error did not flag empty requires_restart: %v", err)
		}
	})
}

func TestParseManifestV2HookEntryShape(t *testing.T) {
	m, err := ParseManifest([]byte(validTierAManifest))
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.Hooks.Pre.Path != "pre.sh" || len(m.Hooks.Pre.SHA256) != 64 {
		t.Fatalf("Hooks.Pre = %+v, want {Path: pre.sh, SHA256: 64-hex}", m.Hooks.Pre)
	}
	if m.Hooks.Post.Path != "" {
		t.Fatalf("Hooks.Post.Path = %q, want empty (hook absent in fixture)", m.Hooks.Post.Path)
	}
}

func TestParseManifestRefusesV1HookStringShorthand(t *testing.T) {
	m := strings.Replace(validTierAManifest,
		`hooks:
  pre:
    path: pre.sh
    sha256: 1111111111111111111111111111111111111111111111111111111111111111`,
		`hooks:
  pre: pre.sh`, 1)
	_, err := ParseManifest([]byte(m))
	if err == nil {
		t.Fatal("ParseManifest accepted v1 string-shorthand hook on v2 schema")
	}
}
