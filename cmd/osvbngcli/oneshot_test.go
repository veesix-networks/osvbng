// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// The -c one-shot path is NewCLI + a single processCommand with no readline
// loop; this drives exactly that against the shared qos fixture, so the
// compact rendering the robot suites assert on is pinned here too.
func TestOneShotSchedulerCompactRendering(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	spec := &openapi3.T{OpenAPI: "3.0.3", Paths: openapi3.NewPaths()}
	spec.Paths.Set("/api/show/qos/scheduler", &openapi3.PathItem{Get: &openapi3.Operation{}})
	specJSON, err := spec.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	fixture, err := os.ReadFile(filepath.Join(fixtureDir, "scheduler_api.json"))
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/openapi.json":
			w.Header().Set("ETag", `"oneshot-etag"`)
			_, _ = w.Write(specJSON)
		case "/api/show/qos/scheduler":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(fixture)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cli, err := NewCLI(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := cli.processCommand("show qos scheduler"); err != nil {
			t.Errorf("processCommand: %v", err)
		}
	})

	for _, token := range []string{"SW_IF", "SESSION", "TX PKTS/BYTES", "BLK D/P",
		"9f8be1a2-77c1-4a83-9e0f-0d5a2f6b3c11", "sv100@eth1"} {
		if !strings.Contains(out, token) {
			t.Errorf("compact output missing %q:\n%s", token, out)
		}
	}
	if strings.Contains(out, "{") {
		t.Errorf("compact output contains JSON:\n%s", out)
	}
	if strings.Contains(out, "Connected to") {
		t.Error("one-shot output contains the interactive banner")
	}

	// The json pipeline must bypass the compact renderer entirely.
	jsonOut := captureStdout(t, func() {
		if err := cli.processCommand("show qos scheduler | json"); err != nil {
			t.Errorf("processCommand json: %v", err)
		}
	})
	if !strings.Contains(jsonOut, `"session_id"`) || !strings.Contains(jsonOut, "{") {
		t.Errorf("| json did not emit raw JSON:\n%s", jsonOut)
	}

	// One-shot error semantics: an unknown command surfaces as an error
	// (main maps it to exit 1).
	if err := cli.processCommand("show qos nonsense"); err == nil {
		t.Error("expected an error for an unknown command")
	}
}
