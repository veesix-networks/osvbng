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
	"time"

	"github.com/veesix-networks/osvbng/pkg/configmgr"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/northbound"
)

func TestUDSEndToEndOpenAPISpec(t *testing.T) {
	udsPath := filepath.Join(t.TempDir(), "api.sock")

	configd := configmgr.NewConfigManager()
	adapter := northbound.NewAdapter(show.NewRegistry(), configd.GetRegistry(), oper.NewRegistry(), configd)

	c := &Component{
		logger:   logger.Get(Namespace),
		adapter:  adapter,
		cfg:      &Config{Enabled: true, UDS: UDSConfig{Enabled: true, Path: udsPath, Mode: "0660", Group: "nogroup"}},
		specJSON: []byte(`{"openapi":"3.0.3"}`),
		specETag: `"e2e-etag"`,
	}
	c.server = &http.Server{Handler: c.newMux()}

	ln, err := listenUDS(c.cfg.UDS, c.logger)
	if err != nil {
		t.Fatalf("listenUDS: %v", err)
	}
	done := make(chan struct{})
	go func() {
		_ = c.server.Serve(ln)
		close(done)
	}()
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = c.server.Shutdown(shutdownCtx)
		<-done
		_ = os.Remove(udsPath)
	})

	client, err := newAPIClient(t, "unix://"+udsPath)
	if err != nil {
		t.Fatalf("client: %v", err)
	}

	resp, err := client.Get(unixHTTPHost + "/api/openapi.json")
	if err != nil {
		t.Fatalf("GET /api/openapi.json: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Fatal("empty body")
	}
}

func TestMultiTCPListenerBindAndServe(t *testing.T) {
	ln1, addr1 := freeTCPListener(t)
	ln2, addr2 := freeTCPListener(t)
	ln1.Close()
	ln2.Close()

	configd := configmgr.NewConfigManager()
	adapter := northbound.NewAdapter(show.NewRegistry(), configd.GetRegistry(), oper.NewRegistry(), configd)

	cfg := &Config{
		Enabled: true,
		Listeners: []ListenerConfig{
			{Address: addr1},
			{Address: addr2},
		},
	}
	if err := cfg.validateListeners(); err != nil {
		t.Fatalf("validateListeners: %v", err)
	}

	c := &Component{
		logger:   logger.Get(Namespace),
		adapter:  adapter,
		cfg:      cfg,
		specJSON: []byte(`{"openapi":"3.0.3"}`),
		specETag: `"multi-etag"`,
	}
	c.server = &http.Server{Handler: c.newMux()}

	openAndServe(t, c, addr1, ln1)
	openAndServe(t, c, addr2, ln2)
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = c.server.Shutdown(shutdownCtx)
	})

	for _, addr := range []string{addr1, addr2} {
		resp, err := http.Get("http://" + addr + "/api/openapi.json")
		if err != nil {
			t.Fatalf("GET %s: %v", addr, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s status = %d, want 200", addr, resp.StatusCode)
		}
	}
}

func freeTCPListener(t *testing.T) (net.Listener, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ephemeral listen: %v", err)
	}
	return ln, ln.Addr().String()
}

func openAndServe(t *testing.T, c *Component, addr string, _ net.Listener) {
	t.Helper()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("rebind %s: %v", addr, err)
	}
	go func() { _ = c.server.Serve(ln) }()
}

const unixHTTPHost = "http://unix"

func newAPIClient(t *testing.T, server string) (*http.Client, error) {
	t.Helper()
	const prefix = "unix://"
	if len(server) <= len(prefix) || server[:len(prefix)] != prefix {
		t.Fatalf("test helper only supports unix:// URLs, got %q", server)
	}
	socketPath := server[len(prefix):]
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
			},
		},
	}, nil
}
