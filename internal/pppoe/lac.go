// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package pppoe

import (
	"net"
	"time"

	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/ppp"
	"github.com/veesix-networks/osvbng/pkg/southbound"
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

	// PPPoESwIfIndex is the subscriber's PPPoE session sw_if_index in
	// VPP — used by the L2TP plugin as the opaque it stashes in
	// vnet_buffer for the LNS→subscriber bridge.
	PPPoESwIfIndex uint32

	// EncapIfIndex is the TX interface for outbound L2TP packets;
	// ~0 lets VPP resolve via FIB on the LNS peer IP.
	EncapIfIndex uint32

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

// LACSessionIndexResolver maps a persisted L2TP (localTunnelID,
// localSessionID) pair back to the current dataplane sw_if_index of
// the L2TP session interface. Used by setupSessionRestore to replay
// SetPPPoESessionLACTunneled across L2TP component re-init without
// persisting the volatile sw_if_index in the PPPoE checkpoint.
type LACSessionIndexResolver func(localTunnelID, localSessionID uint16) (uint32, bool)

// SetLACResolver installs the L2TP-side sw_if_index resolver. Called
// once from cmd-level wiring after the L2TP component is constructed.
// nil leaves restored LAC sessions in PhaseLACTunnelPending until the
// L2TP tunnel is back up; their opdb entries persist and forwarding
// stays down (the PPPoE plugin's locally-decap-with-no-IP path drops
// subscriber traffic at ip4-not-enabled — preferable to silently
// forwarding wrong-class traffic into the local datapath).
func (c *Component) SetLACResolver(fn LACSessionIndexResolver) { c.lacResolver = fn }

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
			"local_session_id", data.LocalSessionID,
			"l2tp_session_index", data.LACL2TPSessionIndex)

		if c.vpp != nil && sess.SwIfIndex != 0 {
			if err := c.vpp.SetPPPoESessionLACTunneled(
				sess.SwIfIndex, data.LACL2TPSessionIndex, true,
			); err != nil {
				c.logger.Error("SetPPPoESessionLACTunneled failed; rejecting subscriber",
					"session_id", sess.SessionID, "error", err)
				switch sess.pendingAuthType {
				case "pap":
					sess.sendPAPNak(sess.pendingPAPID)
				case "chap":
					sess.sendCHAPFailure(sess.pendingCHAPID)
				}
				sess.pendingAuthType = ""
				sess.pendingAuthRequestID = ""
				sess.lcp.FSM().Close()
				return
			}
		}

		switch sess.pendingAuthType {
		case "pap":
			sess.sendPAPAck(sess.pendingPAPID)
		case "chap":
			sess.sendCHAPSuccess(sess.pendingCHAPID)
		}
		sess.Phase = ppp.PhaseLACTunneled
		sess.BoundAt = time.Now()
		sess.pendingAuthType = ""
		sess.pendingAuthRequestID = ""

		var tunneledTo net.IP
		if data.PeerIP != "" {
			tunneledTo = net.ParseIP(data.PeerIP)
		}
		sess.L2TPBinding = &models.L2TPBinding{
			LocalTunnelID:  data.LocalTunnelID,
			PeerTunnelID:   data.PeerTunnelID,
			LocalSessionID: data.LocalSessionID,
			PeerSessionID:  data.PeerSessionID,
		}
		c.checkpointSession(sess)
		_ = c.publishSessionLifecycle(&models.PPPSession{
			SessionID:     sess.SessionID,
			State:         models.SessionStateTunneled,
			AccessType:    string(models.AccessTypePPPoE),
			Protocol:      string(models.ProtocolPPPoESession),
			PPPSessionID:  sess.PPPoESessionID,
			MAC:           sess.MAC,
			OuterVLAN:     sess.OuterVLAN,
			InnerVLAN:     sess.InnerVLAN,
			IfIndex:       sess.SwIfIndex,
			Username:      sess.Username,
			AAASessionID:  sess.AcctSessionID,
			ActivatedAt:   sess.BoundAt,
			TunneledToLNS: tunneledTo,
			L2TP:          sess.L2TPBinding,
		})
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
	match, ok := s.component.cfgMgr.LookupSubscriberGroup(s.OuterVLAN, s.InnerVLAN)
	if !ok || match.VR == nil || !match.VR.HasAccessType(subscriber.AccessTypeLAC) {
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

// startLACVPPSessionAdd kicks off the dataplane bring-up for a LAC
// subscriber. Unlike the local-termination path which adds the PPPoE
// session in VPP only after IPCP converges, LAC mode needs the session
// (and its sw_if_index) installed at handoff time — the L2TP plugin
// stashes that sw_if_index as the opaque the LNS→subscriber bridge
// node uses for session lookup. Called with s.mu held.
func (s *SessionState) startLACVPPSessionAdd() {
	if s.component.vpp == nil {
		// No southbound wired (test environment) — fall straight to
		// the LAC trigger so the FSM can be exercised without VPP.
		s.triggerLACBringUp()
		return
	}

	var localMAC net.HardwareAddr
	if s.component.ifMgr != nil {
		if iface := s.component.ifMgr.Get(s.EncapIfIndex); iface != nil && len(iface.MAC) >= 6 {
			localMAC = net.HardwareAddr(iface.MAC[:6])
		}
	}
	if localMAC == nil {
		s.component.logger.Error("LAC: cannot resolve local MAC; rejecting subscriber",
			"session_id", s.SessionID, "sw_if_index", s.EncapIfIndex)
		s.failLACBringUp()
		return
	}

	s.component.logger.Info("LAC: calling AddPPPoESessionAsync",
		"session_id", s.SessionID,
		"pppoe_session_id", s.PPPoESessionID,
		"client_mac", s.MAC.String(),
		"local_mac", localMAC.String(),
		"encap_if_index", s.EncapIfIndex,
		"svlan", s.OuterVLAN, "cvlan", s.InnerVLAN)

	// LAC subscribers have no local IP assignment. VPP's reverse-route
	// installation may reject 0.0.0.0; pass a TEST-NET-1 placeholder
	// derived from the session ID. The reverse-route is dead-end and
	// never hit because the punt plugin's LAC dispatch diverts the
	// subscriber→LNS path before IP-decap.
	placeholderIP := net.IPv4(192, 0, 2,
		byte((s.PPPoESessionID%254)+1))
	s.component.vpp.AddPPPoESessionAsync(
		s.PPPoESessionID,
		placeholderIP,
		s.MAC,
		localMAC,
		s.EncapIfIndex,
		s.OuterVLAN,
		s.InnerVLAN,
		0, /* decapVrfID — irrelevant for LAC */
		1500,
		southbound.MSSClampPolicy{}, /* disabled */
		s.onLACVPPSessionCreated,
	)
}

// onLACVPPSessionCreated is the AddPPPoESessionAsync callback for the
// LAC handoff path. On success it stores the sw_if_index and fires the
// LAC trigger; on failure it rejects the subscriber.
func (s *SessionState) onLACVPPSessionCreated(swIfIndex uint32, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.component.logger.Error("LAC: AddPPPoESession failed; rejecting subscriber",
			"session_id", s.SessionID, "error", err)
		s.failLACBringUp()
		return
	}
	s.SwIfIndex = swIfIndex
	s.triggerLACBringUp()
}

// triggerLACBringUp fires the registered LAC trigger callback. On a
// synchronous error from the trigger the subscriber is rejected. Called
// with s.mu held.
func (s *SessionState) triggerLACBringUp() {
	if s.component.lacTrigger == nil {
		s.component.logger.Error("LAC: no trigger wired; rejecting subscriber",
			"session_id", s.SessionID)
		s.failLACBringUp()
		return
	}
	if err := s.component.lacTrigger(buildLACBringUpAttrs(s)); err != nil {
		s.component.logger.Warn("LAC trigger failed; rejecting subscriber",
			"session_id", s.SessionID, "error", err)
		s.failLACBringUp()
	}
}

// failLACBringUp rejects the subscriber's PPPoE auth. Called with
// s.mu held.
func (s *SessionState) failLACBringUp() {
	switch s.pendingAuthType {
	case "pap":
		s.sendPAPNak(s.pendingPAPID)
	case "chap":
		s.sendCHAPFailure(s.pendingCHAPID)
	}
	s.pendingAuthType = ""
	s.pendingAuthRequestID = ""
	s.lcp.FSM().Close()
}

// buildLACBringUpAttrs assembles the handoff payload from a PPPoE
// session whose AAA reply carried Tunnel-* attributes. Called with
// s.mu held — the function reads stable session fields plus
// already-extracted AAA attrs.
func buildLACBringUpAttrs(s *SessionState) LACBringUpAttrs {
	attrs := LACBringUpAttrs{
		PPPoESessionID: s.PPPoESessionID,
		Username:       s.Username,
		PPPoESwIfIndex: s.SwIfIndex,
		EncapIfIndex:   s.EncapIfIndex,
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
