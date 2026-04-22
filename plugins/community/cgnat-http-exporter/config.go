// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

// Package cgnathttp is a community exporter plugin that subscribes to
// TopicCGNATMapping and POSTs each port-block allocate/release event
// to a configured HTTP endpoint.
//
// The event stream is the authoritative source for lawful-intercept and
// metadata-retention correlation: "at time T, this outside IP:port range
// was assigned to this subscriber session". A downstream portal can
// persist the POSTs into an append-only log and serve reverse lookups.
//
// Events are consumed off the bus asynchronously (bounded in-memory
// queue + worker goroutines). The subscribe handler never blocks the
// CGNAT component's mapping hot path — if the queue is full the event
// is dropped and a counter is incremented so operators can alert on it.
package cgnathttp

import (
	"fmt"
	"time"

	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
)

// Namespace is the plugin config key. Matches YAML under `plugins:`.
const Namespace = "exporter.cgnat.http"

// Config controls the exporter. Zero-values resolve to the defaults in
// applyDefaults. Timeout / retry / queue sizing are chosen so that an
// operator who only sets `enabled: true` + `endpoint:` gets a useful
// exporter without further tuning.
type Config struct {
	Enabled bool `json:"enabled" yaml:"enabled"`

	// Endpoint is the absolute URL that each event is POSTed to.
	Endpoint string `json:"endpoint" yaml:"endpoint"`

	// Method overrides the HTTP verb. Defaults to POST; any standard
	// verb is accepted but POST is the only one that makes semantic
	// sense for an event stream.
	Method string `json:"method,omitempty" yaml:"method,omitempty"`

	// Timeout bounds each individual HTTP request. Retries get their
	// own timeout budget.
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`

	// TLS + Auth + Headers mirror the subscriber.auth.http plugin for
	// operator familiarity.
	TLS     *TLSConfig        `json:"tls,omitempty" yaml:"tls,omitempty"`
	Auth    *AuthConfig       `json:"auth,omitempty" yaml:"auth,omitempty"`
	Headers map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`

	// QueueSize is the capacity of the internal buffer between the
	// event subscriber and the HTTP worker(s). Events arriving when
	// the queue is full are dropped (see Metric_EventsDropped).
	QueueSize int `json:"queue_size,omitempty" yaml:"queue_size,omitempty"`

	// Workers is the number of concurrent goroutines draining the
	// queue. One is enough for most deployments; increase if the
	// downstream portal is slow and the queue backs up.
	Workers int `json:"workers,omitempty" yaml:"workers,omitempty"`

	// MaxRetries is the number of attempts after the initial POST
	// before an event is given up on. Zero means no retry (single
	// POST attempt).
	MaxRetries int `json:"max_retries,omitempty" yaml:"max_retries,omitempty"`

	// RetryInitial / RetryMax bound exponential backoff. Delay
	// doubles after each failure and is capped at RetryMax.
	RetryInitial time.Duration `json:"retry_initial,omitempty" yaml:"retry_initial,omitempty"`
	RetryMax     time.Duration `json:"retry_max,omitempty"     yaml:"retry_max,omitempty"`

	// IncludeInsideIP controls whether the subscriber's inside
	// (private) IP is included in the POST body. Default true.
	// Disable if your downstream system shouldn't see inside IPs for
	// privacy/compliance reasons — outside IP + port range are still
	// sufficient to correlate via session_id.
	IncludeInsideIP *bool `json:"include_inside_ip,omitempty" yaml:"include_inside_ip,omitempty"`
}

type TLSConfig struct {
	InsecureSkipVerify bool   `json:"insecure_skip_verify,omitempty" yaml:"insecure_skip_verify,omitempty"`
	CACertFile         string `json:"ca_cert_file,omitempty"         yaml:"ca_cert_file,omitempty"`
	CertFile           string `json:"cert_file,omitempty"            yaml:"cert_file,omitempty"`
	KeyFile            string `json:"key_file,omitempty"             yaml:"key_file,omitempty"`
}

type AuthConfig struct {
	Type     string `json:"type,omitempty"     yaml:"type,omitempty"` // "basic" | "bearer"
	Username string `json:"username,omitempty" yaml:"username,omitempty"`
	Password string `json:"password,omitempty" yaml:"password,omitempty"`
	Token    string `json:"token,omitempty"    yaml:"token,omitempty"`
}

// Defaults — tuned for a typical BNG that processes a few thousand PBA
// events per second at peak. QueueSize of 10k absorbs ~30s of backlog
// at 300 events/s; operators with hotter dataplanes should raise it.
const (
	defaultMethod       = "POST"
	defaultTimeout      = 5 * time.Second
	defaultQueueSize    = 10000
	defaultWorkers      = 1
	defaultMaxRetries   = 3
	defaultRetryInitial = 500 * time.Millisecond
	defaultRetryMax     = 30 * time.Second
)

// applyDefaults fills in zero-value fields. Mutates c.
func (c *Config) applyDefaults() {
	if c.Method == "" {
		c.Method = defaultMethod
	}
	if c.Timeout == 0 {
		c.Timeout = defaultTimeout
	}
	if c.QueueSize == 0 {
		c.QueueSize = defaultQueueSize
	}
	if c.Workers == 0 {
		c.Workers = defaultWorkers
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = defaultMaxRetries
	}
	if c.RetryInitial == 0 {
		c.RetryInitial = defaultRetryInitial
	}
	if c.RetryMax == 0 {
		c.RetryMax = defaultRetryMax
	}
	if c.IncludeInsideIP == nil {
		b := true
		c.IncludeInsideIP = &b
	}
}

// validate returns a descriptive error if the config is unusable. Called
// from NewComponent before the worker is started so misconfiguration is
// caught at load time instead of at the first event.
func (c *Config) validate() error {
	if c.Endpoint == "" {
		return fmt.Errorf("endpoint is required")
	}
	if c.Workers < 1 {
		return fmt.Errorf("workers must be >= 1")
	}
	if c.QueueSize < 1 {
		return fmt.Errorf("queue_size must be >= 1")
	}
	if c.MaxRetries < 0 {
		return fmt.Errorf("max_retries must be >= 0")
	}
	if c.RetryInitial < 0 || c.RetryMax < c.RetryInitial {
		return fmt.Errorf("retry_initial must be <= retry_max and both non-negative")
	}
	if c.Auth != nil {
		switch c.Auth.Type {
		case "", "basic", "bearer":
		default:
			return fmt.Errorf("auth.type must be empty, basic, or bearer")
		}
	}
	return nil
}

func init() {
	configmgr.RegisterPluginConfig(Namespace, Config{})
	component.Register(Namespace, NewComponent,
		component.WithAuthor("veesix ::networks contributors"),
		component.WithVersion("1.0.0"),
	)
}
