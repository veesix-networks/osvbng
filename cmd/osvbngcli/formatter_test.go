// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"strings"
	"testing"

	"github.com/veesix-networks/osvbng/cmd/osvbngcli/orderedjson"
)

func render(t *testing.T, raw string, format OutputFormat) string {
	t.Helper()
	data, err := orderedjson.Decode([]byte(raw))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	out, err := NewGenericFormatter().Format(data, format)
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	return out
}

// The table header must follow the server's field order, and the same input
// must render identically on every run. Re-decoding inside the loop reproduces
// what separate CLI invocations do; before the fix the columns reshuffled.
func TestFormatTableColumnsStableAndOrdered(t *testing.T) {
	raw := `[{"username":"a@x","ip":"10.0.0.1","vlan":100},
	         {"username":"b@x","ip":"10.0.0.2","vlan":200}]`

	first := render(t, raw, FormatCLI)
	header := strings.SplitN(first, "\n", 2)[0]

	iU, iIP, iV := strings.Index(header, "username"), strings.Index(header, "ip"), strings.Index(header, "vlan")
	if !(iU >= 0 && iU < iIP && iIP < iV) {
		t.Fatalf("header not in field order: %q", header)
	}

	for i := 0; i < 200; i++ {
		if got := render(t, raw, FormatCLI); got != first {
			t.Fatalf("iteration %d output differs:\n--- first ---\n%s\n--- got ---\n%s", i, first, got)
		}
	}
}

// A row missing a column renders "-" rather than shifting later columns.
func TestFormatTableMissingCell(t *testing.T) {
	out := render(t, `[{"a":"1","b":"2"},{"a":"3"}]`, FormatCLI)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 { // header, separator, 2 rows
		t.Fatalf("want 4 lines, got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[3], "-") {
		t.Fatalf("missing cell should render '-': %q", lines[3])
	}
}

// Single-object output renders as a tree in key order, not sorted.
func TestFormatTreeKeyOrder(t *testing.T) {
	out := render(t, `{"z":"1","a":"2","m":"3"}`, FormatCLI)
	iZ, iA, iM := strings.Index(out, "z:"), strings.Index(out, "a:"), strings.Index(out, "m:")
	if !(iZ >= 0 && iZ < iA && iA < iM) {
		t.Fatalf("tree not in key order:\n%s", out)
	}
}

func TestFormatJSONPreservesOrder(t *testing.T) {
	out := render(t, `{"z":1,"a":2,"m":3}`, FormatJSON)
	iZ, iA, iM := strings.Index(out, `"z"`), strings.Index(out, `"a"`), strings.Index(out, `"m"`)
	if !(iZ >= 0 && iZ < iA && iA < iM) {
		t.Fatalf("json not in key order:\n%s", out)
	}
}

func TestFormatYAMLPreservesOrder(t *testing.T) {
	out := render(t, `{"z":1,"a":2,"m":3}`, FormatYAML)
	iZ, iA, iM := strings.Index(out, "z:"), strings.Index(out, "a:"), strings.Index(out, "m:")
	if !(iZ >= 0 && iZ < iA && iA < iM) {
		t.Fatalf("yaml not in key order:\n%s", out)
	}
}
