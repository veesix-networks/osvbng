// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"strings"
	"testing"
)

const validTierAManifest = `
osvbng_version: 0.13.1
min_compatible_version: 0.12.0
type: A
build_date: 2026-06-15T08:00:00Z
build_commit: abc1234
artifacts:
  - path: /usr/local/bin/osvbngd
    source: osvbngd
    sha256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
    mode: "0755"
hooks:
  pre: pre.sh
estimated_outage_seconds: 30
requires_reboot: false
`

func TestParseManifestHappyPath(t *testing.T) {
	m, err := ParseManifest([]byte(validTierAManifest))
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.Type != TierA {
		t.Fatalf("Type = %q, want %q", m.Type, TierA)
	}
	if m.OsvbngVersion != "0.13.1" {
		t.Fatalf("OsvbngVersion = %q, want 0.13.1", m.OsvbngVersion)
	}
	if len(m.Artifacts) != 1 {
		t.Fatalf("Artifacts len = %d, want 1", len(m.Artifacts))
	}
	if m.Hooks.Pre != "pre.sh" {
		t.Fatalf("Hooks.Pre = %q, want pre.sh", m.Hooks.Pre)
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
		`mode: "0755"`,
		`mode: "0755"
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

func TestParseManifestRefusesRequiresRebootTrue(t *testing.T) {
	m := strings.Replace(validTierAManifest,
		"requires_reboot: false",
		"requires_reboot: true", 1)
	_, err := ParseManifest([]byte(m))
	if err == nil {
		t.Fatal("ParseManifest accepted requires_reboot: true")
	}
	if !strings.Contains(err.Error(), "requires_reboot") {
		t.Fatalf("error did not name requires_reboot: %v", err)
	}
}

func TestParseManifestRefusesEmptyArtifacts(t *testing.T) {
	m := `
osvbng_version: 0.13.1
min_compatible_version: 0.12.0
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
osvbng_version: 0.13.1
min_compatible_version: 0.12.0
type: A
artifacts:
  - path: /usr/local/bin/osvbngd
    source: a
    sha256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
  - path: /usr/local/bin/osvbngd
    source: b
    sha256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
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
