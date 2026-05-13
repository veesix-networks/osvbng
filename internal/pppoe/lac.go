// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package pppoe

import (
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/ppp"
)

// LACBringUpAttrs is the PPPoE-side handoff payload passed to the
// LACTrigger callback when a subscriber with `access-type: lac` is
// authenticated via AAA and the reply carries Tunnel-* attributes.
//
// All fields are primitive types so PPPoE does not import the L2TP
// component (avoiding an import cycle). The cmd-layer wiring translates
// these into the L2TP component's own request shape.
type LACBringUpAttrs struct {
	PPPoESessionID uint16
	Username       string

	// AAAAttrs is the subset of the AAA reply attribute bag the LAC
	// needs to parse out tunnel candidates (tunnel.type,
	// tunnel.server-endpoint, tunnel.password, tunnel.preference,
	// tunnel.assignment-id, tunnel.client-endpoint, …). Stringly typed
	// to match the L2TP-side ParseTunnelSpecs signature.
	AAAAttrs map[string]string

	// Proxy auth material to replay into L2TPv2 ICCN per RFC 3437.
	ProxyAuthenType      uint16
	ProxyAuthenName      string
	ProxyAuthenChallenge []byte
	ProxyAuthenResponse  []byte
}

// LACTrigger is the function PPPoE invokes to ask the L2TP component
// to bring up a LAC tunnel and session. Synchronous errors (no
// candidates, transport not configured) are surfaced immediately; the
// success/failure of the actual tunnel + session bring-up is reported
// asynchronously via TopicL2TPLACDecision.
type LACTrigger func(attrs LACBringUpAttrs) error

// SetLACTrigger installs the LAC bring-up callback. Called once from
// cmd-level wiring after both the PPPoE and L2TP components are
// constructed.
func (c *Component) SetLACTrigger(fn LACTrigger) { c.lacTrigger = fn }

// handleLACDecision is subscribed to TopicL2TPLACDecision. It looks up
// the PPPoE session by PPPoESessionID and either completes the local
// PAP/CHAP-Ack and transitions the session into PhaseLACTunneled (on
// success) or sends PAP/CHAP-Nak and terminates the session (on
// failure).
func (c *Component) handleLACDecision(event events.Event) {
	data, ok := event.Data.(*events.L2TPLACDecisionEvent)
	if !ok {
		return
	}
	c.sessionMu.RLock()
	sess := c.sidIndex[data.PPPoESessionID]
	c.sessionMu.RUnlock()
	if sess == nil {
		return
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	if sess.Phase != ppp.PhaseLACTunnelPending {
		c.logger.Debug("LAC decision for session not in pending phase",
			"session_id", sess.SessionID, "phase", sess.Phase.String())
		return
	}

	if data.Success {
		c.logger.Info("LAC tunnel bring-up succeeded",
			"session_id", sess.SessionID,
			"peer_ip", data.PeerIP,
			"local_tunnel_id", data.LocalTunnelID,
			"local_session_id", data.LocalSessionID)

		switch sess.pendingAuthType {
		case "pap":
			sess.sendPAPAck(sess.pendingPAPID)
		case "chap":
			sess.sendCHAPSuccess(sess.pendingCHAPID)
		}
		sess.Phase = ppp.PhaseLACTunneled
		sess.pendingAuthType = ""
		sess.pendingAuthRequestID = ""
		// Programming the `is_lac_tunneled` flag + L2TP opaque on the
		// PPPoE session struct in VPP is the dataplane bridge wire-up
		// owned by the southbound. The southbound API for that bridge
		// is not yet defined; once it lands, this call site is where
		// it slots in.
		return
	}

	c.logger.Warn("LAC tunnel bring-up failed",
		"session_id", sess.SessionID, "error", data.Error)
	switch sess.pendingAuthType {
	case "pap":
		sess.sendPAPNak(sess.pendingPAPID)
	case "chap":
		sess.sendCHAPFailure(sess.pendingCHAPID)
	}
	sess.pendingAuthType = ""
	sess.pendingAuthRequestID = ""
	sess.lcp.FSM().Close()
}

// shouldTunnelToLAC returns true if the AAA reply triggers LAC mode:
// the subscriber-group must be configured with `access-type: lac` AND
// the reply must carry Tunnel-Type=L2TP. Either condition alone falls
// back to the regular local-termination PPPoE path. Called with s.mu
// held.
func (s *SessionState) shouldTunnelToLAC() bool {
	if s.component.lacTrigger == nil {
		return false
	}
	cfg, err := s.component.cfgMgr.GetRunning()
	if err != nil || cfg == nil || cfg.SubscriberGroups == nil {
		return false
	}
	group, _ := cfg.SubscriberGroups.FindGroupBySVLAN(s.OuterVLAN)
	if group == nil || group.AccessType != "lac" {
		return false
	}
	// AAA must say "tunnel.type = L2TP" on at least one tagged entry.
	for k, v := range s.Attributes {
		base := k
		if i := indexByte(k, ':'); i >= 0 {
			base = k[:i]
		}
		if base == "tunnel.type" && equalFold(v, "L2TP") {
			return true
		}
	}
	return false
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// buildLACBringUpAttrs assembles the handoff payload from a PPPoE
// session whose AAA reply carried Tunnel-* attributes. Called with
// s.mu held — the function reads stable session fields plus
// already-extracted AAA attrs.
func buildLACBringUpAttrs(s *SessionState) LACBringUpAttrs {
	attrs := LACBringUpAttrs{
		PPPoESessionID: s.PPPoESessionID,
		Username:       s.Username,
		AAAAttrs:       make(map[string]string, len(s.Attributes)),
	}
	for k, v := range s.Attributes {
		attrs.AAAAttrs[k] = v
	}
	if s.pendingAuthType == "chap" {
		attrs.ProxyAuthenType = 2 // RFC 2661 §4.4.30: 2 = CHAP
		attrs.ProxyAuthenName = s.Username
		if len(s.chapChallenge) > 0 {
			attrs.ProxyAuthenChallenge = append([]byte(nil), s.chapChallenge...)
		}
		if len(s.chapResponse) > 0 {
			attrs.ProxyAuthenResponse = append([]byte(nil), s.chapResponse...)
		}
	}
	return attrs
}
