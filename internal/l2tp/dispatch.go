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
	c.log.Debug("l2tp dispatch", "msg_type", msgType, "tunnel_id", h.TunnelID, "src", pkt.IPv4.SrcIP.String(), "avp_count", len(avps))

	if msgType == l2tppkt.MsgTypeSCCRQ {
		return c.dispatchSCCRQ(pkt, h, avps)
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

	// ZLB (no AVPs): pure ack already consumed by Recv above.
	if len(avps) == 0 {
		return nil
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
		icrpBody, s, err := c.HandleICRQ(t, avps)
		if err != nil {
			return err
		}
		if icrpBody != nil && t.Channel != nil && s != nil {
			return t.Channel.SendSession(icrpBody, s.PeerID, time.Now())
		}
		return nil
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

func (c *Component) dispatchSCCRQ(pkt *dataplane.ParsedPacket, h *l2tppkt.Header, avps []l2tppkt.AVP) error {
	hostAVP := l2tppkt.FindFirst(avps, 0, l2tppkt.AVPHostName)
	if hostAVP == nil {
		return ErrMissingHostName
	}
	_ = l2tppkt.DecodeString(hostAVP)

	if c.resolveLNSConfig == nil {
		return ErrLNSConfigUnresolved
	}
	cfg, ok := c.resolveLNSConfig(l2tppkt.DecodeString(hostAVP))
	if !ok {
		return ErrLACNotAuthorized
	}

	sccrpBody, t, err := c.HandleSCCRQ(pkt.IPv4.DstIP, pkt.IPv4.SrcIP, avps, cfg)
	if err != nil {
		return err
	}
	if sccrpBody != nil && t != nil && t.Channel != nil {
		// Register the just-consumed SCCRQ on the freshly-built channel
		// so SCCRP carries Nr=h.Ns+1 and the next inbound (SCCCN at Ns=1)
		// matches c.nr.
		now := time.Now()
		if _, err := t.Channel.Recv(h.Ns, h.Nr, now); err != nil {
			return err
		}
		return t.Channel.Send(sccrpBody, now)
	}
	return nil
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
