// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

// Package pppdisp routes inbound PPP frames to LCP/IPCP/IPv6CP FSMs
// and host-specific auth/echo callbacks. Reused by PPPoE termination
// and L2TP LNS — both run the same PPP state machines, only the
// transport that delivers PPP frames differs.
//
// The package name is `pppdisp` rather than `ppp` to avoid a collision
// with `github.com/veesix-networks/osvbng/pkg/ppp` (the FSM library).
package pppdisp

import (
	"encoding/binary"
	"errors"

	"github.com/veesix-networks/osvbng/pkg/ppp"
)

var (
	ErrFrameShort           = errors.New("ppp dispatcher: frame shorter than 4 header bytes")
	ErrFrameLengthMismatch  = errors.New("ppp dispatcher: declared length exceeds payload")
)

// Dispatcher holds references to the per-session FSMs and a set of
// host-specific callbacks. It owns no state itself — all configuration
// and lifecycle live in the host, which constructs a Dispatcher in
// its `initPPP` and calls `HandleFrame` from its receive path.
//
// All callbacks may be nil; a nil callback for an inbound PPP packet
// type means "ignore". The default fallback for an unknown protocol
// is `SendProtocolReject`, which, if nil, results in a silent drop.
type Dispatcher struct {
	LCP    *ppp.LCP
	IPCP   *ppp.IPCP
	IPv6CP *ppp.IPv6CP

	// PhaseFn returns the host's current PPP phase. Used to gate
	// IPCP / IPv6CP packets per RFC 1661 §3.4 — they are processed
	// only in PhaseNetwork or PhaseOpen.
	PhaseFn func() ppp.Phase

	// Auth packet handlers. The host integrates with AAA so the
	// dispatcher does not interpret the codes itself.
	HandlePAP  func(code, id uint8, data []byte) error
	HandleCHAP func(code, id uint8, data []byte) error

	// LCP echo handlers. Echo timing and dead-peer detection live in
	// the host (typically a shared `pkg/ppp.TimeWheel`).
	OnEchoReq func(id uint8, data []byte)
	OnEchoRep func(id uint8, data []byte)

	// OnProtocolReject is called when the peer sends LCP
	// Protocol-Reject for a protocol we previously sent. The host
	// typically closes the rejected protocol's FSM (RFC 1661 §5.7).
	OnProtocolReject func(rejectedProto uint16)

	// SendProtocolReject is called when an unknown PPP protocol
	// arrives. The host typically wraps the original frame and sends
	// it back as LCP Protocol-Reject.
	SendProtocolReject func(proto uint16, payload []byte)
}

// HandleFrame parses a PPP frame and routes it. `proto` is the PPP
// protocol number from the transport (PPPoE: pppoe.ppp_proto; L2TP:
// the PPP protocol field inside the L2TP payload). `payload` is the
// PPP Information field starting with the code+id+length header.
//
// The host's session lock must be held for the duration of the call —
// the FSM Input methods and the host callbacks all read/write
// session state.
func (d *Dispatcher) HandleFrame(proto uint16, payload []byte) error {
	if len(payload) < 4 {
		return ErrFrameShort
	}
	code := payload[0]
	id := payload[1]
	length := binary.BigEndian.Uint16(payload[2:4])
	if int(length) > len(payload) {
		return ErrFrameLengthMismatch
	}
	data := payload[4:length]

	switch proto {
	case ppp.ProtoLCP:
		return d.handleLCP(code, id, data)
	case ppp.ProtoPAP:
		if d.HandlePAP != nil {
			return d.HandlePAP(code, id, data)
		}
	case ppp.ProtoCHAP:
		if d.HandleCHAP != nil {
			return d.HandleCHAP(code, id, data)
		}
	case ppp.ProtoIPCP:
		if d.IPCP == nil || !d.inNetworkPhase() {
			return nil
		}
		d.IPCP.FSM().Input(code, id, data)
	case ppp.ProtoIPv6CP:
		if d.IPv6CP == nil || !d.inNetworkPhase() {
			return nil
		}
		d.IPv6CP.FSM().Input(code, id, data)
	default:
		if d.SendProtocolReject != nil {
			d.SendProtocolReject(proto, payload)
		}
	}
	return nil
}

func (d *Dispatcher) handleLCP(code, id uint8, data []byte) error {
	switch code {
	case ppp.EchoReq:
		if d.OnEchoReq != nil {
			d.OnEchoReq(id, data)
		}
	case ppp.EchoRep:
		if d.OnEchoRep != nil {
			d.OnEchoRep(id, data)
		}
	case ppp.ProtoRej:
		if len(data) >= 2 && d.OnProtocolReject != nil {
			d.OnProtocolReject(binary.BigEndian.Uint16(data[0:2]))
		}
	default:
		if d.LCP != nil {
			d.LCP.FSM().Input(code, id, data)
		}
	}
	return nil
}

func (d *Dispatcher) inNetworkPhase() bool {
	if d.PhaseFn == nil {
		return true
	}
	p := d.PhaseFn()
	return p == ppp.PhaseNetwork || p == ppp.PhaseOpen
}
