// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"errors"
	"testing"
	"time"
)

type sentMsg struct {
	body []byte
	ns   uint16
	nr   uint16
}

func recordingSend(out *[]sentMsg) SendFunc {
	return func(body []byte, ns, nr uint16) error {
		*out = append(*out, sentMsg{body: append([]byte(nil), body...), ns: ns, nr: nr})
		return nil
	}
}

func TestControlChannelSendAssignsNs(t *testing.T) {
	// Per RFC 2661 §5.8 slow-start starts with cwnd=1, so only the
	// first message goes out immediately; the second queues behind
	// until the first is ACKed. We verify that both messages get
	// distinct Ns values regardless and that exactly one was sent.
	var sent []sentMsg
	ch := NewControlChannel(Config{PeerRWS: 4}, recordingSend(&sent), nil)

	now := time.Unix(0, 0)
	if err := ch.Send([]byte("m1"), now); err != nil {
		t.Fatal(err)
	}
	if err := ch.Send([]byte("m2"), now); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 {
		t.Fatalf("slow-start cwnd=1: want 1 send, got %d", len(sent))
	}
	if sent[0].ns != 0 {
		t.Fatalf("first send Ns must be 0, got %d", sent[0].ns)
	}
	if ch.Ns() != 2 {
		t.Fatalf("next Ns should be 2 after assigning 0 and 1, got %d", ch.Ns())
	}
}

func TestControlChannelCwndLimitsInflight(t *testing.T) {
	var sent []sentMsg
	ch := NewControlChannel(Config{PeerRWS: 4}, recordingSend(&sent), nil)
	ch.cwnd = 1 // force minimum window

	now := time.Unix(0, 0)
	_ = ch.Send([]byte("m1"), now)
	_ = ch.Send([]byte("m2"), now)
	_ = ch.Send([]byte("m3"), now)

	if len(sent) != 1 {
		t.Fatalf("cwnd=1 must hold back queued messages, sent=%d", len(sent))
	}
}

func TestControlChannelRecvAdvancesNr(t *testing.T) {
	var sent []sentMsg
	ch := NewControlChannel(Config{PeerRWS: 4}, recordingSend(&sent), nil)
	now := time.Unix(0, 0)

	accept, err := ch.Recv(0, 0, now)
	if err != nil || !accept {
		t.Fatalf("first recv: accept=%v err=%v", accept, err)
	}
	if ch.Nr() != 1 {
		t.Fatalf("Nr should be 1 after first recv, got %d", ch.Nr())
	}

	// Duplicate (same ns) should not advance.
	accept, _ = ch.Recv(0, 0, now)
	if accept {
		t.Fatal("duplicate should be rejected")
	}
	if ch.Nr() != 1 {
		t.Fatalf("Nr changed on duplicate")
	}
}

func TestControlChannelAckRemovesFromQueue(t *testing.T) {
	var sent []sentMsg
	ch := NewControlChannel(Config{PeerRWS: 4}, recordingSend(&sent), nil)
	now := time.Unix(0, 0)

	_ = ch.Send([]byte("m1"), now)
	if len(ch.queue) != 1 {
		t.Fatalf("want 1 queued (cwnd=1 limits), got %d", len(ch.queue))
	}

	// Peer ACKs m1 with Nr=1 (next-expected). m1 (ns=0) leaves the queue.
	accept, _ := ch.Recv(0, 1, now)
	if !accept {
		t.Fatal("recv with Nr=1 should be accepted")
	}
	if len(ch.queue) != 0 {
		t.Fatalf("queue should be empty after Nr=1 ACKs m1, got %d", len(ch.queue))
	}
	// And cwnd should have grown on the ACK.
	if ch.Cwnd() != 2 {
		t.Fatalf("cwnd should grow to 2 after first ACK in slow-start, got %d", ch.Cwnd())
	}
}

func TestControlChannelZLBOnInactivity(t *testing.T) {
	var sent []sentMsg
	ch := NewControlChannel(Config{PeerRWS: 4, ZLBDelay: 10 * time.Millisecond},
		recordingSend(&sent), nil)
	now := time.Unix(0, 0)

	// Inbound message creates an ACK obligation.
	_, _ = ch.Recv(0, 0, now)
	if len(sent) != 0 {
		t.Fatal("Recv should not directly emit")
	}

	// Tick after zlbDelay should fire a ZLB.
	ch.Tick(now.Add(50 * time.Millisecond))
	if len(sent) != 1 {
		t.Fatalf("want 1 ZLB sent, got %d", len(sent))
	}
	if sent[0].nr != 1 {
		t.Fatalf("ZLB should carry Nr=1, got %d", sent[0].nr)
	}
	if len(sent[0].body) != 0 {
		t.Fatalf("ZLB body must be empty, got %d bytes", len(sent[0].body))
	}
}

func TestControlChannelRetransmitAndBackoff(t *testing.T) {
	var sent []sentMsg
	ch := NewControlChannel(Config{
		PeerRWS:    4,
		RTOInitial: 10 * time.Millisecond,
		RTOMax:     80 * time.Millisecond,
		MaxRetries: 3,
	}, recordingSend(&sent), nil)

	now := time.Unix(0, 0)
	_ = ch.Send([]byte("m"), now)
	if len(sent) != 1 {
		t.Fatal("expected initial send")
	}

	// Tick past first RTO triggers retransmit.
	ch.Tick(now.Add(20 * time.Millisecond))
	if len(sent) != 2 {
		t.Fatalf("expected retransmit, got %d sends", len(sent))
	}

	// cwnd should drop to 1, ssthresh halved.
	if ch.Cwnd() != 1 {
		t.Fatalf("cwnd should reset to 1 on retransmit, got %d", ch.Cwnd())
	}
}

func TestControlChannelMaxRetriesDeclaresDead(t *testing.T) {
	var sent []sentMsg
	var died bool
	ch := NewControlChannel(Config{
		PeerRWS:    4,
		RTOInitial: 1 * time.Millisecond,
		RTOMax:     10 * time.Millisecond,
		MaxRetries: 2,
	}, recordingSend(&sent), func() { died = true })

	now := time.Unix(0, 0)
	_ = ch.Send([]byte("m"), now)

	// Advance through 3 retransmit ticks (initial send + 2 retries
	// = MaxRetries 2 → exceeds on attempt 3 → dead).
	for step := 1; step <= 5; step++ {
		ch.Tick(now.Add(time.Duration(step) * 50 * time.Millisecond))
	}
	if !died {
		t.Fatal("expected dead callback after exhausting retries")
	}
}

func TestSeqLessWraparound(t *testing.T) {
	if !seqLess(0xfffe, 0x0001) {
		t.Fatal("0xfffe should be < 0x0001 under serial arithmetic")
	}
	if seqLess(0x0001, 0xfffe) {
		t.Fatal("0x0001 should not be < 0xfffe")
	}
	if seqLess(5, 5) {
		t.Fatal("equal should not be less")
	}
}

func TestSetPeerWindowClamps(t *testing.T) {
	ch := NewControlChannel(Config{PeerRWS: 8}, func(_ []byte, _, _ uint16) error { return nil }, nil)
	ch.cwnd = 16
	ch.ssthresh = 16

	ch.SetPeerWindow(4)
	if ch.peerWindow != 4 {
		t.Fatalf("peerWindow not updated: %d", ch.peerWindow)
	}
	if ch.cwnd != 4 {
		t.Fatalf("cwnd not clamped: %d", ch.cwnd)
	}
	if ch.Ssthresh() != 4 {
		t.Fatalf("ssthresh not clamped: %d", ch.Ssthresh())
	}

	// Zero / negative should snap to 1.
	ch.SetPeerWindow(0)
	if ch.peerWindow != 1 {
		t.Fatalf("zero RWS should snap to 1, got %d", ch.peerWindow)
	}
}

func TestSendErrorPropagates(t *testing.T) {
	want := errors.New("net broken")
	ch := NewControlChannel(Config{PeerRWS: 4}, func(_ []byte, _, _ uint16) error {
		return want
	}, nil)
	if err := ch.Send([]byte("x"), time.Unix(0, 0)); err != want {
		t.Fatalf("want propagated error, got %v", err)
	}
}
