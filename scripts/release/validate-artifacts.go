// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

//go:build ignore

package main

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type artifactsFile struct {
	SchemaVersion int             `yaml:"schema_version"`
	Artifacts     []artifactEntry `yaml:"artifacts"`
}

type artifactEntry struct {
	Source          string `yaml:"source"`
	InstallPath     string `yaml:"install_path"`
	Mode            string `yaml:"mode"`
	UID             int    `yaml:"uid"`
	GID             int    `yaml:"gid"`
	RequiresRestart string `yaml:"requires_restart"`
}

var validRestart = map[string]bool{"osvbngd": true, "vpp": true, "both": true, "none": true}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: validate-artifacts <path-to-artifacts.yaml>")
		os.Exit(1)
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", os.Args[1], err)
		os.Exit(1)
	}
	var af artifactsFile
	if err := yaml.Unmarshal(raw, &af); err != nil {
		fmt.Fprintf(os.Stderr, "decode %s: %v\n", os.Args[1], err)
		os.Exit(1)
	}

	failed := 0
	installPaths := make(map[string]int, len(af.Artifacts))
	for i, a := range af.Artifacts {
		if a.Source == "" {
			fmt.Fprintf(os.Stderr, "[%d] source is empty\n", i)
			failed++
			continue
		}
		if a.InstallPath == "" {
			fmt.Fprintf(os.Stderr, "[%d] %s: install_path is empty\n", i, a.Source)
			failed++
		}
		if prev, dup := installPaths[a.InstallPath]; dup {
			fmt.Fprintf(os.Stderr, "[%d] %s: install_path %q duplicates [%d]\n", i, a.Source, a.InstallPath, prev)
			failed++
		}
		installPaths[a.InstallPath] = i

		if _, err := os.Stat(a.Source); err != nil {
			fmt.Fprintf(os.Stderr, "[%d] %s: source missing on disk: %v\n", i, a.Source, err)
			failed++
		}

		if !validRestart[a.RequiresRestart] {
			fmt.Fprintf(os.Stderr, "[%d] %s: requires_restart=%q is not a valid value (must be one of osvbngd / vpp / both / none)\n", i, a.Source, a.RequiresRestart)
			failed++
		}

		if a.Mode == "" {
			fmt.Fprintf(os.Stderr, "[%d] %s: mode is empty\n", i, a.Source)
			failed++
		} else if _, err := strconv.ParseUint(a.Mode, 8, 32); err != nil {
			fmt.Fprintf(os.Stderr, "[%d] %s: mode %q is not a valid octal string: %v\n", i, a.Source, a.Mode, err)
			failed++
		}

		if a.Source == "templates/dataplane.conf.tmpl" && a.RequiresRestart != "vpp" {
			fmt.Fprintf(os.Stderr, "[%d] %s: templates/dataplane.conf.tmpl MUST be requires_restart: vpp, got %q\n", i, a.Source, a.RequiresRestart)
			failed++
		}
	}

	if failed > 0 {
		fmt.Fprintf(os.Stderr, "\n%d validation finding(s)\n", failed)
		os.Exit(1)
	}
	fmt.Printf("release/artifacts.yaml OK: %d entries, schema_version=%d\n", len(af.Artifacts), af.SchemaVersion)
}
