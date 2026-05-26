// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import "testing"

func TestNullReporterDoesNotPanic(t *testing.T) {
	var r ProgressReporter = NullReporter{}
	r.Stage(1, 9, "do thing")
	r.Progress("doing")
	r.Detail("trace line")
	r.Warn("something off")
}

func TestRecordingReporterCapturesEverything(t *testing.T) {
	var r RecordingReporter
	r.Stage(2, 5, "snapshot")
	r.Progress("collecting metadata")
	r.Detail("/usr/local/bin/osvbngd uid=0 gid=0 mode=0755")
	r.Warn("operator-modified template will be overwritten")

	if len(r.Events) != 4 {
		t.Fatalf("events len = %d, want 4", len(r.Events))
	}

	if !r.HasEvent("stage", "snapshot") {
		t.Fatalf("HasEvent missed stage event: %+v", r.Events)
	}
	if !r.HasEvent("progress", "collecting") {
		t.Fatalf("HasEvent missed progress: %+v", r.Events)
	}
	if !r.HasEvent("warn", "operator-modified") {
		t.Fatalf("HasEvent missed warn: %+v", r.Events)
	}
	if r.HasEvent("warn", "nope-not-here") {
		t.Fatalf("HasEvent returned true for absent substring")
	}
}

func TestRecordingReporterImplementsProgressReporter(t *testing.T) {
	var _ ProgressReporter = (*RecordingReporter)(nil)
}

func TestChainCoordinatorIsAnInterface(t *testing.T) {
	// Just ensure the interface compiles and ApplyOptions has the
	// expected zero-value behaviour (PruneKeepN is the zero PrunePolicy).
	var opts ApplyOptions
	if opts.PrunePolicy != PruneKeepN {
		t.Fatalf("zero ApplyOptions.PrunePolicy = %v, want PruneKeepN", opts.PrunePolicy)
	}
	if opts.ExpectedFrom != "" {
		t.Fatalf("zero ApplyOptions.ExpectedFrom should be empty, got %q", opts.ExpectedFrom)
	}
	if opts.KeepN != 0 {
		t.Fatalf("zero ApplyOptions.KeepN should be 0 (will default to 1 in Runner), got %d", opts.KeepN)
	}
}
