// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"errors"
	"net"
	"time"

	l2tppkt "github.com/veesix-networks/osvbng/pkg/l2tp"
)

// LNSConfig is the per-peer configuration used by the LNS path when an
// inbound SCCRQ arrives from a known LAC (matched by Host Name AVP).
type LNSConfig struct {
	LocalHostname     string
	VendorName        string
	ReceiveWindowSize uint16
	Secret            []byte // for Challenge-AVP verification
	ChallengeRequired bool
	HelloInterval     time.Duration

	// PPP framing byte count on data packets (2 = HDLC prefix
	// present, 0 = ACFC compressed). Resolved from profile +
	// peer-policy at lookup time.
	PPPHdrSkip uint8
}

// HandleSCCRQ processes an inbound SCCRQ on the LNS side. It validates
// the peer's Host Name + Challenge, allocates a local Tunnel-ID,
// constructs the Tunnel + ControlChannel + FSM, and returns the SCCRP
// body to send. The caller is responsible for transmitting the reply
// via the (yet-to-be-bound) UDP socket; the control channel arms its
// retransmit timer on the first send.
//
// `localIP` is the address the SCCRQ was sent to and is stored on the
// tunnel for downstream use by the southbound (which needs both ends
// of the L2TP-over-UDP flow to install a session).
//
// Errors fall into two classes:
//   - protocol violations (missing mandatory AVPs, bad version) →
//     return ErrSCCRQ* and the caller emits StopCCN.
//   - resource exhaustion (tunnel-ID allocator full) → ErrTunnelExhaustion.
func (c *Component) HandleSCCRQ(localIP, peerIP net.IP, avps []l2tppkt.AVP, cfg LNSConfig) (sccrpBody []byte, t *Tunnel, err error) {
	if l2tppkt.DecodeMessageType(avps) != l2tppkt.MsgTypeSCCRQ {
		return nil, nil, ErrNotSCCRQ
	}

	hostNameAVP := l2tppkt.FindFirst(avps, 0, l2tppkt.AVPHostName)
	if hostNameAVP == nil {
		return nil, nil, ErrMissingHostName
	}
	peerHostname := l2tppkt.DecodeString(hostNameAVP)

	assignedAVP := l2tppkt.FindFirst(avps, 0, l2tppkt.AVPAssignedTunnelID)
	if assignedAVP == nil || len(assignedAVP.Value) < 2 {
		return nil, nil, ErrMissingAssignedTunnelID
	}
	peerTunnelID := l2tppkt.DecodeUint16(assignedAVP)

	// Challenge validation (if required by profile or sent by peer).
	var ourChallengeResp []byte
	if challengeAVP := l2tppkt.FindFirst(avps, 0, l2tppkt.AVPChallenge); challengeAVP != nil {
		if len(cfg.Secret) == 0 {
			return nil, nil, ErrChallengeWithoutSecret
		}
		ourChallengeResp = l2tppkt.ComputeChallengeResponse(
			byte(l2tppkt.MsgTypeSCCRP), cfg.Secret, challengeAVP.Value,
		)
	} else if cfg.ChallengeRequired {
		return nil, nil, ErrChallengeRequired
	}

	// Mutual authentication: when the profile requires challenge auth
	// and a Secret is configured, the LNS issues its own Challenge in
	// SCCRP and verifies the peer's Challenge-Response in SCCCN. Stored
	// on the tunnel until SCCCN clears it.
	var ourChallenge []byte
	if cfg.ChallengeRequired && len(cfg.Secret) > 0 {
		ourChallenge, err = l2tppkt.NewChallenge()
		if err != nil {
			return nil, nil, err
		}
	}

	// Allocate our Tunnel-ID and instantiate state.
	localID, err := c.allocateTunnelID(peerIP)
	if err != nil {
		return nil, nil, ErrTunnelExhaustion
	}

	t = &Tunnel{
		LocalIP:              localIP,
		PeerIP:               peerIP,
		LocalID:              localID,
		PeerID:               peerTunnelID,
		LocalPort:            1701,
		PeerPort:             1701,
		Role:                 l2tppkt.RoleResponder,
		FSM:                  l2tppkt.NewTunnelFSM(l2tppkt.RoleResponder),
		LocalHostname:        cfg.LocalHostname,
		PeerHostname:         peerHostname,
		Secret:               cfg.Secret,
		Sessions:             make(map[uint16]*Session),
		HelloInterval:        cfg.HelloInterval,
		PPPHdrSkip:           cfg.PPPHdrSkip,
		CreatedAt:            time.Now(),
		outstandingChallenge: ourChallenge,
	}
	if err := t.FSM.RecvSCCRQ(); err != nil {
		c.releaseTunnelID(peerIP, localID)
		return nil, nil, err
	}
	if err := c.registerTunnel(t); err != nil {
		c.releaseTunnelID(peerIP, localID)
		return nil, nil, err
	}

	c.startTunnelRunner(t, cfg.HelloInterval)

	sccrpBody = l2tppkt.BuildSCCRP(l2tppkt.SCCRPParams{
		LocalTunnelID:     localID,
		ReceiveWindowSize: cfg.ReceiveWindowSize,
		HostName:          cfg.LocalHostname,
		VendorName:        cfg.VendorName,
		FramingCaps:       l2tppkt.FramingSync,
		BearerCaps:        l2tppkt.BearerDigital,
		Challenge:         ourChallenge,
		ChallengeResponse: ourChallengeResp,
	})
	return sccrpBody, t, nil
}

// HandleSCCCN advances the tunnel FSM to Established after the peer's
// SCCCN arrives. If `outstandingChallenge` is non-empty (the LNS issued
// a Challenge AVP in SCCRP) the SCCCN must carry a Challenge-Response
// AVP that verifies against it; on success the tunnel's outstanding-
// challenge state is cleared so a retransmitted SCCCN is idempotent.
func (c *Component) HandleSCCCN(t *Tunnel, avps []l2tppkt.AVP, outstandingChallenge []byte) error {
	if l2tppkt.DecodeMessageType(avps) != l2tppkt.MsgTypeSCCCN {
		return ErrNotSCCCN
	}
	if len(outstandingChallenge) > 0 {
		respAVP := l2tppkt.FindFirst(avps, 0, l2tppkt.AVPChallengeResponse)
		if respAVP == nil {
			return ErrMissingChallengeResponse
		}
		if err := l2tppkt.VerifyChallengeResponse(
			byte(l2tppkt.MsgTypeSCCCN), t.Secret, outstandingChallenge, respAVP.Value,
		); err != nil {
			return err
		}
		t.mu.Lock()
		t.outstandingChallenge = nil
		t.mu.Unlock()
	}
	if err := t.FSM.RecvSCCCN(); err != nil {
		return err
	}
	// Tunnel is now Established. Install it in the VPP L2TPv2 plugin
	// so subsequent AddPPPoL2TPSession calls (on ICRQ) can resolve it.
	if err := c.installTunnelVPP(t); err != nil {
		c.log.Error("AddL2TPTunnel failed on LNS",
			"peer_ip", t.PeerIP.String(),
			"local_tunnel_id", t.LocalID,
			"peer_tunnel_id", t.PeerID,
			"error", err)
		return err
	}
	return nil
}

// HandleICRQ processes an inbound ICRQ on the LNS side. It allocates a
// local Session-ID, instantiates the Session, drives the session FSM,
// and returns the ICRP body to send. The session's PPP termination is
// not started until ICCN arrives (HandleICCN).
func (c *Component) HandleICRQ(t *Tunnel, avps []l2tppkt.AVP) (icrpBody []byte, s *Session, err error) {
	if l2tppkt.DecodeMessageType(avps) != l2tppkt.MsgTypeICRQ {
		return nil, nil, ErrNotICRQ
	}
	assignedAVP := l2tppkt.FindFirst(avps, 0, l2tppkt.AVPAssignedSessionID)
	if assignedAVP == nil || len(assignedAVP.Value) < 2 {
		return nil, nil, ErrMissingAssignedSessionID
	}
	peerSessionID := l2tppkt.DecodeUint16(assignedAVP)

	// Allocate session ID under the tunnel scope.
	t.mu.Lock()
	if t.Sessions == nil {
		t.Sessions = make(map[uint16]*Session)
	}
	// Simple linear search for an unused ID; tunnels rarely exceed
	// a few hundred sessions and the search is bounded by 64k. A
	// per-tunnel IDAllocator would be the next-step refinement.
	var localID uint16
	for try := uint16(1); try != 0; try++ {
		if _, used := t.Sessions[try]; !used {
			localID = try
			break
		}
	}
	if localID == 0 {
		t.mu.Unlock()
		return nil, nil, ErrSessionExhaustion
	}
	t.mu.Unlock()

	s = &Session{
		SessionID:  makeSessionID(t.PeerIP, t.LocalID, localID),
		Tunnel:     t,
		LocalID:    localID,
		PeerID:     peerSessionID,
		Role:       l2tppkt.SessionRoleLNS,
		FSM:        l2tppkt.NewSessionFSM(l2tppkt.SessionRoleLNS),
		Attributes: make(map[string]string),
	}
	if err := s.FSM.RecvICRQ(); err != nil {
		return nil, nil, err
	}
	t.addSession(s)

	if err := c.installLNSSessionVPP(s); err != nil {
		t.removeSession(s.LocalID)
		return nil, nil, err
	}

	icrpBody = l2tppkt.BuildICRP(l2tppkt.ICRPParams{
		LocalSessionID: localID,
	})
	return icrpBody, s, nil
}

// HandleICCN advances the session FSM to Established and starts PPP
// termination. After this call returns successfully the session has
// LCP / IPCP / IPv6CP / PAP / CHAP allocated and LCP has been driven
// open; inbound PPP control frames route through the session's
// dispatcher from now on.
func (c *Component) HandleICCN(s *Session, avps []l2tppkt.AVP) error {
	if l2tppkt.DecodeMessageType(avps) != l2tppkt.MsgTypeICCN {
		return ErrNotICCN
	}
	s.ActivatedAt = time.Now()
	if err := s.FSM.RecvICCN(); err != nil {
		return err
	}
	c.initSessionPPP(s)
	return nil
}

// HandleCDN tears the session down.
func (c *Component) HandleCDN(s *Session) {
	s.FSM.Disconnect()
	if s.Tunnel != nil {
		s.Tunnel.removeSession(s.LocalID)
	}
}

// HandleStopCCN tears the tunnel down.
func (c *Component) HandleStopCCN(t *Tunnel) {
	t.FSM.Stop()
	c.stopTunnelRunner(t.PeerIP, t.LocalID)
	c.unregisterTunnel(t.PeerIP, t.LocalID)
	c.releaseTunnelID(t.PeerIP, t.LocalID)
	c.uninstallTunnelVPP(t)
}

var (
	ErrNotSCCRQ                = errors.New("l2tp: expected SCCRQ")
	ErrNotSCCCN                = errors.New("l2tp: expected SCCCN")
	ErrNotICRQ                 = errors.New("l2tp: expected ICRQ")
	ErrNotICCN                 = errors.New("l2tp: expected ICCN")
	ErrMissingHostName         = errors.New("l2tp: SCCRQ missing Host Name AVP")
	ErrMissingAssignedTunnelID = errors.New("l2tp: missing Assigned Tunnel ID AVP")
	ErrMissingAssignedSessionID = errors.New("l2tp: missing Assigned Session ID AVP")
	ErrMissingChallengeResponse = errors.New("l2tp: missing Challenge Response AVP")
	ErrChallengeWithoutSecret   = errors.New("l2tp: peer sent Challenge but no secret configured")
	ErrChallengeRequired       = errors.New("l2tp: SCCRQ missing required Challenge AVP")
	ErrTunnelExhaustion        = errors.New("l2tp: tunnel ID space exhausted for peer")
	ErrSessionExhaustion       = errors.New("l2tp: session ID space exhausted in tunnel")
)
