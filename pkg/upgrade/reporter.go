// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

// ProgressReporter is the structured event surface the Runner uses to
// stream progress, free-form detail, and warnings out to the operator.
// The Runner never touches stdout directly; the osvbngcli builtin (in
// cmd/osvbngcli/) supplies a ProgressReporter impl that formats events
// into the operator-facing "[N/M] stage" UI.
//
// Stage marks a major step boundary with an index/total pair so the
// osvbngcli builtin can render a progress line. Progress is a short status line
// nested under the current stage. Detail is a free-form line that
// usually comes from subprocess stdout/stderr passthrough (pre.sh /
// post.sh). Warn is operator-visible non-fatal.
//
// All methods must be safe to call from any goroutine; an implementation
// that writes to stdout should serialize internally if needed.
type ProgressReporter interface {
	Stage(step int, total int, name string)
	Progress(msg string)
	Detail(msg string)
	Warn(msg string)
}

// NullReporter discards every event. Use in tests where progress
// rendering is not under test, or in code paths where a non-nil
// Reporter is required but no surface is available (e.g. Runner.Status
// reading state for non-interactive callers).
type NullReporter struct{}

// Stage is a no-op.
func (NullReporter) Stage(int, int, string) {}

// Progress is a no-op.
func (NullReporter) Progress(string) {}

// Detail is a no-op.
func (NullReporter) Detail(string) {}

// Warn is a no-op.
func (NullReporter) Warn(string) {}

// RecordingReporter captures every event as a typed list. Test helper —
// not used in production but exposed in case integration tests in
// downstream packages want to assert on the event stream without
// rebuilding their own recorder.
type RecordingReporter struct {
	Events []RecordedEvent
}

// RecordedEvent is one observation by RecordingReporter, tagged by kind.
type RecordedEvent struct {
	Kind  string // "stage" | "progress" | "detail" | "warn"
	Step  int
	Total int
	Name  string
}

// Stage records a stage event.
func (r *RecordingReporter) Stage(step, total int, name string) {
	r.Events = append(r.Events, RecordedEvent{Kind: "stage", Step: step, Total: total, Name: name})
}

// Progress records a progress event.
func (r *RecordingReporter) Progress(msg string) {
	r.Events = append(r.Events, RecordedEvent{Kind: "progress", Name: msg})
}

// Detail records a detail event.
func (r *RecordingReporter) Detail(msg string) {
	r.Events = append(r.Events, RecordedEvent{Kind: "detail", Name: msg})
}

// Warn records a warning event.
func (r *RecordingReporter) Warn(msg string) {
	r.Events = append(r.Events, RecordedEvent{Kind: "warn", Name: msg})
}

// HasEvent returns true if any recorded event has the given kind and
// substring match on Name. Convenience for tests.
func (r *RecordingReporter) HasEvent(kind, substring string) bool {
	for _, e := range r.Events {
		if e.Kind == kind && containsSub(e.Name, substring) {
			return true
		}
	}
	return false
}

func containsSub(s, sub string) bool {
	if sub == "" {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
