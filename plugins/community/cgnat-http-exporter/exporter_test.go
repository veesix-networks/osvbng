// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnathttp

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
)

func loggerForTest() *logger.Logger { return logger.Get("cgnat-http-exporter-test") }

// fakeBus is a minimal in-process event bus that only implements what
// the Component needs. Using the real local bus would work too but
// couples these tests to its internals.
type fakeBus struct {
	mu       sync.Mutex
	handlers map[string][]events.Handler
}

func newFakeBus() *fakeBus { return &fakeBus{handlers: map[string][]events.Handler{}} }

func (b *fakeBus) Publish(topic string, ev events.Event) {
	b.mu.Lock()
	hs := append([]events.Handler(nil), b.handlers[topic]...)
	b.mu.Unlock()
	for _, h := range hs {
		h(ev)
	}
}

func (b *fakeBus) Subscribe(topic string, h events.Handler) events.Subscription {
	b.mu.Lock()
	b.handlers[topic] = append(b.handlers[topic], h)
	b.mu.Unlock()
	return &fakeSub{bus: b, topic: topic, handler: h}
}

func (b *fakeBus) SubscribeAll(events.Handler) events.Subscription { return noopSub{} }
func (b *fakeBus) Stats() events.Stats                             { return events.Stats{} }
func (b *fakeBus) SetDebugTopics([]string)                         {}
func (b *fakeBus) DebugTopics() []string                           { return nil }
func (b *fakeBus) Close() error                                    { return nil }

type fakeSub struct {
	bus     *fakeBus
	topic   string
	handler events.Handler
}

// Unsubscribe is a no-op for fakeBus — tests drive events explicitly
// and stop the worker via channel close, so we don't exercise the
// bus-side unsubscription path here.
func (s *fakeSub) Unsubscribe() {}

type noopSub struct{}

func (noopSub) Unsubscribe() {}

// sampleMapping returns a deterministic CGNATMapping for fixture use.
func sampleMapping() *models.CGNATMapping {
	return &models.CGNATMapping{
		SessionID:      "sess-1",
		PoolName:       "cgnat-syd-01",
		PoolID:         7,
		InsideIP:       net.ParseIP("10.50.14.9"),
		InsideVRFID:    42,
		OutsideIP:      net.ParseIP("100.64.12.7"),
		PortBlockStart: 49152,
		PortBlockEnd:   49351,
		SwIfIndex:      3,
	}
}

// TestComponent_EndToEnd drives one allocate + one release event
// through a Component wired to a local httptest server and asserts
// both land at the endpoint with the expected JSON shape.
func TestComponent_EndToEnd(t *testing.T) {
	var mu sync.Mutex
	var bodies [][]byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, b)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := testConfig(srv.URL)
	cfg.Workers = 1
	cfg.MaxRetries = 0
	cl, _ := newClient(cfg)

	c := &Component{
		logger: loggerForTest(),
		cfg:    cfg,
		bus:    newFakeBus(),
		client: cl,
		queue:  make(chan []byte, cfg.QueueSize),
	}
	// Use a raw manual lifecycle so the test doesn't need a full
	// component.Base wired to a runtime.
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.wg.Add(1)
	go c.worker(ctx)

	c.bus.Subscribe(events.TopicCGNATMapping, c.handleEvent)

	// Publish allocate + release.
	c.bus.Publish(events.TopicCGNATMapping, events.Event{
		Source: "cgnat",
		Data:   &events.CGNATMappingEvent{SRGName: "default", SessionID: "sess-1", Mapping: sampleMapping(), IsAdd: true},
	})
	c.bus.Publish(events.TopicCGNATMapping, events.Event{
		Source: "cgnat",
		Data:   &events.CGNATMappingEvent{SRGName: "default", SessionID: "sess-1", Mapping: sampleMapping(), IsAdd: false},
	})

	// Wait for both to land at the server, with a generous bound.
	waitFor(t, 2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(bodies) == 2
	})

	// Drain and stop cleanly.
	close(c.queue)
	cancel()
	c.wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 2 {
		t.Fatalf("got %d bodies, want 2", len(bodies))
	}

	events := []string{"allocate", "release"}
	for i, b := range bodies {
		var got payload
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("body[%d]: %v", i, err)
		}
		if got.Event != events[i] {
			t.Errorf("body[%d].event = %q, want %q", i, got.Event, events[i])
		}
		if got.OutsideIP != "100.64.12.7" || got.PortBlockStart != 49152 || got.PortBlockEnd != 49351 {
			t.Errorf("body[%d] mapping fields wrong: %+v", i, got)
		}
		if got.SessionID != "sess-1" || got.PoolName != "cgnat-syd-01" {
			t.Errorf("body[%d] session/pool fields wrong: %+v", i, got)
		}
		if got.InsideIP != "10.50.14.9" {
			t.Errorf("body[%d] inside_ip = %q, want 10.50.14.9", i, got.InsideIP)
		}
	}

	stats := c.GetStats()
	if stats.Received != 2 || stats.Sent != 2 || stats.Failed != 0 || stats.Dropped != 0 {
		t.Errorf("stats: %+v, want received=2 sent=2", stats)
	}
}

// TestComponent_IncludeInsideIP_False ensures the inside IP is omitted
// from the payload when operators disable that for privacy reasons.
func TestComponent_IncludeInsideIP_False(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := testConfig(srv.URL)
	no := false
	cfg.IncludeInsideIP = &no
	cl, _ := newClient(cfg)

	c := &Component{
		logger: loggerForTest(),
		cfg:    cfg,
		bus:    newFakeBus(),
		client: cl,
		queue:  make(chan []byte, 4),
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.wg.Add(1)
	go c.worker(ctx)

	c.handleEvent(events.Event{
		Source: "cgnat",
		Data:   &events.CGNATMappingEvent{SessionID: "s", Mapping: sampleMapping(), IsAdd: true},
	})

	waitFor(t, time.Second, func() bool { return len(body) > 0 })

	close(c.queue)
	cancel()
	c.wg.Wait()

	var got payload
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.InsideIP != "" || got.InsideVRFID != 0 {
		t.Errorf("inside fields leaked when IncludeInsideIP=false: %+v", got)
	}
	if got.OutsideIP == "" {
		t.Errorf("outside_ip missing — should still be present")
	}
}

// TestComponent_DropOnFull verifies the subscribe handler doesn't block
// when the queue is saturated; it increments the drop counter instead.
// This is the contract that keeps the CGNAT mapping hot path safe.
func TestComponent_DropOnFull(t *testing.T) {
	// Stall the server forever so the worker can't drain.
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-block
	}))
	defer func() { close(block); srv.Close() }()

	cfg := testConfig(srv.URL)
	cfg.QueueSize = 2
	cfg.Workers = 1
	cl, _ := newClient(cfg)

	c := &Component{
		logger: loggerForTest(),
		cfg:    cfg,
		bus:    newFakeBus(),
		client: cl,
		queue:  make(chan []byte, cfg.QueueSize),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.cancel = cancel
	c.wg.Add(1)
	go c.worker(ctx)

	// Push enough events that the queue must overflow. The first
	// lands at the worker (in-flight, server is stalled), the next
	// two fill the queue, then subsequent ones get dropped.
	for range 10 {
		c.handleEvent(events.Event{
			Source: "cgnat",
			Data:   &events.CGNATMappingEvent{SessionID: "x", Mapping: sampleMapping(), IsAdd: true},
		})
	}

	if got := c.received.Load(); got != 10 {
		t.Errorf("received = %d, want 10", got)
	}
	if c.dropped.Load() == 0 {
		t.Errorf("expected some drops when queue is saturated; dropped=0")
	}
}

// TestComponent_Marshal_NilMapping is a defensive test: bad event data
// must not crash the handler. The CGNAT component always sets Mapping,
// but if that invariant ever breaks we want the exporter to log and
// move on, not take the BNG process with it.
func TestComponent_Marshal_NilMapping(t *testing.T) {
	cfg := testConfig("http://ignored")
	cl, _ := newClient(cfg)
	c := &Component{
		logger: loggerForTest(),
		cfg:    cfg,
		bus:    newFakeBus(),
		client: cl,
		queue:  make(chan []byte, 1),
	}
	c.handleEvent(events.Event{Source: "cgnat", Data: &events.CGNATMappingEvent{Mapping: nil}})
	c.handleEvent(events.Event{Source: "cgnat", Data: nil})
	if c.received.Load() != 0 {
		t.Errorf("expected received=0 for malformed events, got %d", c.received.Load())
	}
}

// waitFor polls predicate up to d; fails the test if it never becomes true.
func waitFor(t *testing.T, d time.Duration, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v", d)
}

// Counter touch so linters notice sync/atomic usage even if future
// refactors swap the specific atomic we import here.
var _ = atomic.Uint64{}
