// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"bytes"
	"net"
	"testing"

	l2tppkt "github.com/veesix-networks/osvbng/pkg/l2tp"
)

// The LNS control plane and the dataplane encapsulate frames for the
// same session, so both must use the framing resolved on the tunnel.
func TestSendPPPFrameFollowsSessionFraming(t *testing.T) {
	cases := []struct {
		name    string
		hdrSkip uint8
		want    []byte
	}{
		{"hdlc", 2, []byte{0xff, 0x03, 0xc0, 0x21, 0x01, 0x07, 0x00, 0x04}},
		{"compressed", 0, []byte{0xc0, 0x21, 0x01, 0x07, 0x00, 0x04}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []byte
			var gotHdr l2tppkt.Header
			c := &Component{}
			c.SetSendControlFn(func(_, _ net.IP, _, _ uint16, h l2tppkt.Header, body []byte) error {
				gotHdr = h
				got = body
				return nil
			})
			s := &Session{
				Tunnel: &Tunnel{PeerID: 7, PPPHdrSkip: tc.hdrSkip},
				PeerID: 3,
			}
			if err := c.sendPPPFrame(s, pppProtoLCP, buildPPPPacket(1, 7, nil)); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("body = % x, want % x", got, tc.want)
			}
			if gotHdr.TunnelID != 7 || gotHdr.SessionID != 3 {
				t.Fatalf("header ids = %d/%d, want 7/3", gotHdr.TunnelID, gotHdr.SessionID)
			}
		})
	}
}
