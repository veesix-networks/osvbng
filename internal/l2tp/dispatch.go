// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"errors"
	"fmt"
	"time"

	"github.com/veesix-networks/osvbng/pkg/dataplane"
	l2tppkt "github.com/veesix-networks/osvbng/pkg/l2tp"
)

// Dispatch is the entry point for inbound L2TPv2 control frames from
// the punt channel. It parses the L2TPv2 header and AVPs, looks up the
// matching tunnel (or treats the frame as an SCCRQ for a new tunnel),
// and routes to the appropriate handler.
//
// The caller (dataplane component) supplies a fully-parsed packet with
// IPv4 and UDP layers populated; the L2TPv2 payload lives in
// `pkt.UDP.Payload`.
func (c *Component) Dispatch(pkt *dataplane.ParsedPacket) error {
	if pkt == nil || pkt.UDP == nil || pkt.IPv4 == nil {
		return ErrPuntPacketShape
	}
	body := pkt.UDP.Payload
	if l2tppkt.IsL2TPv3(body) {
		// v3 detect-and-reject is handled by the punt plugin's data
		// path; control-plane v3 SCCRQ from the wire is rare. Emit a
		// StopCCN with version-unsupported per RFC 2661 §4.4.2.
		return c.respondV3Unsupported(pkt)
	}

	h, payload, err := l2tppkt.Parse(body)
	if err != nil {
		return fmt.Errorf("l2tp parse: %w", err)
	}
	if h.Version != l2tppkt.Version2 {
		return ErrUnsupportedVersion
	}
	if !h.IsControl {
		// T=0 data messages reaching userspace carry PPP control
		// protocols (LCP / CHAP / IPCP / IPv6CP / Echo). User IP
		// traffic is intercepted by the VPP dataplane and never
		// punted. Look up the session and feed the PPP frame to the
		// session's dispatcher.
		s := c.LookupSession(pkt.IPv4.SrcIP, h.TunnelID, h.SessionID)
		if s == nil {
			return ErrNoSuchSession
		}
		return c.dispatchPPPFrame(s, payload)
	}

	avps, err := l2tppkt.ParseAVPs(payload)
	if err != nil {
		return fmt.Errorf("l2tp parse avps: %w", err)
	}
	msgType := l2tppkt.DecodeMessageType(avps)

	// SCCRQ is the only message type that arrives before a tunnel
	// exists locally. Everything else must resolve to an existing
	// tunnel by (peer_ip, local_tunnel_id).
	if msgType == l2tppkt.MsgTypeSCCRQ {
		return c.dispatchSCCRQ(pkt, avps)
	}

	t := c.LookupTunnel(pkt.IPv4.SrcIP, h.TunnelID)
	if t == nil {
		return ErrNoSuchTunnel
	}

	// Advance the control channel's ACK state from the inbound Ns/Nr
	// before processing the message. Without this the send window stays
	// closed and outbound replies (SCCCN, ICCN, …) sit in the queue
	// behind the now-acknowledged previous send.
	if t.Channel != nil {
		accept, err := t.Channel.Recv(h.Ns, h.Nr, time.Now())
		if err != nil {
			return err
		}
		if !accept {
			// Duplicate or out-of-window — channel has already ACKed.
			return nil
		}
	}

	switch msgType {
	case l2tppkt.MsgTypeSCCRP:
		return c.handleSCCRP(t, avps)
	case l2tppkt.MsgTypeICRP:
		s := c.LookupSession(pkt.IPv4.SrcIP, h.TunnelID, h.SessionID)
		if s == nil {
			return ErrNoSuchSession
		}
		return c.handleICRP(s, avps)
	case l2tppkt.MsgTypeSCCCN:
		t.mu.Lock()
		oc := t.outstandingChallenge
		t.mu.Unlock()
		return c.HandleSCCCN(t, avps, oc)
	case l2tppkt.MsgTypeHello:
		// Hello has no body beyond Message Type; the control channel
		// already extracted Ns/Nr and ACKed it. Nothing further.
		return nil
	case l2tppkt.MsgTypeStopCCN:
		c.HandleStopCCN(t)
		return nil
	case l2tppkt.MsgTypeICRQ:
		_, _, err := c.HandleICRQ(t, avps)
		return err
	case l2tppkt.MsgTypeICCN:
		s := c.LookupSession(pkt.IPv4.SrcIP, h.TunnelID, h.SessionID)
		if s == nil {
			return ErrNoSuchSession
		}
		return c.HandleICCN(s, avps)
	case l2tppkt.MsgTypeCDN:
		s := c.LookupSession(pkt.IPv4.SrcIP, h.TunnelID, h.SessionID)
		if s == nil {
			return ErrNoSuchSession
		}
		c.HandleCDN(s)
		return nil
	}
	return ErrUnsupportedMessageType
}

func (c *Component) dispatchSCCRQ(pkt *dataplane.ParsedPacket, avps []l2tppkt.AVP) error {
	// Look up the peer policy by Host Name AVP. Without a matching
	// policy we reject with StopCCN ResultCode=4 (unauthorized).
	hostAVP := l2tppkt.FindFirst(avps, 0, l2tppkt.AVPHostName)
	if hostAVP == nil {
		return ErrMissingHostName
	}
	_ = l2tppkt.DecodeString(hostAVP)

	// In a fully-wired component the LNSConfig comes from
	// pkg/config/l2tp via the peer-policies map. The cmd-level wiring
	// supplies this resolution callback; absent it we cannot proceed.
	if c.resolveLNSConfig == nil {
		return ErrLNSConfigUnresolved
	}
	cfg, ok := c.resolveLNSConfig(l2tppkt.DecodeString(hostAVP))
	if !ok {
		return ErrLACNotAuthorized
	}

	_, _, err := c.HandleSCCRQ(pkt.IPv4.DstIP, pkt.IPv4.SrcIP, avps, cfg)
	return err
}

// respondV3Unsupported is a stub for the rare case of a v3 control
// frame surfacing at this layer. The dispatcher returns the StopCCN
// bytes; the caller (transmission layer) is responsible for sending.
func (c *Component) respondV3Unsupported(pkt *dataplane.ParsedPacket) error {
	_ = pkt
	return ErrV3Unsupported
}

var (
	ErrPuntPacketShape       = errors.New("l2tp: punt packet missing IPv4/UDP layers")
	ErrUnsupportedVersion    = errors.New("l2tp: unsupported version field")
	ErrDataAtControlPath     = errors.New("l2tp: data packet at control path")
	ErrNoSuchTunnel          = errors.New("l2tp: no tunnel for inbound packet")
	ErrNoSuchSession         = errors.New("l2tp: no session for inbound packet")
	ErrUnsupportedMessageType = errors.New("l2tp: unsupported control message type")
	ErrLNSConfigUnresolved   = errors.New("l2tp: no LNS config resolver registered")
	ErrLACNotAuthorized      = errors.New("l2tp: LAC hostname not in peer-policies")
	ErrV3Unsupported         = errors.New("l2tp: v3 control frame rejected")
)
