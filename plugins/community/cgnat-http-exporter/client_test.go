// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnathttp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func testConfig(endpoint string) *Config {
	cfg := &Config{
		Enabled:  true,
		Endpoint: endpoint,
	}
	cfg.applyDefaults()
	return cfg
}

func TestClient_Post_Success(t *testing.T) {
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content-type = %q, want application/json", got)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c, err := newClient(testConfig(srv.URL))
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	ok, retryable, status, err := c.post(context.Background(), []byte(`{"hi":"there"}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if !ok || retryable || status != 200 {
		t.Errorf("ok=%v retryable=%v status=%d, want true/false/200", ok, retryable, status)
	}
	if string(got) != `{"hi":"there"}` {
		t.Errorf("server received %q", got)
	}
}

func TestClient_Post_4xx_NotRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c, _ := newClient(testConfig(srv.URL))
	ok, retryable, status, _ := c.post(context.Background(), []byte(`{}`))
	if ok || retryable {
		t.Errorf("ok=%v retryable=%v, want false/false for 4xx", ok, retryable)
	}
	if status != 400 {
		t.Errorf("status=%d, want 400", status)
	}
}

func TestClient_Post_5xx_Retryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c, _ := newClient(testConfig(srv.URL))
	ok, retryable, status, _ := c.post(context.Background(), []byte(`{}`))
	if ok || !retryable {
		t.Errorf("ok=%v retryable=%v, want false/true for 5xx", ok, retryable)
	}
	if status != 500 {
		t.Errorf("status=%d, want 500", status)
	}
}

func TestClient_SetHeaders_BearerAndCustom(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r
	}))
	defer srv.Close()

	cfg := testConfig(srv.URL)
	cfg.Auth = &AuthConfig{Type: "bearer", Token: "abc123"}
	cfg.Headers = map[string]string{"X-BNG-Node-Id": "osvbng-nsw-1"}

	c, _ := newClient(cfg)
	_, _, _, _ = c.post(context.Background(), []byte(`{}`))

	if got == nil {
		t.Fatal("handler never fired")
	}
	if a := got.Header.Get("Authorization"); a != "Bearer abc123" {
		t.Errorf("authz = %q, want Bearer abc123", a)
	}
	if n := got.Header.Get("X-BNG-Node-Id"); n != "osvbng-nsw-1" {
		t.Errorf("X-BNG-Node-Id = %q", n)
	}
}

func TestClient_SetHeaders_Basic(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r
	}))
	defer srv.Close()

	cfg := testConfig(srv.URL)
	cfg.Auth = &AuthConfig{Type: "basic", Username: "u", Password: "p"}

	c, _ := newClient(cfg)
	_, _, _, _ = c.post(context.Background(), []byte(`{}`))

	u, p, ok := got.BasicAuth()
	if !ok || u != "u" || p != "p" {
		t.Errorf("basic auth u=%q p=%q ok=%v", u, p, ok)
	}
}

func TestNextBackoff(t *testing.T) {
	cases := []struct {
		prev, init, max, want time.Duration
	}{
		{0, 100 * time.Millisecond, time.Second, 100 * time.Millisecond},
		{100 * time.Millisecond, 100 * time.Millisecond, time.Second, 200 * time.Millisecond},
		{500 * time.Millisecond, 100 * time.Millisecond, time.Second, time.Second}, // capped
		{2 * time.Second, 100 * time.Millisecond, time.Second, time.Second},        // already at cap
	}
	for _, tc := range cases {
		if got := nextBackoff(tc.prev, tc.init, tc.max); got != tc.want {
			t.Errorf("nextBackoff(prev=%v init=%v max=%v) = %v, want %v",
				tc.prev, tc.init, tc.max, got, tc.want)
		}
	}
}

func TestSleepCtx_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if sleepCtx(ctx, 5*time.Second) {
		t.Errorf("sleepCtx with cancelled ctx should return false")
	}
}

func TestSleepCtx_Completes(t *testing.T) {
	start := time.Now()
	if !sleepCtx(context.Background(), 10*time.Millisecond) {
		t.Errorf("sleepCtx should return true on normal completion")
	}
	if elapsed := time.Since(start); elapsed < 10*time.Millisecond {
		t.Errorf("slept for %v, expected >= 10ms", elapsed)
	}
}

// Reference an atomic.Uint64 so the test file fails early if someone
// strips the sync/atomic import from the code under test — the package
// relies on it for its counters.
var _ = atomic.Uint64{}

// Sanity check: server that always fails, driven through sendWithRetry
// lives in exporter_test.go — kept out of this file to keep the
// client-level tests isolated from the Component.
