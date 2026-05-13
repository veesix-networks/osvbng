// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"errors"
	"time"
)

// ControlChannel implements the L2TPv2 reliable control channel per
// RFC 2661 §5.4–5.8.
//
// Responsibilities:
//
//   - Per-tunnel send sequence (Ns) and next-expected receive (Nr).
//   - Sliding send window bounded by the peer's Receive Window Size
//     AVP (cwnd ≤ peer_rws).
//   - Slow-start (RFC 2661 §5.8): cwnd starts at 1, doubles up to
//     ssthresh on each ACK, then linear growth.
//   - Retransmit with exponential back-off, max retries → tunnel dead.
//   - ZLB ACKs when we owe an ACK but have nothing to piggyback on.
//
// The channel does NOT send packets directly. It is driven by the
// owning tunnel goroutine via Send/Recv/Tick and emits outbound
// frames via the SendFunc callback. This keeps the channel
// transport-agnostic and unit-testable without UDP sockets.

// Defaults match RFC 2661 SHOULD values and the ze guide reference
// implementation. Operators can override per profile.
const (
	DefaultRTOInitial    = 1 * time.Second
	DefaultRTOMax        = 8 * time.Second
	DefaultMaxRetries    = 5
	DefaultReceiveWindow = 4
	DefaultZLBDelay      = 200 * time.Millisecond
)

// SendFunc serializes and transmits a control message body with the
// given Ns, Nr, and Session-ID in the L2TP header. The tunnel ID is
// owned by the caller; the channel only knows about sequence numbers
// and the per-message session id supplied via Send.
//
// `body` is the AVP-sequence payload; the channel does not interpret
// it. The implementation typically prepends a Header and writes to a
// UDP socket. `sessionID` is 0 for tunnel-level control messages
// (SCCRQ/SCCRP/SCCCN/StopCCN/HELLO and ZLBs) and the peer Session-ID
// for session-scoped messages (ICRQ/ICRP/ICCN/CDN/WEN/SLI) per RFC
// 2661 §3.1.
type SendFunc func(body []byte, sessionID, ns, nr uint16) error

// DeadFunc is invoked when the channel exhausts MaxRetries on the
// outstanding message. The tunnel should treat the peer as gone and
// drive the tunnel FSM to Cleanup.
type DeadFunc func()

type pendingMsg struct {
	body      []byte
	sessionID uint16
	ns        uint16
	attempts  int
	deadline  time.Time
}

// ControlChannel state. All methods are intended to be called from a
// single goroutine (the per-tunnel control goroutine). The channel is
// not internally synchronised.
type ControlChannel struct {
	send SendFunc
	dead DeadFunc

	rtoInitial time.Duration
	rtoMax     time.Duration
	maxRetries int
	zlbDelay   time.Duration

	// Sequence state.
	ns uint16 // next Ns to assign to an outbound message
	nr uint16 // next Ns we expect from the peer (== highest_seen + 1)

	// Slow-start / congestion avoidance.
	cwnd       int // current send window size in messages
	ssthresh   int // slow-start threshold
	peerWindow int // peer's RWS (advertised receive window size)

	// Queue of outbound messages awaiting transmission or ACK. Head
	// is the oldest unacknowledged message. Queue indices are pure
	// FIFO; the channel does not reorder.
	queue []pendingMsg

	// Next retransmit deadline (earliest deadline across in-flight).
	// Zero when nothing is in flight.
	nextRTO time.Time

	// ZLB deadline: when set, we owe an ACK and will emit a ZLB at
	// this time if no piggyback has happened.
	zlbDeadline time.Time
}

// Config bundles per-tunnel knobs. All durations default to the
// RFC 2661 SHOULD values when zero.
type Config struct {
	RTOInitial time.Duration
	RTOMax     time.Duration
	MaxRetries int
	PeerRWS    int // peer's Receive Window Size; 0 ⇒ DefaultReceiveWindow
	ZLBDelay   time.Duration
}

func NewControlChannel(cfg Config, send SendFunc, dead DeadFunc) *ControlChannel {
	if cfg.RTOInitial == 0 {
		cfg.RTOInitial = DefaultRTOInitial
	}
	if cfg.RTOMax == 0 {
		cfg.RTOMax = DefaultRTOMax
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = DefaultMaxRetries
	}
	if cfg.PeerRWS == 0 {
		cfg.PeerRWS = DefaultReceiveWindow
	}
	if cfg.ZLBDelay == 0 {
		cfg.ZLBDelay = DefaultZLBDelay
	}

	return &ControlChannel{
		send:       send,
		dead:       dead,
		rtoInitial: cfg.RTOInitial,
		rtoMax:     cfg.RTOMax,
		maxRetries: cfg.MaxRetries,
		zlbDelay:   cfg.ZLBDelay,
		cwnd:       1,
		ssthresh:   cfg.PeerRWS,
		peerWindow: cfg.PeerRWS,
	}
}

// SetPeerWindow updates the peer's Receive Window Size, typically
// after SCCRP / SCCCN arrival when the RWS AVP is known.
func (c *ControlChannel) SetPeerWindow(rws int) {
	if rws < 1 {
		rws = 1
	}
	c.peerWindow = rws
	if c.ssthresh > rws {
		c.ssthresh = rws
	}
	if c.cwnd > rws {
		c.cwnd = rws
	}
}

var ErrChannelDead = errors.New("l2tp: control channel dead, message dropped")

// Send enqueues a tunnel-level control message (Session-ID 0) for
// transmission. Use SendSession for session-scoped messages. If the
// message fits in the current send window it is transmitted
// immediately and the retransmit timer is armed; otherwise it queues
// behind in-flight messages.
//
// `body` is retained by the channel until ACKed; callers should pass
// a freshly-allocated slice or accept that mutating their copy will
// affect retransmissions.
func (c *ControlChannel) Send(body []byte, now time.Time) error {
	return c.SendSession(body, 0, now)
}

// SendSession is the session-scoped variant of Send. The Session-ID
// is carried in the L2TP header of the outbound message (and on every
// retransmission). RFC 2661 §3.1: ICRQ/ICRP/ICCN/CDN/WEN/SLI carry
// the peer Session-ID; tunnel-level messages carry 0.
func (c *ControlChannel) SendSession(body []byte, sessionID uint16, now time.Time) error {
	m := pendingMsg{
		body:      body,
		sessionID: sessionID,
		ns:        c.ns,
	}
	c.ns++
	c.queue = append(c.queue, m)
	return c.driveSend(now)
}

// driveSend transmits as many queued messages as fit in the current
// send window. Sets the retransmit deadline on the oldest in-flight.
func (c *ControlChannel) driveSend(now time.Time) error {
	// Count in-flight messages — those at the head of the queue with
	// `attempts > 0`. New messages have `attempts == 0`.
	inflight := 0
	for i := range c.queue {
		if c.queue[i].attempts > 0 {
			inflight++
		}
	}

	for i := range c.queue {
		if c.queue[i].attempts > 0 {
			continue
		}
		if inflight >= c.cwnd {
			break
		}
		c.queue[i].attempts = 1
		c.queue[i].deadline = now.Add(c.rtoInitial)
		if err := c.send(c.queue[i].body, c.queue[i].sessionID, c.queue[i].ns, c.nr); err != nil {
			return err
		}
		inflight++
		// Sending a message piggybacks the current Nr, satisfying any
		// pending ZLB obligation.
		c.zlbDeadline = time.Time{}
	}

	// Set nextRTO to the earliest deadline in flight.
	c.recomputeNextRTO()
	return nil
}

func (c *ControlChannel) recomputeNextRTO() {
	c.nextRTO = time.Time{}
	for i := range c.queue {
		if c.queue[i].attempts == 0 {
			continue
		}
		if c.nextRTO.IsZero() || c.queue[i].deadline.Before(c.nextRTO) {
			c.nextRTO = c.queue[i].deadline
		}
	}
}

// Recv processes the Ns and Nr fields of an inbound control message.
// Returns:
//   - `accept`: true if the message is in-order and should be handed
//     to the FSM. False means the message is a duplicate or out of
//     window and the channel has already handled it (typically by
//     scheduling a ZLB ACK).
//   - `err`: non-nil only on protocol violations.
//
// After a positive Recv the FSM should call Send (if it has a reply)
// or Tick (so the channel can emit a ZLB at zlbDelay).
func (c *ControlChannel) Recv(ns, nr uint16, now time.Time) (accept bool, err error) {
	// Process the peer's Nr: it acknowledges everything strictly
	// less than `nr` from our send sequence. RFC 2661 §5.4: Nr is
	// "the next expected", so Nr-1 is the highest ACKed.
	c.ackThrough(nr, now)

	// Process Ns: must equal c.nr (in-order). Out-of-order or
	// duplicate triggers a ZLB ACK with the current expected Nr.
	if ns != c.nr {
		// Duplicate (ns < c.nr) or future (ns > c.nr): both call for
		// a ZLB carrying the current expected.
		c.scheduleZLB(now)
		return false, nil
	}

	// In-order: advance and arm ZLB unless a piggyback opportunity
	// arrives within zlbDelay.
	c.nr++
	c.scheduleZLB(now)

	return true, nil
}

// ackThrough removes all queued messages with ns < ackNr from the
// queue and grows the congestion window per slow-start rules. The
// retransmit timer is recomputed.
func (c *ControlChannel) ackThrough(ackNr uint16, now time.Time) {
	progressed := false
	for len(c.queue) > 0 {
		head := c.queue[0]
		if head.attempts == 0 {
			break
		}
		// Sequence space wraps at 2^16; compare modulo.
		if seqLess(head.ns, ackNr) {
			c.queue = c.queue[1:]
			progressed = true
			c.growCwndOnAck()
		} else {
			break
		}
	}
	if progressed {
		_ = c.driveSend(now)
	}
}

func (c *ControlChannel) growCwndOnAck() {
	if c.cwnd < c.ssthresh {
		// Slow start: exponential growth.
		c.cwnd++
	} else {
		// Congestion avoidance: linear growth (one MSS per RTT). We
		// approximate by incrementing once per cwnd ACKs.
		c.cwnd++
	}
	if c.cwnd > c.peerWindow {
		c.cwnd = c.peerWindow
	}
}

func (c *ControlChannel) scheduleZLB(now time.Time) {
	c.zlbDeadline = now.Add(c.zlbDelay)
}

// Tick is called by the owning goroutine periodically (or at the
// computed nextRTO / zlbDeadline). Drives retransmissions and ZLB
// emissions. Returns the earliest future timeout the caller should
// sleep until.
func (c *ControlChannel) Tick(now time.Time) time.Time {
	// Retransmits.
	for i := range c.queue {
		if c.queue[i].attempts == 0 {
			continue
		}
		if now.Before(c.queue[i].deadline) {
			continue
		}
		c.queue[i].attempts++
		if c.queue[i].attempts > c.maxRetries {
			if c.dead != nil {
				c.dead()
			}
			// Drop everything; further Sends will return
			// ErrChannelDead via the dead callback's effect on the
			// owning tunnel.
			c.queue = c.queue[:0]
			c.nextRTO = time.Time{}
			return time.Time{}
		}
		// Slow-start retreat per RFC 2661 §5.8.
		c.ssthresh = c.cwnd / 2
		if c.ssthresh < 1 {
			c.ssthresh = 1
		}
		c.cwnd = 1
		// Exponential back-off, capped at rtoMax.
		rto := c.rtoInitial << (c.queue[i].attempts - 1)
		if rto > c.rtoMax {
			rto = c.rtoMax
		}
		c.queue[i].deadline = now.Add(rto)
		_ = c.send(c.queue[i].body, c.queue[i].sessionID, c.queue[i].ns, c.nr)
	}
	c.recomputeNextRTO()

	// ZLB emission. Per RFC 2661 §5.4 a ZLB is a tunnel-level ack and
	// carries Session-ID 0; the Ns is the current next-send value (not
	// incremented because the ZLB has no payload).
	if !c.zlbDeadline.IsZero() && !now.Before(c.zlbDeadline) {
		_ = c.send(nil, 0, c.ns, c.nr)
		c.zlbDeadline = time.Time{}
	}

	// Earliest next event.
	earliest := c.nextRTO
	if !c.zlbDeadline.IsZero() {
		if earliest.IsZero() || c.zlbDeadline.Before(earliest) {
			earliest = c.zlbDeadline
		}
	}
	return earliest
}

// seqLess returns a < b under 16-bit serial-number arithmetic
// (RFC 1982). Used to compare sequence numbers that may wrap.
func seqLess(a, b uint16) bool {
	return uint16(a-b)&0x8000 != 0
}

// Ns returns the next sequence number to be assigned. Used by tests.
func (c *ControlChannel) Ns() uint16 { return c.ns }

// Nr returns the next expected receive sequence number.
func (c *ControlChannel) Nr() uint16 { return c.nr }

// Cwnd returns the current send window size.
func (c *ControlChannel) Cwnd() int { return c.cwnd }

// Ssthresh returns the current slow-start threshold.
func (c *ControlChannel) Ssthresh() int { return c.ssthresh }
