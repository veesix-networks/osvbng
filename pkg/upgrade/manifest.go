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

const (
	TierA                 = "A"
	ManifestSchemaVersion = 2
)

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

type ManifestHooks struct {
	Pre  HookEntry `yaml:"pre,omitempty"`
	Post HookEntry `yaml:"post,omitempty"`
}

type HookEntry struct {
	Path   string `yaml:"path,omitempty"`
	SHA256 string `yaml:"sha256,omitempty"`
}

// validRestartClasses is the load-bearing enum guard. KnownFields(true)
// catches unknown keys but not bad values; a typo here would silently
// under-restart.
var validRestartClasses = map[string]bool{
	"osvbngd": true,
	"vpp":     true,
	"both":    true,
	"none":    true,
}

var tierBForbiddenKeys = []string{
	"deb_packages",
	"dpkg_baseline",
}

func ParseManifestFile(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	return ParseManifest(data)
}

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

func (m *Manifest) Validate() error {
	if m.Type == "" {
		return fmt.Errorf("manifest: type field is required")
	}
	if m.Type != TierA {
		return fmt.Errorf("manifest: this build handles tier %q only; tarball declares tier %q", TierA, m.Type)
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

// checkForbiddenKeys runs before strict decode so a Tier-B manifest hits
// a tier-specific error message instead of generic "unknown field".
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
