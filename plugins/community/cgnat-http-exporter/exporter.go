// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnathttp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
)

// Component is the cgnat-http-exporter. One per osvbng instance when
// the plugin is enabled. It subscribes to TopicCGNATMapping and fans
// events out to a worker pool that POSTs each one to the configured
// endpoint.
type Component struct {
	*component.Base

	logger *logger.Logger
	cfg    *Config
	bus    events.Bus
	client *client

	// queue carries marshalled event payloads from the subscribe
	// handler to the worker pool. Keeping the marshalled bytes on
	// the queue (not the raw *CGNATMappingEvent) means we do the
	// allocation once, and workers do not hold a reference to the
	// mapping object the CGNAT component owns.
	queue chan []byte

	// Counters. Exposed via GetStats() for `show cgnat-http-exporter`
	// style handlers or Prometheus in a future pass.
	received atomic.Uint64
	sent     atomic.Uint64
	failed   atomic.Uint64
	dropped  atomic.Uint64 // queue was full on ingest

	sub    events.Subscription
	wg     sync.WaitGroup
	cancel context.CancelFunc
}

// NewComponent is the plugin entry point wired via component.Register
// in config.go. Returns (nil, nil) when the plugin config is absent or
// disabled — same convention as the hello reference plugin.
func NewComponent(deps component.Dependencies) (component.Component, error) {
	raw, ok := configmgr.GetPluginConfig(Namespace)
	if !ok {
		return nil, nil
	}
	cfg, ok := raw.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type for %s", Namespace)
	}
	if !cfg.Enabled {
		return nil, nil
	}
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", Namespace, err)
	}
	if deps.EventBus == nil {
		return nil, fmt.Errorf("%s: event bus dependency missing", Namespace)
	}

	cl, err := newClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("%s: http client: %w", Namespace, err)
	}

	return &Component{
		Base:   component.NewBase(Namespace),
		logger: logger.Get(Namespace),
		cfg:    cfg,
		bus:    deps.EventBus,
		client: cl,
		queue:  make(chan []byte, cfg.QueueSize),
	}, nil
}

// Start subscribes to the CGNAT mapping topic and spins up the worker
// pool. Non-blocking — returns as soon as workers are running.
func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)

	workerCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	for i := 0; i < c.cfg.Workers; i++ {
		c.wg.Add(1)
		go c.worker(workerCtx)
	}

	c.sub = c.bus.Subscribe(events.TopicCGNATMapping, c.handleEvent)

	c.logger.Info("cgnat-http-exporter started",
		"endpoint", c.cfg.Endpoint,
		"workers", c.cfg.Workers,
		"queue_size", c.cfg.QueueSize,
		"max_retries", c.cfg.MaxRetries,
	)
	return nil
}

// Stop unsubscribes, drains the queue for a short grace period, then
// cancels the workers. Events still in the queue at shutdown are
// logged but not retried — they'd be racing against a dead process.
func (c *Component) Stop(ctx context.Context) error {
	if c.sub != nil {
		c.sub.Unsubscribe()
	}
	// Close the queue so workers see EOF and exit after draining.
	close(c.queue)

	// Give workers a few seconds to drain before we force-cancel.
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		c.logger.Warn("cgnat-http-exporter: workers did not drain in 5s, forcing shutdown",
			"queued", len(c.queue))
		if c.cancel != nil {
			c.cancel()
		}
		c.wg.Wait()
	}

	c.StopContext()
	c.logger.Info("cgnat-http-exporter stopped",
		"received", c.received.Load(),
		"sent", c.sent.Load(),
		"failed", c.failed.Load(),
		"dropped", c.dropped.Load(),
	)
	return nil
}

// handleEvent is called synchronously from the event bus. It MUST NOT
// block — the CGNAT component is the publisher and any hold here slows
// the mapping-allocation path. Non-blocking enqueue; drop on overflow.
func (c *Component) handleEvent(ev events.Event) {
	data, ok := ev.Data.(*events.CGNATMappingEvent)
	if !ok || data == nil || data.Mapping == nil {
		return
	}
	c.received.Add(1)

	body, err := c.marshal(data)
	if err != nil {
		c.failed.Add(1)
		c.logger.Warn("cgnat-http-exporter: marshal failed", "err", err)
		return
	}

	// Non-blocking send: drop if the queue is saturated. Dropping is
	// preferable to blocking the publisher (CGNAT mapping hot path).
	// Operators should alert on a non-zero drop counter.
	select {
	case c.queue <- body:
	default:
		c.dropped.Add(1)
		c.logger.Warn("cgnat-http-exporter: queue full, dropping event",
			"session_id", data.SessionID,
			"queue_size", c.cfg.QueueSize,
		)
	}
}

// worker drains the queue and POSTs each event. Retries with
// exponential backoff. Exits when the queue is closed and drained, or
// when ctx is cancelled (forced shutdown).
func (c *Component) worker(ctx context.Context) {
	defer c.wg.Done()
	for body := range c.queue {
		c.sendWithRetry(ctx, body)
		if ctx.Err() != nil {
			return
		}
	}
}

// sendWithRetry layers exponential backoff on top of client.post.
// Aborts early when the event is a 4xx (non-retryable) or when ctx is
// cancelled. Updates sent/failed counters.
func (c *Component) sendWithRetry(ctx context.Context, body []byte) {
	var delay time.Duration
	for attempt := 0; attempt <= c.cfg.MaxRetries; attempt++ {
		ok, retryable, status, err := c.client.post(ctx, body)
		if ok {
			c.sent.Add(1)
			return
		}
		if !retryable || attempt == c.cfg.MaxRetries {
			c.failed.Add(1)
			c.logger.Warn("cgnat-http-exporter: send failed",
				"attempt", attempt+1,
				"status", status,
				"err", err,
				"retryable", retryable,
			)
			return
		}
		delay = nextBackoff(delay, c.cfg.RetryInitial, c.cfg.RetryMax)
		if !sleepCtx(ctx, delay) {
			return
		}
	}
}

// payload is the JSON structure we POST per event. Field names chosen
// for consumer ergonomics over matching internal Go naming exactly.
type payload struct {
	Event          string    `json:"event"` // "allocate" | "release"
	At             time.Time `json:"at"`
	SRGName        string    `json:"srg_name,omitempty"`
	SessionID      string    `json:"session_id,omitempty"`
	PoolName       string    `json:"pool_name"`
	PoolID         uint32    `json:"pool_id"`
	OutsideIP      string    `json:"outside_ip"`
	PortBlockStart uint16    `json:"port_block_start"`
	PortBlockEnd   uint16    `json:"port_block_end"`
	InsideIP       string    `json:"inside_ip,omitempty"`
	InsideVRFID    uint32    `json:"inside_vrf_id,omitempty"`
}

func (c *Component) marshal(ev *events.CGNATMappingEvent) ([]byte, error) {
	m := ev.Mapping
	p := payload{
		Event:          eventKind(ev.IsAdd),
		At:             time.Now().UTC(),
		SRGName:        ev.SRGName,
		SessionID:      ev.SessionID,
		PoolName:       m.PoolName,
		PoolID:         m.PoolID,
		OutsideIP:      ipString(m.OutsideIP),
		PortBlockStart: m.PortBlockStart,
		PortBlockEnd:   m.PortBlockEnd,
	}
	if c.cfg.IncludeInsideIP != nil && *c.cfg.IncludeInsideIP {
		p.InsideIP = ipString(m.InsideIP)
		p.InsideVRFID = m.InsideVRFID
	}
	return json.Marshal(&p)
}

func eventKind(isAdd bool) string {
	if isAdd {
		return "allocate"
	}
	return "release"
}

// ipString handles a nil net.IP gracefully — test events sometimes omit
// it, and we'd rather emit "" than panic on .String().
func ipString(ip interface{ String() string }) string {
	if ip == nil {
		return ""
	}
	return ip.String()
}

// Stats snapshots the atomic counters. Exposed so a future show handler
// can render them without reaching into atomics directly.
type Stats struct {
	Received uint64 `json:"received"`
	Sent     uint64 `json:"sent"`
	Failed   uint64 `json:"failed"`
	Dropped  uint64 `json:"dropped"`
	Queued   int    `json:"queued"`
}

func (c *Component) GetStats() Stats {
	return Stats{
		Received: c.received.Load(),
		Sent:     c.sent.Load(),
		Failed:   c.failed.Load(),
		Dropped:  c.dropped.Load(),
		Queued:   len(c.queue),
	}
}

// compile-time check that we satisfy the component interface.
var _ component.Component = (*Component)(nil)

// compile-time sanity check on event payload — catches upstream renames.
var _ = (*events.CGNATMappingEvent)(&events.CGNATMappingEvent{Mapping: &models.CGNATMapping{}})
