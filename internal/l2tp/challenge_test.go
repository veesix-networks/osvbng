// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"encoding/binary"
	"net"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/logger"
	l2tppkt "github.com/veesix-networks/osvbng/pkg/l2tp"
)

// be16 returns a 2-byte big-endian encoding for AVP values that hold a
// uint16 (Message Type, Assigned-Tunnel-ID, etc.).
func be16(v uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, v)
	return b
}

// sccrqAVPs builds the minimal SCCRQ AVP set HandleSCCRQ requires:
// Message Type, Host Name, Assigned Tunnel-ID, plus an optional peer
// Challenge.
func sccrqAVPs(hostname string, peerTunnelID uint16, peerChallenge []byte) []l2tppkt.AVP {
	out := []l2tppkt.AVP{
		{Mandatory: true, VendorID: 0, Type: l2tppkt.AVPMessageType, Value: be16(l2tppkt.MsgTypeSCCRQ)},
		{Mandatory: true, VendorID: 0, Type: l2tppkt.AVPHostName, Value: []byte(hostname)},
		{Mandatory: true, VendorID: 0, Type: l2tppkt.AVPAssignedTunnelID, Value: be16(peerTunnelID)},
	}
	if peerChallenge != nil {
		out = append(out, l2tppkt.AVP{
			Mandatory: true, VendorID: 0, Type: l2tppkt.AVPChallenge, Value: peerChallenge,
		})
	}
	return out
}

func TestHandleSCCRQEmitsOwnChallenge(t *testing.T) {
	c := New(logger.Get("l2tp"))
	cfg := LNSConfig{
		LocalHostname:     "lns",
		ReceiveWindowSize: 16,
		Secret:            []byte("s3cret"),
		ChallengeRequired: true,
	}

	body, tunnel, err := c.HandleSCCRQ(net.IPv4(10, 0, 0, 1), net.IPv4(10, 0, 0, 2), sccrqAVPs("lac", 99, make([]byte, l2tppkt.ChallengeLen)), cfg)
	if err != nil {
		t.Fatalf("HandleSCCRQ: %v", err)
	}
	if tunnel.outstandingChallenge == nil {
		t.Fatal("expected outstandingChallenge to be set when ChallengeRequired=true")
	}
	if len(tunnel.outstandingChallenge) != l2tppkt.ChallengeLen {
		t.Fatalf("challenge len = %d, want %d", len(tunnel.outstandingChallenge), l2tppkt.ChallengeLen)
	}

	// SCCRP body must carry an AVP-encoded Challenge matching what we
	// stored on the tunnel.
	avps, err := l2tppkt.ParseAVPs(body)
	if err != nil {
		t.Fatalf("ParseAVPs(sccrp): %v", err)
	}
	ch := l2tppkt.FindFirst(avps, 0, l2tppkt.AVPChallenge)
	if ch == nil {
		t.Fatal("SCCRP body missing Challenge AVP")
	}
	if string(ch.Value) != string(tunnel.outstandingChallenge) {
		t.Fatal("SCCRP Challenge AVP value differs from tunnel outstandingChallenge")
	}
}

func TestHandleSCCRQNoChallengeWhenNotRequired(t *testing.T) {
	c := New(logger.Get("l2tp"))
	cfg := LNSConfig{
		LocalHostname:     "lns",
		ReceiveWindowSize: 16,
		Secret:            []byte("s3cret"),
		ChallengeRequired: false,
	}
	_, tunnel, err := c.HandleSCCRQ(net.IPv4(10, 0, 0, 1), net.IPv4(10, 0, 0, 3), sccrqAVPs("lac", 100, nil), cfg)
	if err != nil {
		t.Fatalf("HandleSCCRQ: %v", err)
	}
	if tunnel.outstandingChallenge != nil {
		t.Fatal("expected no outstandingChallenge when ChallengeRequired=false")
	}
}

func TestHandleSCCCNVerifiesPeerResponse(t *testing.T) {
	c := New(logger.Get("l2tp"))
	secret := []byte("s3cret")
	cfg := LNSConfig{
		LocalHostname:     "lns",
		ReceiveWindowSize: 16,
		Secret:            secret,
		ChallengeRequired: true,
	}

	_, tunnel, err := c.HandleSCCRQ(net.IPv4(10, 0, 0, 1), net.IPv4(10, 0, 0, 4), sccrqAVPs("lac", 101, make([]byte, l2tppkt.ChallengeLen)), cfg)
	if err != nil {
		t.Fatalf("HandleSCCRQ: %v", err)
	}
	challenge := tunnel.outstandingChallenge

	goodResp := l2tppkt.ComputeChallengeResponse(byte(l2tppkt.MsgTypeSCCCN), secret, challenge)
	sccccnAVPs := []l2tppkt.AVP{
		{Mandatory: true, VendorID: 0, Type: l2tppkt.AVPMessageType, Value: be16(l2tppkt.MsgTypeSCCCN)},
		{Mandatory: true, VendorID: 0, Type: l2tppkt.AVPChallengeResponse, Value: goodResp},
	}
	if err := c.HandleSCCCN(tunnel, sccccnAVPs, challenge); err != nil {
		t.Fatalf("HandleSCCCN with valid response: %v", err)
	}
	if tunnel.outstandingChallenge != nil {
		t.Fatal("outstandingChallenge must be cleared after successful verify")
	}
}

func TestHandleSCCCNRejectsBadResponse(t *testing.T) {
	c := New(logger.Get("l2tp"))
	cfg := LNSConfig{
		LocalHostname:     "lns",
		ReceiveWindowSize: 16,
		Secret:            []byte("s3cret"),
		ChallengeRequired: true,
	}
	_, tunnel, err := c.HandleSCCRQ(net.IPv4(10, 0, 0, 1), net.IPv4(10, 0, 0, 5), sccrqAVPs("lac", 102, make([]byte, l2tppkt.ChallengeLen)), cfg)
	if err != nil {
		t.Fatalf("HandleSCCRQ: %v", err)
	}
	challenge := tunnel.outstandingChallenge

	badResp := make([]byte, l2tppkt.ResponseLen)
	sccccnAVPs := []l2tppkt.AVP{
		{Mandatory: true, VendorID: 0, Type: l2tppkt.AVPMessageType, Value: be16(l2tppkt.MsgTypeSCCCN)},
		{Mandatory: true, VendorID: 0, Type: l2tppkt.AVPChallengeResponse, Value: badResp},
	}
	if err := c.HandleSCCCN(tunnel, sccccnAVPs, challenge); err != l2tppkt.ErrChallengeBadResponse {
		t.Fatalf("expected ErrChallengeBadResponse, got %v", err)
	}
}

func TestHandleSCCCNRejectsMissingResponse(t *testing.T) {
	c := New(logger.Get("l2tp"))
	cfg := LNSConfig{
		LocalHostname:     "lns",
		ReceiveWindowSize: 16,
		Secret:            []byte("s3cret"),
		ChallengeRequired: true,
	}
	_, tunnel, err := c.HandleSCCRQ(net.IPv4(10, 0, 0, 1), net.IPv4(10, 0, 0, 6), sccrqAVPs("lac", 103, make([]byte, l2tppkt.ChallengeLen)), cfg)
	if err != nil {
		t.Fatalf("HandleSCCRQ: %v", err)
	}
	challenge := tunnel.outstandingChallenge

	sccccnAVPs := []l2tppkt.AVP{
		{Mandatory: true, VendorID: 0, Type: l2tppkt.AVPMessageType, Value: be16(l2tppkt.MsgTypeSCCCN)},
	}
	if err := c.HandleSCCCN(tunnel, sccccnAVPs, challenge); err != ErrMissingChallengeResponse {
		t.Fatalf("expected ErrMissingChallengeResponse, got %v", err)
	}
}
