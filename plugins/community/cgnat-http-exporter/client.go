// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnathttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/veesix-networks/osvbng/pkg/netbind"
)

// client owns the *http.Client plus the request-shaping logic. Kept
// small on purpose — no retry/backoff state lives here (see sendWithRetry
// for that); this struct is just "build + execute a single request".
type client struct {
	cfg  *Config
	http *http.Client
}

func newClient(cfg *Config) (*client, error) {
	binding, err := cfg.EndpointBinding.Resolve(netbind.FamilyV4, nil)
	if err != nil {
		return nil, fmt.Errorf("resolve binding: %w", err)
	}
	httpClient := netbind.HTTPClient(binding, cfg.Timeout)

	if cfg.TLS != nil {
		tlsCfg, err := buildTLSConfig(cfg.TLS)
		if err != nil {
			return nil, err
		}
		if t, ok := httpClient.Transport.(*http.Transport); ok {
			t.TLSClientConfig = tlsCfg
		}
	}

	return &client{cfg: cfg, http: httpClient}, nil
}

// post issues one HTTP request. Retry logic is layered on top by the
// caller (sendWithRetry).
//
// Returned booleans:
//   - ok:      true when the server responded 2xx.
//   - retryable: true when the caller should back off and retry; false
//     for 4xx responses (configuration-level errors — retrying won't fix
//     them, and hammering the portal for a bad payload would be rude).
//     Network errors and 5xx are retryable.
func (c *client) post(ctx context.Context, body []byte) (ok, retryable bool, status int, err error) {
	req, err := http.NewRequestWithContext(ctx, c.cfg.Method, c.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return false, false, 0, fmt.Errorf("build request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		// Network / DNS / timeout — transient, retryable.
		return false, true, 0, err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return true, false, resp.StatusCode, nil
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		// Client error — our payload or auth is wrong. Don't retry;
		// surface to the operator via logs + metrics instead.
		return false, false, resp.StatusCode, fmt.Errorf("http %d", resp.StatusCode)
	default:
		// 5xx, 3xx-non-redirect, anything else — retryable.
		return false, true, resp.StatusCode, fmt.Errorf("http %d", resp.StatusCode)
	}
}

func (c *client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")

	if c.cfg.Auth != nil {
		switch strings.ToLower(c.cfg.Auth.Type) {
		case "basic":
			req.SetBasicAuth(c.cfg.Auth.Username, c.cfg.Auth.Password)
		case "bearer":
			req.Header.Set("Authorization", "Bearer "+c.cfg.Auth.Token)
		}
	}
	for k, v := range c.cfg.Headers {
		req.Header.Set(k, v)
	}
}

func buildTLSConfig(cfg *TLSConfig) (*tls.Config, error) {
	out := &tls.Config{
		InsecureSkipVerify: cfg.InsecureSkipVerify,
		MinVersion:         tls.VersionTLS12,
	}
	if cfg.CACertFile != "" {
		pem, err := os.ReadFile(cfg.CACertFile)
		if err != nil {
			return nil, fmt.Errorf("read ca cert %q: %w", cfg.CACertFile, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("parse ca cert %q", cfg.CACertFile)
		}
		out.RootCAs = pool
	}
	if cfg.CertFile != "" && cfg.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load client keypair: %w", err)
		}
		out.Certificates = []tls.Certificate{cert}
	}
	return out, nil
}

// nextBackoff returns the next delay in an exponential sequence clamped
// at max. Pure function to keep worker logic testable without fakes.
func nextBackoff(prev, initial, max time.Duration) time.Duration {
	if prev == 0 {
		return initial
	}
	next := prev * 2
	if next > max {
		return max
	}
	return next
}

// sleepCtx sleeps for d unless ctx is cancelled first; returns false if
// ctx cancelled (caller should abandon the work).
func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
