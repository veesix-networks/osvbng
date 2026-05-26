// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/upgrade"
)

// replReporter is the osvbngcli-facing ProgressReporter the upgrade builtin
// wires into Runner. Formats events as the "[N/M] stage…" progress UI
// the operator sees while an upgrade runs.
type replReporter struct{}

func newReplReporter() upgrade.ProgressReporter { return replReporter{} }

// Stage prints a header for the major step, e.g. "[5/14] Snapshotting…".
func (replReporter) Stage(step, total int, name string) {
	fmt.Printf("[%d/%d] %s\n", step, total, name)
}

// Progress prints a short under-stage status line indented for
// readability. Lines longer than ~78 cols stay un-wrapped so the
// operator sees the full text.
func (replReporter) Progress(msg string) {
	fmt.Printf("        > %s\n", msg)
}

// Detail prints multi-line free-form text — typically hook stdout/stderr.
// Indents every line so the source is visually grouped with the active
// stage.
func (replReporter) Detail(msg string) {
	msg = strings.TrimRight(msg, "\n")
	if msg == "" {
		return
	}
	for _, line := range strings.Split(msg, "\n") {
		fmt.Printf("          %s\n", line)
	}
}

// Warn surfaces a non-fatal warning with a clear prefix.
func (replReporter) Warn(msg string) {
	fmt.Printf("        ! WARN: %s\n", msg)
}
