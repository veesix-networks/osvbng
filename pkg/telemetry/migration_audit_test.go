// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMigrationAudit enforces that the legacy state.RegisterMetric and
// Prom-side RegisterMetricSingle/Multi APIs, plus the osvbng:state: cache
// prefix, have no source references in the repo after the #59 migration.
// Source files only — *_test.go, vendor/, and .git/ are skipped.
func TestMigrationAudit(t *testing.T) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("locate repo root: %v", err)
	}

	forbidden := []struct {
		needle string
		why    string
	}{
		{"state.RegisterMetric", "legacy state.RegisterMetric retired in osvbng-context #59"},
		{"metrics.RegisterMetricSingle", "legacy Prom-side RegisterMetricSingle retired"},
		{"metrics.RegisterMetricMulti", "legacy Prom-side RegisterMetricMulti retired"},
		{"osvbng:state:", "legacy osvbng:state:* cache prefix retired"},
	}

	var failures []string
	err = filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		rel, _ := filepath.Rel(repoRoot, path)
		for _, f := range forbidden {
			if strings.Contains(text, f.needle) {
				failures = append(failures, rel+": contains "+f.needle+" ("+f.why+")")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if len(failures) > 0 {
		t.Errorf("migration audit failed (%d references):", len(failures))
		for _, f := range failures {
			t.Errorf("  %s", f)
		}
	}
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
