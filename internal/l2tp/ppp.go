// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"encoding/binary"
	"errors"

	pppdisp "github.com/veesix-networks/osvbng/internal/ppp"
	l2tppkt "github.com/veesix-networks/osvbng/pkg/l2tp"
	"github.com/veesix-networks/osvbng/pkg/ppp"
)

// PPP protocol numbers used to discriminate control protocols (which
// run in this Go process via the PPP FSMs) from user IP traffic (which
// the VPP dataplane forwards and the Go control plane never sees).
const (
	pppProtoIPv4   uint16 = 0x0021
	pppProtoIPv6   uint16 = 0x0057
	pppProtoLCP    uint16 = 0xc021
	pppProtoPAP    uint16 = 0xc023
	pppProtoCHAP   uint16 = 0xc223
	pppProtoIPCP   uint16 = 0x8021
	pppProtoIPv6CP uint16 = 0x8057
)

var (
	ErrPPPFrameShort     = errors.New("l2tp: PPP frame shorter than 2-byte protocol field")
	ErrSessionPPPMissing = errors.New("l2tp: session has no PPP dispatcher")
	ErrSendNotConfigured = errors.New("l2tp: no send transport configured")
)

// initSessionPPP allocates the PPP FSMs on a session and wires them
// into a dispatcher backed by sendPPPFrame. Called from HandleICCN
// once the LNS knows the session is up and PPP termination should
// begin.
func (c *Component) initSessionPPP(s *Session) {
	sendCb := func(proto uint16) func(code, id uint8, data []byte) {
		return func(code, id uint8, data []byte) {
			pkt := buildPPPPacket(code, id, data)
			_ = c.sendPPPFrame(s, proto, pkt)
		}
	}

	s.LCP = ppp.NewLCP(ppp.Callbacks{
		Send:      sendCb(pppProtoLCP),
		LayerUp:   func() { c.onLCPUp(s) },
		LayerDown: func() { c.onLCPDown(s) },
	})
	s.LCP.SetAuthProto(ppp.ProtoCHAP, ppp.CHAPMD5)

	s.IPCP = ppp.NewIPCP(ppp.Callbacks{
		Send:      sendCb(pppProtoIPCP),
		LayerUp:   func() { c.onIPCPUp(s) },
		LayerDown: func() { c.onIPCPDown(s) },
	})
	s.IPv6CP = ppp.NewIPv6CP(ppp.Callbacks{
		Send:      sendCb(pppProtoIPv6CP),
		LayerUp:   func() { c.onIPv6CPUp(s) },
		LayerDown: func() { c.onIPv6CPDown(s) },
	})
	s.PAP = &ppp.PAPHandler{Send: sendCb(pppProtoPAP)}
	s.CHAP = &ppp.CHAPHandler{Send: sendCb(pppProtoCHAP)}

	s.PPPDispatcher = &pppdisp.Dispatcher{
		LCP:        s.LCP,
		IPCP:       s.IPCP,
		IPv6CP:     s.IPv6CP,
		PhaseFn:    func() ppp.Phase { return s.Phase },
		HandlePAP:  func(code, id uint8, data []byte) error { return c.handlePAPPacket(s, code, id, data) },
		HandleCHAP: func(code, id uint8, data []byte) error { return c.handleCHAPPacket(s, code, id, data) },
		OnProtocolReject: func(rejected uint16) {
			switch rejected {
			case pppProtoIPCP:
				if s.IPCP != nil {
					s.IPCP.FSM().Close()
				}
			case pppProtoIPv6CP:
				if s.IPv6CP != nil {
					s.IPv6CP.FSM().Close()
				}
			}
		},
	}

	s.mu.Lock()
	s.Phase = ppp.PhaseEstablish
	s.LCP.FSM().Up()
	s.LCP.FSM().Open()
	s.mu.Unlock()
}

// dispatchPPPFrame routes a single inbound PPP frame to the session's
// PPP dispatcher.
//
// `frame` starts at the 2-byte PPP protocol field (the L2TP header was
// stripped before this call). IPv4 and IPv6 payloads are never
// expected here in a VPP-integrated build — the C plugin forwards
// those straight to FIB without punting them. In a kernel-UDP smoke
// test where no VPP is in the loop, IP frames also arrive on this
// path; we drop them because there is no dataplane to forward them
// to. Either way, this function is for PPP *control* protocols only.
func (c *Component) dispatchPPPFrame(s *Session, frame []byte) error {
	if len(frame) < 2 {
		return ErrPPPFrameShort
	}
	proto := binary.BigEndian.Uint16(frame[:2])
	if proto == pppProtoIPv4 || proto == pppProtoIPv6 {
		// Dataplane traffic. Never reachable in a VPP-integrated
		// build because VPP handles IP forwarding before any punt;
		// dropped here only to keep a kernel-UDP smoke test honest.
		return nil
	}
	if s.PPPDispatcher == nil {
		return ErrSessionPPPMissing
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.PPPDispatcher.HandleFrame(proto, frame[2:])
}

// buildPPPPacket frames a `code | id | length | data` PPP packet
// per RFC 1661 §5. The PPP FSM hands the inner `data` (already in the
// configure-option / echo / terminate shape) and the code/id pair; we
// prepend the 4-byte fixed header.
func buildPPPPacket(code, id uint8, data []byte) []byte {
	out := make([]byte, 4+len(data))
	out[0] = code
	out[1] = id
	binary.BigEndian.PutUint16(out[2:4], uint16(4+len(data)))
	copy(out[4:], data)
	return out
}

// sendPPPFrame builds an L2TPv2 data message carrying the supplied
// PPP packet and ships it via the component's configured transport.
// The data flag is T=0; Ns/Nr are omitted per RFC 2661 §3.1 because
// data messages are unreliable. `pppPacket` is the full PPP packet
// from buildPPPPacket (code | id | length | data).
func (c *Component) sendPPPFrame(s *Session, proto uint16, pppPacket []byte) error {
	if c.send == nil {
		return ErrSendNotConfigured
	}
	body := make([]byte, 0, 2+len(pppPacket))
	body = binary.BigEndian.AppendUint16(body, proto)
	body = append(body, pppPacket...)

	t := s.Tunnel
	h := l2tppkt.NewData(t.PeerID, s.PeerID)
	return c.send(t.LocalIP, t.PeerIP, t.LocalPort, t.PeerPort, *h, body)
}
