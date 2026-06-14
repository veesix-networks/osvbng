// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestNormalizeServerURLUnixScheme(t *testing.T) {
	u, err := normalizeServerURL("unix:///run/osvbng/api.sock")
	if err != nil {
		t.Fatalf("normalize unix: %v", err)
	}
	if u.Scheme != "unix" {
		t.Errorf("scheme = %q, want unix", u.Scheme)
	}
	if u.Path != "/run/osvbng/api.sock" {
		t.Errorf("path = %q, want /run/osvbng/api.sock", u.Path)
	}
}

func TestNormalizeServerURLUnixRejectsEmptyPath(t *testing.T) {
	if _, err := normalizeServerURL("unix://"); err == nil {
		t.Fatal("expected error for unix URL with no path")
	}
}

func TestNormalizeServerURLTCPUnchanged(t *testing.T) {
	u, err := normalizeServerURL("http://localhost:8080")
	if err != nil {
		t.Fatalf("normalize http: %v", err)
	}
	if u.Scheme != "http" || u.Host != "localhost:8080" {
		t.Errorf("got scheme=%q host=%q", u.Scheme, u.Host)
	}
}

func TestResolveAutoServerPicksUDSWhenPresent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "api.sock")
	seedSocketInode(t, path)

	got := resolveAutoServer(path, "http://localhost:8080")
	if want := "unix://" + path; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveAutoServerFallsBackToTCPWhenAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.sock")
	got := resolveAutoServer(path, "http://localhost:8080")
	if got != "http://localhost:8080" {
		t.Errorf("got %q, want http://localhost:8080", got)
	}
}

func TestResolveAutoServerIgnoresNonSocketFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "regular.file")
	if err := os.WriteFile(path, []byte("not a socket"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got := resolveAutoServer(path, "http://localhost:8080")
	if got != "http://localhost:8080" {
		t.Errorf("got %q, want fallback", got)
	}
}

func TestNewAPIClientUDSRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "api.sock")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("pong " + r.Host))
	})
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{Handler: mux}
	done := make(chan struct{})
	go func() {
		_ = srv.Serve(ln)
		close(done)
	}()
	t.Cleanup(func() {
		_ = srv.Shutdown(context.Background())
		<-done
	})

	client, err := newAPIClient("unix://" + path)
	if err != nil {
		t.Fatalf("newAPIClient: %v", err)
	}
	if client.socketPath != path {
		t.Errorf("socketPath = %q, want %q", client.socketPath, path)
	}
	if client.requestBase() != unixHTTPHost {
		t.Errorf("requestBase = %q, want %q", client.requestBase(), unixHTTPHost)
	}

	req, err := http.NewRequest(http.MethodGet, client.resolvePath("/ping"), nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := client.httpClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "pong unix" {
		t.Errorf("body = %q, want %q", string(body), "pong unix")
	}
}

func TestContractCachePathDiffersByScheme(t *testing.T) {
	udsPath, err := contractCachePath("unix:///run/osvbng/api.sock")
	if err != nil {
		t.Fatalf("contractCachePath unix: %v", err)
	}
	tcpPath, err := contractCachePath("http://localhost:8080")
	if err != nil {
		t.Fatalf("contractCachePath tcp: %v", err)
	}
	if udsPath == tcpPath {
		t.Errorf("cache keys collided: %q", udsPath)
	}
}

func seedSocketInode(t *testing.T, path string) {
	t.Helper()
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("unix.Socket: %v", err)
	}
	if err := unix.Bind(fd, &unix.SockaddrUnix{Name: path}); err != nil {
		_ = unix.Close(fd)
		t.Fatalf("unix.Bind %s: %v", path, err)
	}
	_ = unix.Close(fd)
}
