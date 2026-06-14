// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/logger"
	"golang.org/x/sys/unix"
)

func TestApplyUDSDefaults(t *testing.T) {
	cfg := &Config{}
	applyUDSDefaults(cfg)
	if cfg.UDS.Path != defaultUDSPath {
		t.Errorf("Path = %q, want %q", cfg.UDS.Path, defaultUDSPath)
	}
	if cfg.UDS.Mode != defaultUDSMode {
		t.Errorf("Mode = %q, want %q", cfg.UDS.Mode, defaultUDSMode)
	}
	if cfg.UDS.Group != defaultUDSGroup {
		t.Errorf("Group = %q, want %q", cfg.UDS.Group, defaultUDSGroup)
	}
}

func TestApplyUDSDefaultsPreservesOverrides(t *testing.T) {
	cfg := &Config{UDS: UDSConfig{Path: "/tmp/x.sock", Mode: "0600", Group: "wheel"}}
	applyUDSDefaults(cfg)
	if cfg.UDS.Path != "/tmp/x.sock" || cfg.UDS.Mode != "0600" || cfg.UDS.Group != "wheel" {
		t.Errorf("operator overrides clobbered: %+v", cfg.UDS)
	}
}

func TestPrepareUDSPathNonExistent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.sock")
	if err := prepareUDSPath(path); err != nil {
		t.Fatalf("prepareUDSPath: %v", err)
	}
}

func TestPrepareUDSPathStaleSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stale.sock")
	seedStaleUDS(t, path)

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stale socket should exist after fd close: %v", err)
	}
	if fi.Mode()&os.ModeSocket == 0 {
		t.Fatalf("expected socket, got mode %v", fi.Mode())
	}

	if err := prepareUDSPath(path); err != nil {
		t.Fatalf("prepareUDSPath: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("stale socket not removed: %v", err)
	}
}

// seedStaleUDS leaves a bound AF_UNIX inode at path without Go's
// *UnixListener.Close cleanup hook, mimicking what survives a SIGKILL
// of a process that was listening on the socket.
func seedStaleUDS(t *testing.T, path string) {
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

func TestPrepareUDSPathRefusesNonSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "regular.file")
	if err := os.WriteFile(path, []byte("hi"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	err := prepareUDSPath(path)
	if err == nil {
		t.Fatal("expected error refusing to overwrite non-socket file")
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("non-socket file was removed: %v", statErr)
	}
}

func TestParseUDSMode(t *testing.T) {
	cases := map[string]os.FileMode{"0660": 0o660, "0600": 0o600, "0644": 0o644, "660": 0o660}
	for in, want := range cases {
		got, err := parseUDSMode(in)
		if err != nil {
			t.Errorf("parseUDSMode(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseUDSMode(%q) = %o, want %o", in, got, want)
		}
	}
	if _, err := parseUDSMode("nope"); err == nil {
		t.Error("parseUDSMode(\"nope\") expected error")
	}
}

func TestListenUDSBindsAndChmods(t *testing.T) {
	path := filepath.Join(t.TempDir(), "api.sock")
	ln, err := listenUDS(UDSConfig{Enabled: true, Path: path, Mode: "0660", Group: "nogroup"}, logger.Get("test"))
	if err != nil {
		t.Fatalf("listenUDS: %v", err)
	}
	defer ln.Close()

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Mode()&os.ModeSocket == 0 {
		t.Errorf("expected socket, got mode %v", fi.Mode())
	}
	if perm := fi.Mode().Perm(); perm != 0o660 {
		t.Errorf("perm = %o, want 0660", perm)
	}
}

func TestListenUDSGracefulOnUnknownGroup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "api.sock")
	ln, err := listenUDS(UDSConfig{Enabled: true, Path: path, Mode: "0660", Group: "definitely-not-a-real-group-xyz"}, logger.Get("test"))
	if err != nil {
		t.Fatalf("listenUDS should not fail on unknown group, got: %v", err)
	}
	defer ln.Close()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("socket missing: %v", err)
	}
}

func TestListenUDSRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "api.sock")
	ln, err := listenUDS(UDSConfig{Enabled: true, Path: path, Mode: "0660", Group: "nogroup"}, logger.Get("test"))
	if err != nil {
		t.Fatalf("listenUDS: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("pong"))
	})
	srv := &http.Server{Handler: mux}
	done := make(chan struct{})
	go func() {
		_ = srv.Serve(ln)
		close(done)
	}()
	t.Cleanup(func() {
		_ = srv.Shutdown(context.Background())
		<-done
		_ = os.Remove(path)
	})

	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", path)
		},
	}}
	resp, err := client.Get("http://unix/ping")
	if err != nil {
		t.Fatalf("GET /ping: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "pong" {
		t.Errorf("body = %q, want %q", string(body), "pong")
	}
}
