// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/dataplane"
	l2tppkt "github.com/veesix-networks/osvbng/pkg/l2tp"
)

// SendControlFn transmits an L2TPv2 control packet. The implementation
// is injected by the cmd-level wiring because the right path depends
// on the runtime topology (kernel UDP socket, VPP punt-egress, or a
// VPP TAP host interface). The contract is "deliver this body as an
// IPv4 + UDP + L2TPv2 packet from `localIP:localPort` to
// `peerIP:peerPort`"; the implementation is responsible for the IP
// framing and any FIB / routing concerns.
//
// `body` is the L2TP-message payload starting with the AVP sequence;
// the implementation prepends the L2TP header with the supplied Ns/Nr
// (already chosen by the per-tunnel control channel) and is free to
// build the IP/UDP framing however it sees fit.
type SendControlFn func(localIP, peerIP net.IP, localPort, peerPort uint16, header l2tppkt.Header, body []byte) error

// tunnelRunner is the per-tunnel control-plane goroutine. It owns the
// retransmit / ZLB / Hello timers for one tunnel and serialises
// inbound message dispatch with outbound sends so the control channel
// is single-threaded per RFC 2661 §5.4.
//
// Lifecycle: started when the tunnel reaches `WaitCtlReply` (initiator)
// or `WaitCtlConn` (responder); stopped when the tunnel FSM enters
// Cleanup or the owning Component shuts down.
type tunnelRunner struct {
	tunnel *Tunnel
	send   SendControlFn

	hello  *HelloScheduler
	stop   chan struct{}
	done   chan struct{}
	tickCh chan time.Time

	mu      sync.Mutex
	stopped bool
}

func newTunnelRunner(t *Tunnel, send SendControlFn, helloInterval time.Duration) *tunnelRunner {
	return &tunnelRunner{
		tunnel: t,
		send:   send,
		hello:  NewHelloScheduler(helloInterval),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
		tickCh: make(chan time.Time, 1),
	}
}

func (r *tunnelRunner) Start() {
	if r.tunnel.Channel == nil {
		return
	}
	r.hello.Start(func(body []byte) error {
		// HELLO is a reliable control message and must increment Ns.
		// Routing through the channel queues it for retransmit and
		// keeps the send sequence monotonic.
		return r.tunnel.Channel.Send(body, time.Now())
	})

	go r.loop()
}

func (r *tunnelRunner) Stop() {
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	r.stopped = true
	close(r.stop)
	r.mu.Unlock()

	r.hello.Stop()
	<-r.done
}

func (r *tunnelRunner) loop() {
	defer close(r.done)
	t := time.NewTimer(200 * time.Millisecond)
	defer t.Stop()

	for {
		select {
		case <-r.stop:
			return
		case <-t.C:
		}
		next := r.tunnel.Channel.Tick(time.Now())
		if next.IsZero() {
			t.Reset(500 * time.Millisecond)
		} else {
			d := time.Until(next)
			if d < 50*time.Millisecond {
				d = 50 * time.Millisecond
			}
			t.Reset(d)
		}
	}
}

func (r *tunnelRunner) sendBody(body []byte, sessionID, ns, nr uint16) error {
	h := l2tppkt.NewControl(r.tunnel.PeerID, sessionID, ns, nr)
	return r.send(r.tunnel.LocalIP, r.tunnel.PeerIP,
		r.tunnel.LocalPort, r.tunnel.PeerPort, *h, body)
}

// startTunnelRunner attaches a runner to the tunnel and registers the
// control-channel send callback so retransmits/ZLBs route through the
// component's SendControlFn. Called once per tunnel after the FSM has
// moved out of Idle.
func (c *Component) startTunnelRunner(t *Tunnel, helloInterval time.Duration) {
	if c.send == nil {
		return
	}
	r := newTunnelRunner(t, c.send, helloInterval)

	cfg := l2tppkt.Config{PeerRWS: 16}
	t.Channel = l2tppkt.NewControlChannel(cfg, func(body []byte, sessionID, ns, nr uint16) error {
		return r.sendBody(body, sessionID, ns, nr)
	}, func() {
		// Channel declared dead — drive the tunnel to Cleanup.
		t.FSM.Stop()
		c.unregisterTunnel(t.PeerIP, t.LocalID)
		c.uninstallTunnelVPP(t)
	})

	c.mu.Lock()
	if c.runners == nil {
		c.runners = make(map[tunnelKey]*tunnelRunner)
	}
	c.runners[makeTunnelKey(t.PeerIP, t.LocalID)] = r
	c.mu.Unlock()

	r.Start()
}

// stopTunnelRunner ends the runner for a tunnel that has been removed.
func (c *Component) stopTunnelRunner(peerIP net.IP, localID uint16) {
	c.mu.Lock()
	r := c.runners[makeTunnelKey(peerIP, localID)]
	delete(c.runners, makeTunnelKey(peerIP, localID))
	c.mu.Unlock()
	if r != nil {
		r.Stop()
	}
}

// puntConsumer reads parsed L2TP punt packets from the dataplane and
// hands each to Dispatch. One goroutine per Component; Dispatch itself
// is short and non-blocking (per-tunnel work happens on the runner).
func (c *Component) puntConsumer(ctx context.Context, ch <-chan *dataplane.ParsedPacket) {
	for {
		select {
		case <-ctx.Done():
			return
		case pkt, ok := <-ch:
			if !ok {
				return
			}
			if err := c.Dispatch(pkt); err != nil {
				c.log.Debug("l2tp dispatch error", "error", err)
			}
		}
	}
}
