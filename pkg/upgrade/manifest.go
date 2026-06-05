// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// TierA is the only tier handled by this implementation. Tarballs declaring
// any other tier value are refused early in the apply flow.
const TierA = "A"

// ManifestSchemaVersion is the manifest grammar this build understands.
// A manifest declaring a higher value is refused; a manifest declaring a
// lower value is refused.
const ManifestSchemaVersion = 2

// Manifest describes the contents of a signed osvbng release tarball.
// Tier-B fields (deb_packages, dpkg_baseline) are listed in
// tierBForbiddenKeys and cause strict decoding to fail with a clear
// error directing the operator to the Tier-B handler (separate spec).
type Manifest struct {
	SchemaVersion          int                `yaml:"schema_version"`
	OsvbngVersion          string             `yaml:"osvbng_version"`
	MinCompatibleVersion   string             `yaml:"min_compatible_version"`
	PreviousVersion        string             `yaml:"previous_version,omitempty"`
	PreviousManifestSHA256 string             `yaml:"previous_manifest_sha256,omitempty"`
	Type                   string             `yaml:"type"`
	BuildDate              time.Time          `yaml:"build_date"`
	BuildCommit            string             `yaml:"build_commit"`
	Artifacts              []ManifestArtifact `yaml:"artifacts"`
	Hooks                  ManifestHooks      `yaml:"hooks,omitempty"`
	EstimatedOutageSec     int                `yaml:"estimated_outage_seconds,omitempty"`
}

// ManifestArtifact describes a single file the tarball will install.
// RequiresRestart is mandatory and drives the aggregate restart plan
// (see RestartPlan / planFromManifest). expected_current_sha256 is
// optional drift-detection: when set, the runner warns if the on-disk
// file no longer matches the expected pre-upgrade hash (the apply
// continues anyway — the rollback snapshot preserves the drifted bytes
// for forensics).
type ManifestArtifact struct {
	Path                  string `yaml:"path"`
	Source                string `yaml:"source"`
	SHA256                string `yaml:"sha256"`
	ExpectedCurrentSHA256 string `yaml:"expected_current_sha256,omitempty"`
	Mode                  string `yaml:"mode,omitempty"`
	UID                   int    `yaml:"uid,omitempty"`
	GID                   int    `yaml:"gid,omitempty"`
	RequiresRestart       string `yaml:"requires_restart"`
}

// ManifestHooks names optional pre/post scripts inside the tarball.
// Empty path disables that hook.
type ManifestHooks struct {
	Pre  HookEntry `yaml:"pre,omitempty"`
	Post HookEntry `yaml:"post,omitempty"`
}

// HookEntry is a hook script reference: relative path + expected sha256.
// The runner cross-checks the on-disk hash against SHA256 immediately
// before exec; mismatch is a hard error.
type HookEntry struct {
	Path   string `yaml:"path,omitempty"`
	SHA256 string `yaml:"sha256,omitempty"`
}

// validRestartClasses is the closed enum permitted for
// artifacts[].requires_restart. KnownFields(true) at decode time catches
// unknown YAML KEYS, not bad enum VALUES — Validate is the load-bearing
// guard against a typo silently under-restarting the system.
var validRestartClasses = map[string]bool{
	"osvbngd": true,
	"vpp":     true,
	"both":    true,
	"none":    true,
}

// tierBForbiddenKeys are top-level manifest keys reserved for the Tier-B
// (apt-bundle) spec. Tier A refuses any manifest containing them, even
// if `type: A` is declared, so a build-pipeline misconfiguration can't
// silently produce a Tier-A tarball that ships Tier-B intent.
var tierBForbiddenKeys = []string{
	"deb_packages",
	"dpkg_baseline",
}

// ParseManifestFile reads and decodes a manifest from disk.
func ParseManifestFile(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	return ParseManifest(data)
}

// ParseManifest decodes manifest YAML with strict field handling and the
// Tier-B forbidden-fields guard, then runs structural validation.
//
// Strict decoding (KnownFields(true)) means unknown top-level or
// artifact-level fields are hard errors: a typo'd field name fails fast
// rather than silently being ignored.
func ParseManifest(data []byte) (*Manifest, error) {
	if err := checkForbiddenKeys(data); err != nil {
		return nil, err
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}

	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Validate performs structural and tier-policy checks on a decoded
// manifest. Called automatically by ParseManifest; exposed so tests can
// exercise it directly.
func (m *Manifest) Validate() error {
	if m.Type == "" {
		return fmt.Errorf("manifest: type field is required")
	}
	if m.Type != TierA {
		return fmt.Errorf("manifest: this build handles tier %q only; tarball declares tier %q (apt-bundle path is handled by a separate, later release)", TierA, m.Type)
	}

	if m.SchemaVersion != ManifestSchemaVersion {
		return fmt.Errorf("manifest: schema_version=%d is not supported (this build accepts schema_version=%d only)", m.SchemaVersion, ManifestSchemaVersion)
	}

	if m.OsvbngVersion == "" {
		return fmt.Errorf("manifest: osvbng_version is required")
	}
	if m.MinCompatibleVersion == "" {
		return fmt.Errorf("manifest: min_compatible_version is required")
	}

	if len(m.Artifacts) == 0 {
		return fmt.Errorf("manifest: artifacts list is empty")
	}
	seen := make(map[string]bool, len(m.Artifacts))
	for i, art := range m.Artifacts {
		if art.Path == "" {
			return fmt.Errorf("manifest: artifact[%d] path is empty", i)
		}
		if art.Source == "" {
			return fmt.Errorf("manifest: artifact[%d] (%s) source is empty", i, art.Path)
		}
		if art.SHA256 == "" {
			return fmt.Errorf("manifest: artifact[%d] (%s) sha256 is empty", i, art.Path)
		}
		if len(art.SHA256) != 64 {
			return fmt.Errorf("manifest: artifact[%d] (%s) sha256 is not 64 hex chars: %q", i, art.Path, art.SHA256)
		}
		if seen[art.Path] {
			return fmt.Errorf("manifest: artifact[%d] (%s) is a duplicate path", i, art.Path)
		}
		seen[art.Path] = true
		if art.RequiresRestart == "" {
			return fmt.Errorf("manifest: artifact[%d] (%s) requires_restart is empty (must be one of: osvbngd, vpp, both, none)", i, art.Path)
		}
		if !validRestartClasses[art.RequiresRestart] {
			return fmt.Errorf("manifest: artifact[%d] (%s) requires_restart=%q is not a valid value (must be one of: osvbngd, vpp, both, none)", i, art.Path, art.RequiresRestart)
		}
	}

	return nil
}

// checkForbiddenKeys scans the raw YAML for reserved Tier-B field names
// before strict decoding kicks in. Without this, strict decoding's
// "unknown field" error would surface for a Tier-B manifest fed to a
// Tier-A build, but the message would be generic. The dedicated check
// produces a clear "this is a Tier-B field" message that points the
// operator at the right follow-up.
func checkForbiddenKeys(data []byte) error {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("decode manifest (forbidden-keys check): %w", err)
	}
	var present []string
	for _, key := range tierBForbiddenKeys {
		if _, ok := raw[key]; ok {
			present = append(present, key)
		}
	}
	if len(present) > 0 {
		return fmt.Errorf("manifest: contains Tier-B-only fields %s — this Tier-A build cannot apply tarballs that declare apt-bundle semantics",
			strings.Join(present, ", "))
	}
	return nil
}
