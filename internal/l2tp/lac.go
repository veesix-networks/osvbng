// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"errors"
	"net"
	"sync/atomic"
	"time"

	"github.com/veesix-networks/osvbng/pkg/events"
	l2tppkt "github.com/veesix-networks/osvbng/pkg/l2tp"
)

// LACBringUpRequest is the PPPoE → L2TP handoff request. The PPPoE
// component builds one from the AAA Access-Accept attributes
// (Tunnel-* fields parsed into TunnelSpecs, Username, etc.) plus
// per-session identity (PPPoESessionID for the dataplane binding) and
// the proxy LCP / proxy-auth AVPs the LNS will replay against its PPP
// FSM.
//
// `LocalName` is the Host Name AVP we put on outbound SCCRQ; defaults
// to the LNSConfig hostname configured on the component.
type LACBringUpRequest struct {
	PPPoESessionID         uint16
	Username               string
	TunnelSpecs            []TunnelSpec
	LocalName              string
	LocalIP                net.IP

	// PPPoESwIfIndex is the partner PPPoE session's sw_if_index in
	// VPP. Stashed in `vnet_buffer_l2tpv2_opaque` by the L2TP plugin
	// when decapsulating LNS→subscriber frames so the
	// osvbng-pppoe-lac-tx node can look up the session and prepend
	// Eth+VLAN+PPPoE before TX. Required for AddL2TPSessionRaw.
	PPPoESwIfIndex uint32

	// EncapIfIndex is the TX interface VPP should use when emitting
	// outbound L2TP packets. Typically the BNG's uplink. ~0 lets the
	// plugin pick via FIB lookup on the peer IP.
	EncapIfIndex uint32

	// Proxy LCP / proxy auth replay material, copied into ICCN per
	// RFC 3437.
	LastSentLCPConfReq     []byte
	LastReceivedLCPConfReq []byte
	ProxyAuthenType        uint16
	ProxyAuthenName        string
	ProxyAuthenChallenge   []byte
	ProxyAuthenResponse    []byte
}

// lacCallSerial is the monotonically increasing Call Serial Number
// stamped on outbound ICRQ messages. RFC 2661 §4.4.13 requires
// uniqueness across the lifetime of the LAC; a process-wide counter is
// sufficient since LAC restart implies fresh state.
var lacCallSerial atomic.Uint32

// StartLACSession kicks off LAC bring-up for one PPPoE subscriber. The
// first non-denylisted TunnelSpec in `req` is tried; the outcome is
// reported asynchronously on TopicL2TPLACDecision once the session has
// either reached SessionEstablished or the candidate list is exhausted.
//
// Returns an error synchronously only if the request is malformed (no
// candidates, missing fields, send transport not configured); transient
// LNS failures are reported via the decision event so the caller does
// not need to subscribe twice.
func (c *Component) StartLACSession(req LACBringUpRequest) error {
	if len(req.TunnelSpecs) == 0 {
		return ErrNoTunnelCandidates
	}
	if c.send == nil {
		return ErrSendNotConfigured
	}

	skipped := 0
	for i := range req.TunnelSpecs {
		spec := req.TunnelSpecs[i]
		if spec.ServerIP == nil {
			continue
		}
		if c.denylist != nil && c.denylist.IsDenied(spec.ServerIP) {
			skipped++
			c.log.Debug("LAC candidate skipped (denylisted)",
				"peer_ip", spec.ServerIP.String(),
				"reason", c.denylist.Reason(spec.ServerIP))
			continue
		}
		if err := c.tryLACTunnel(req, spec); err == nil {
			return nil
		} else {
			c.log.Debug("LAC tunnel candidate failed, trying next",
				"peer_ip", spec.ServerIP.String(), "error", err)
		}
	}
	if skipped > 0 && skipped == len(req.TunnelSpecs) {
		c.publishLACDecision(req.PPPoESessionID, nil, nil, ErrAllCandidatesDenied)
		return ErrAllCandidatesDenied
	}
	c.publishLACDecision(req.PPPoESessionID, nil, nil, ErrNoTunnelCandidates)
	return ErrNoTunnelCandidates
}

// tryLACTunnel attempts to bring up a tunnel + session against one
// LNS candidate. Returns nil on success once the SCCRQ has been sent
// (the rest of the bring-up runs asynchronously through Dispatch);
// returns an error if local state setup failed and the caller should
// move to the next candidate.
func (c *Component) tryLACTunnel(req LACBringUpRequest, spec TunnelSpec) error {
	peerIP := spec.ServerIP
	localIP := req.LocalIP
	if localIP == nil {
		localIP = spec.ClientIP
	}
	if localIP == nil {
		// Vendor pattern: pool-config `source-ipv4` per-LNS (Cisco
		// `source-ip`, RTBrick `client-ipv4`). Look up the running
		// config and find the LNS entry whose IPv4 matches the peer,
		// then use its SourceIPv4 as the L2TP local IP.
		localIP = c.lookupConfiguredSourceIP(peerIP)
	}
	if localIP == nil {
		return ErrNoLocalIP
	}

	localTunnelID, err := c.allocateTunnelID(peerIP)
	if err != nil {
		return ErrTunnelExhaustion
	}

	hostName := req.LocalName
	if hostName == "" {
		hostName = c.localHostname
	}

	var ourChallenge []byte
	if len(spec.Password) > 0 {
		ourChallenge, err = l2tppkt.NewChallenge()
		if err != nil {
			c.releaseTunnelID(peerIP, localTunnelID)
			return err
		}
	}

	t := &Tunnel{
		LocalIP:              localIP,
		PeerIP:               peerIP,
		LocalID:              localTunnelID,
		LocalPort:            1701,
		PeerPort:             1701,
		Role:                 l2tppkt.RoleInitiator,
		FSM:                  l2tppkt.NewTunnelFSM(l2tppkt.RoleInitiator),
		LocalHostname:        hostName,
		Secret:               []byte(spec.Password),
		Sessions:             make(map[uint16]*Session),
		CreatedAt:            time.Now(),
		outstandingChallenge: ourChallenge,
		PPPHdrSkip:           spec.PPPHdrSkip,
	}
	if err := t.FSM.SendSCCRQ(); err != nil {
		c.releaseTunnelID(peerIP, localTunnelID)
		return err
	}
	if err := c.registerTunnel(t); err != nil {
		c.releaseTunnelID(peerIP, localTunnelID)
		return err
	}

	c.startTunnelRunner(t, 60*time.Second)

	// Stash the bring-up request on the tunnel under the LAC mutex
	// so the SCCRP handler can find it when the tunnel reaches
	// Established and the LAC session needs to be opened.
	c.lacMu.Lock()
	if c.lacPending == nil {
		c.lacPending = make(map[tunnelKey]*LACBringUpRequest)
	}
	c.lacPending[makeTunnelKey(peerIP, localTunnelID)] = &req
	c.lacMu.Unlock()

	sccrqBody := l2tppkt.BuildSCCRQ(l2tppkt.SCCRQParams{
		LocalTunnelID:     localTunnelID,
		ReceiveWindowSize: 16,
		HostName:          hostName,
		FramingCaps:       l2tppkt.FramingSync,
		BearerCaps:        l2tppkt.BearerDigital,
		Challenge:         ourChallenge,
	})
	if err := t.Channel.Send(sccrqBody, time.Now()); err != nil {
		c.stopTunnelRunner(peerIP, localTunnelID)
		c.unregisterTunnel(peerIP, localTunnelID)
		c.releaseTunnelID(peerIP, localTunnelID)
		c.clearLACPending(peerIP, localTunnelID)
		return err
	}
	return nil
}

// handleSCCRP processes an inbound SCCRP on a LAC-side tunnel. It
// learns the peer Tunnel-ID, verifies our challenge if we issued one,
// computes a Challenge-Response if the peer challenged us, and sends
// SCCCN. The tunnel FSM advances to Established and the pending LAC
// session is opened with an ICRQ.
func (c *Component) handleSCCRP(t *Tunnel, avps []l2tppkt.AVP) error {
	if l2tppkt.DecodeMessageType(avps) != l2tppkt.MsgTypeSCCRP {
		return ErrNotSCCRP
	}
	if t.Role != l2tppkt.RoleInitiator {
		return ErrUnexpectedSCCRP
	}

	assigned := l2tppkt.FindFirst(avps, 0, l2tppkt.AVPAssignedTunnelID)
	if assigned == nil || len(assigned.Value) < 2 {
		return ErrMissingAssignedTunnelID
	}
	peerTunnelID := l2tppkt.DecodeUint16(assigned)

	if hostName := l2tppkt.FindFirst(avps, 0, l2tppkt.AVPHostName); hostName != nil {
		t.PeerHostname = l2tppkt.DecodeString(hostName)
	}

	t.mu.Lock()
	t.PeerID = peerTunnelID
	outstanding := t.outstandingChallenge
	t.mu.Unlock()

	// Verify the LNS's response to our Challenge, if any.
	if len(outstanding) > 0 {
		respAVP := l2tppkt.FindFirst(avps, 0, l2tppkt.AVPChallengeResponse)
		if respAVP == nil {
			return ErrMissingChallengeResponse
		}
		if err := l2tppkt.VerifyChallengeResponse(
			byte(l2tppkt.MsgTypeSCCRP), t.Secret, outstanding, respAVP.Value,
		); err != nil {
			return err
		}
		t.mu.Lock()
		t.outstandingChallenge = nil
		t.mu.Unlock()
	}

	// Compute a Challenge-Response for the LNS's challenge, if any.
	var peerResp []byte
	if peerChallenge := l2tppkt.FindFirst(avps, 0, l2tppkt.AVPChallenge); peerChallenge != nil {
		if len(t.Secret) == 0 {
			return ErrChallengeWithoutSecret
		}
		peerResp = l2tppkt.ComputeChallengeResponse(
			byte(l2tppkt.MsgTypeSCCCN), t.Secret, peerChallenge.Value,
		)
	}

	if err := t.FSM.RecvSCCRP(); err != nil {
		return err
	}

	sccccnBody := l2tppkt.BuildSCCCN(peerResp)
	if err := t.Channel.Send(sccccnBody, time.Now()); err != nil {
		return err
	}

	// Tunnel is now Established. Install it in the VPP L2TPv2 plugin so
	// subsequent session-add binapi calls can resolve the tunnel.
	if err := c.installTunnelVPP(t); err != nil {
		c.log.Error("AddL2TPTunnel failed; aborting LAC bring-up",
			"peer_ip", t.PeerIP.String(),
			"local_tunnel_id", t.LocalID,
			"peer_tunnel_id", t.PeerID,
			"error", err)
		return err
	}

	// Open the pending LAC session.
	c.lacMu.Lock()
	req := c.lacPending[makeTunnelKey(t.PeerIP, t.LocalID)]
	c.lacMu.Unlock()
	if req == nil {
		return ErrLACRequestMissing
	}
	return c.openLACSession(t, req)
}

// openLACSession allocates a session ID under the tunnel, registers
// the session, and sends ICRQ. The ICRP handler completes the bring-up.
func (c *Component) openLACSession(t *Tunnel, req *LACBringUpRequest) error {
	t.mu.Lock()
	if t.Sessions == nil {
		t.Sessions = make(map[uint16]*Session)
	}
	var localID uint16
	for try := uint16(1); try != 0; try++ {
		if _, used := t.Sessions[try]; !used {
			localID = try
			break
		}
	}
	if localID == 0 {
		t.mu.Unlock()
		return ErrSessionExhaustion
	}
	t.mu.Unlock()

	s := &Session{
		SessionID:      makeSessionID(t.PeerIP, t.LocalID, localID),
		Tunnel:         t,
		LocalID:        localID,
		Role:           l2tppkt.SessionRoleLAC,
		FSM:            l2tppkt.NewSessionFSM(l2tppkt.SessionRoleLAC),
		Username:       req.Username,
		PPPoESessionID: req.PPPoESessionID,
		Attributes:     make(map[string]string),
	}
	if err := s.FSM.SendICRQ(); err != nil {
		return err
	}
	t.addSession(s)

	icrqBody := l2tppkt.BuildICRQ(l2tppkt.ICRQParams{
		LocalSessionID:   localID,
		CallSerialNumber: lacCallSerial.Add(1),
	})
	return t.Channel.Send(icrqBody, time.Now())
}

// handleICRP processes the LNS's reply to our ICRQ. The peer Session-ID
// is learned, the FSM advances, and ICCN is sent to complete the
// session bring-up.
func (c *Component) handleICRP(s *Session, avps []l2tppkt.AVP) error {
	if l2tppkt.DecodeMessageType(avps) != l2tppkt.MsgTypeICRP {
		return ErrNotICRP
	}
	if s.Role != l2tppkt.SessionRoleLAC {
		return ErrUnexpectedICRP
	}
	assigned := l2tppkt.FindFirst(avps, 0, l2tppkt.AVPAssignedSessionID)
	if assigned == nil || len(assigned.Value) < 2 {
		return ErrMissingAssignedSessionID
	}

	s.mu.Lock()
	s.PeerID = l2tppkt.DecodeUint16(assigned)
	s.mu.Unlock()

	if err := s.FSM.RecvICRP(); err != nil {
		return err
	}

	t := s.Tunnel
	c.lacMu.Lock()
	req := c.lacPending[makeTunnelKey(t.PeerIP, t.LocalID)]
	c.lacMu.Unlock()
	if req == nil {
		return ErrLACRequestMissing
	}

	iccnBody := l2tppkt.BuildICCN(l2tppkt.ICCNParams{
		Framing:                l2tppkt.FramingSync,
		LastSentLCPConfReq:     req.LastSentLCPConfReq,
		LastReceivedLCPConfReq: req.LastReceivedLCPConfReq,
		ProxyAuthenType:        req.ProxyAuthenType,
		ProxyAuthenName:        req.ProxyAuthenName,
		ProxyAuthenChallenge:   req.ProxyAuthenChallenge,
		ProxyAuthenResponse:    req.ProxyAuthenResponse,
	})
	// RFC 2661 §3.1, §5.1: session-scoped messages after the peer has
	// assigned a Session ID carry that peer Session ID in the L2TP
	// header. ICRP delivered s.PeerID above.
	if err := t.Channel.SendSession(iccnBody, s.PeerID, time.Now()); err != nil {
		return err
	}

	if c.vpp != nil {
		poolIndex, err := c.vpp.AddL2TPSessionRaw(
			t.LocalIP, t.PeerIP,
			t.LocalID, s.LocalID, s.PeerID,
			lacRawNextNode, req.PPPoESwIfIndex, req.EncapIfIndex,
			t.PPPHdrSkip,
		)
		if err != nil {
			c.log.Error("AddL2TPSessionRaw failed; aborting LAC bring-up",
				"session_id", s.SessionID, "error", err)
			c.clearLACPending(t.PeerIP, t.LocalID)
			c.publishLACDecision(req.PPPoESessionID, nil, nil, err)
			return err
		}
		s.mu.Lock()
		s.SwIfIndex = poolIndex
		s.mu.Unlock()
	}

	s.mu.Lock()
	s.ActivatedAt = time.Now()
	s.PPPoESwIfIndex = req.PPPoESwIfIndex
	s.mu.Unlock()
	c.clearLACPending(t.PeerIP, t.LocalID)
	c.publishLACDecision(req.PPPoESessionID, t, s, nil)
	return nil
}

// lacRawNextNode is the VPP graph node the L2TPv2 plugin forwards
// decapsulated PPP frames to in the LNS→subscriber direction. The
// PPPoE plugin registers this node at init time per
// AMENDMENT-PLUGIN-TRANSPARENCY.md.
const lacRawNextNode = "osvbng-pppoe-lac-tx"

// publishLACDecision emits the TopicL2TPLACDecision event the PPPoE
// component subscribes to. On success `t`/`s` describe the bound
// session; on failure they are nil and `err` carries the reason.
func (c *Component) publishLACDecision(pppoeSessionID uint16, t *Tunnel, s *Session, err error) {
	if c.eventBus == nil {
		return
	}
	evt := &events.L2TPLACDecisionEvent{
		PPPoESessionID: pppoeSessionID,
		Success:        err == nil,
	}
	if err != nil {
		evt.Error = err.Error()
	}
	if t != nil {
		if t.LocalIP != nil {
			evt.LocalIP = t.LocalIP.String()
		}
		if t.PeerIP != nil {
			evt.PeerIP = t.PeerIP.String()
		}
		evt.LocalTunnelID = t.LocalID
		evt.PeerTunnelID = t.PeerID
	}
	if s != nil {
		evt.LocalSessionID = s.LocalID
		evt.PeerSessionID = s.PeerID
		evt.LACL2TPSessionIndex = s.SwIfIndex
	}
	c.eventBus.Publish(events.TopicL2TPLACDecision, events.Event{
		Source: c.Name(),
		Data:   evt,
	})
}

func (c *Component) clearLACPending(peerIP net.IP, localTunnelID uint16) {
	c.lacMu.Lock()
	delete(c.lacPending, makeTunnelKey(peerIP, localTunnelID))
	c.lacMu.Unlock()
}

var (
	ErrNoTunnelCandidates  = errors.New("l2tp: no LAC tunnel candidates")
	ErrAllCandidatesDenied = errors.New("l2tp: all LAC candidates denylisted")
	ErrNotSCCRP           = errors.New("l2tp: expected SCCRP")
	ErrNotICRP            = errors.New("l2tp: expected ICRP")
	ErrUnexpectedSCCRP    = errors.New("l2tp: SCCRP on non-initiator tunnel")
	ErrUnexpectedICRP     = errors.New("l2tp: ICRP on non-LAC session")
	ErrLACRequestMissing  = errors.New("l2tp: LAC bring-up request lost")
	ErrNoLocalIP          = errors.New("l2tp: no local IP for tunnel; set tunnel-pool lns.source-ipv4 or AAA Tunnel-Client-Endpoint")
)

// lookupConfiguredSourceIP scans the running L2TPConfig for an LNS
// entry whose `ipv4` matches `peerIP` and returns its `source-ipv4`.
// Returns nil if no match. The scan is bounded by the number of LNS
// entries configured (typically <100), so a linear walk is fine.
func (c *Component) lookupConfiguredSourceIP(peerIP net.IP) net.IP {
	if c.cfgMgr == nil {
		return nil
	}
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil || cfg.L2TP == nil {
		return nil
	}
	for _, pool := range cfg.L2TP.TunnelPools {
		if pool == nil {
			continue
		}
		for i := range pool.LNS {
			lnsIP := net.ParseIP(pool.LNS[i].IPv4)
			if lnsIP == nil || !lnsIP.Equal(peerIP) {
				continue
			}
			if pool.LNS[i].SourceIPv4 == "" {
				continue
			}
			if src := net.ParseIP(pool.LNS[i].SourceIPv4); src != nil {
				return src
			}
		}
	}
	return nil
}
