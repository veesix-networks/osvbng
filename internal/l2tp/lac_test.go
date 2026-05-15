// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/google/gopacket/layers"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/events/local"
	l2tppkt "github.com/veesix-networks/osvbng/pkg/l2tp"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
)

// captureTransport records outbound control packets so the test can
// assert on the wire-level message sequence.
type captureTransport struct {
	mu      sync.Mutex
	packets []capturedPacket
}

type capturedPacket struct {
	header l2tppkt.Header
	body   []byte
}

func (t *captureTransport) Send(localIP, peerIP net.IP, localPort, peerPort uint16, h l2tppkt.Header, body []byte) error {
	t.mu.Lock()
	cp := capturedPacket{header: h}
	cp.body = append(cp.body, body...)
	t.packets = append(t.packets, cp)
	t.mu.Unlock()
	return nil
}

func (t *captureTransport) snapshot() []capturedPacket {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]capturedPacket, len(t.packets))
	copy(out, t.packets)
	return out
}

func TestStartLACSessionAllDenylisted(t *testing.T) {
	c := New(logger.Get("l2tp"))
	c.SetSendControlFn(func(_, _ net.IP, _, _ uint16, _ l2tppkt.Header, _ []byte) error { return nil })

	peer := net.IPv4(10, 0, 0, 2)
	c.Denylist().Add(peer, "transport-failure", time.Minute)

	err := c.StartLACSession(LACBringUpRequest{
		PPPoESessionID: 1,
		TunnelSpecs:    []TunnelSpec{{ServerIP: peer, Password: "x"}},
	})
	if err != ErrAllCandidatesDenied {
		t.Fatalf("want ErrAllCandidatesDenied, got %v", err)
	}
}

func TestStartLACSessionNoCandidates(t *testing.T) {
	c := New(logger.Get("l2tp"))
	c.SetSendControlFn(func(_, _ net.IP, _, _ uint16, _ l2tppkt.Header, _ []byte) error { return nil })

	err := c.StartLACSession(LACBringUpRequest{PPPoESessionID: 1})
	if err != ErrNoTunnelCandidates {
		t.Fatalf("want ErrNoTunnelCandidates, got %v", err)
	}
}

func TestStartLACSessionEmitsSCCRQ(t *testing.T) {
	cap := &captureTransport{}
	c := New(logger.Get("l2tp"))
	c.SetSendControlFn(cap.Send)
	c.SetLocalHostname("bng1")

	req := LACBringUpRequest{
		PPPoESessionID: 7,
		Username:       "alice",
		LocalIP:        net.IPv4(10, 0, 0, 1),
		TunnelSpecs: []TunnelSpec{
			{ServerIP: net.IPv4(10, 0, 0, 2), Password: "shared"},
		},
	}
	if err := c.StartLACSession(req); err != nil {
		t.Fatalf("StartLACSession: %v", err)
	}

	pkts := cap.snapshot()
	if len(pkts) == 0 {
		t.Fatal("expected SCCRQ on the wire")
	}
	avps, err := l2tppkt.ParseAVPs(pkts[0].body)
	if err != nil {
		t.Fatalf("ParseAVPs: %v", err)
	}
	if mt := l2tppkt.DecodeMessageType(avps); mt != l2tppkt.MsgTypeSCCRQ {
		t.Fatalf("first packet is %d, want SCCRQ (%d)", mt, l2tppkt.MsgTypeSCCRQ)
	}
	host := l2tppkt.FindFirst(avps, 0, l2tppkt.AVPHostName)
	if host == nil || string(host.Value) != "bng1" {
		t.Fatalf("expected Host Name 'bng1', got %v", host)
	}
	ch := l2tppkt.FindFirst(avps, 0, l2tppkt.AVPChallenge)
	if ch == nil {
		t.Fatal("expected Challenge AVP since password was configured")
	}
}

// buildSCCRPBody mints an SCCRP body the LAC dispatch can consume. It
// echoes the LAC's challenge back via a valid Challenge-Response AVP.
func buildSCCRPBody(lnsLocalTunnelID uint16, secret, lacChallenge []byte) []byte {
	resp := l2tppkt.ComputeChallengeResponse(byte(l2tppkt.MsgTypeSCCRP), secret, lacChallenge)
	return l2tppkt.BuildSCCRP(l2tppkt.SCCRPParams{
		LocalTunnelID:     lnsLocalTunnelID,
		ReceiveWindowSize: 16,
		HostName:          "lns",
		FramingCaps:       l2tppkt.FramingSync,
		BearerCaps:        l2tppkt.BearerDigital,
		ChallengeResponse: resp,
	})
}

func TestLACSCCRPTriggersSCCCNAndICRQ(t *testing.T) {
	cap := &captureTransport{}
	bus := local.NewBus()
	defer func() { _ = bus.Close() }()
	c := New(logger.Get("l2tp"))
	c.SetSendControlFn(cap.Send)
	c.SetLocalHostname("bng1")
	c.SetEventBus(bus)
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = c.Stop(context.Background()) }()

	if err := c.StartLACSession(LACBringUpRequest{
		PPPoESessionID: 7,
		LocalIP:        net.IPv4(10, 0, 0, 1),
		TunnelSpecs: []TunnelSpec{
			{ServerIP: net.IPv4(10, 0, 0, 2), Password: "shared"},
		},
	}); err != nil {
		t.Fatalf("StartLACSession: %v", err)
	}

	// Pluck the tunnel out of the lookup map and feed an SCCRP via the
	// public handler. The tunnel was registered with LocalID allocated
	// from the per-peer allocator (deterministically the first ID).
	tnl := c.LookupTunnel(net.IPv4(10, 0, 0, 2), 1)
	if tnl == nil {
		t.Fatal("LAC tunnel not registered with LocalID=1")
	}

	sccrpBody := buildSCCRPBody(99, tnl.Secret, tnl.outstandingChallenge)

	// Build an L2TP control header that ACKs our SCCRQ (Nr=1) and
	// carries Ns=0 (the LNS's first message). Wrap in IPv4+UDP and
	// route through Dispatch so the control-channel ACK state updates.
	hdr := l2tppkt.NewControl(tnl.LocalID, 0, 0, 1)
	wire := hdr.AppendTo(make([]byte, 0, 12+len(sccrpBody)), len(sccrpBody))
	wire = append(wire, sccrpBody...)

	pkt := &dataplane.ParsedPacket{
		Protocol: models.ProtocolL2TP,
		IPv4: &layers.IPv4{
			SrcIP: net.IPv4(10, 0, 0, 2).To4(),
			DstIP: net.IPv4(10, 0, 0, 1).To4(),
		},
		UDP: &layers.UDP{
			SrcPort: 1701,
			DstPort: 1701,
		},
	}
	pkt.UDP.Payload = wire
	if err := c.Dispatch(pkt); err != nil {
		t.Fatalf("Dispatch sccrp: %v", err)
	}

	// We expect: SCCRQ (already captured) → SCCCN → ICRQ
	pkts := cap.snapshot()
	if len(pkts) < 3 {
		t.Fatalf("expected at least 3 packets (SCCRQ, SCCCN, ICRQ); got %d", len(pkts))
	}
	check := func(idx int, want uint16, label string) {
		a, err := l2tppkt.ParseAVPs(pkts[idx].body)
		if err != nil {
			t.Fatalf("ParseAVPs[%d]: %v", idx, err)
		}
		if got := l2tppkt.DecodeMessageType(a); got != want {
			t.Fatalf("packet[%d] message type = %d, want %s (%d)", idx, got, label, want)
		}
	}
	check(0, l2tppkt.MsgTypeSCCRQ, "SCCRQ")
	check(1, l2tppkt.MsgTypeSCCCN, "SCCCN")
	check(2, l2tppkt.MsgTypeICRQ, "ICRQ")

	if tnl.PeerID != 99 {
		t.Fatalf("PeerID = %d, want 99 (from SCCRP)", tnl.PeerID)
	}

	// Allow the bus a moment in case the test runner is loaded.
	time.Sleep(10 * time.Millisecond)
}
