// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/veesix-networks/osvbng/pkg/aaa"
	"github.com/veesix-networks/osvbng/pkg/allocator"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/ppp"
)

const (
	chapRetryTimeout = 3 * time.Second
	chapMaxRetries   = 10
)

// onLCPUp is invoked by the LCP FSM when negotiation converges. The
// session transitions to PhaseAuthenticate and the LNS issues a CHAP
// challenge. If the peer LCP advertises no auth requirement the
// session skips straight to PhaseNetwork.
//
// Called with s.mu held — the FSM input path (dispatcher) and the
// FSM.Open path (initSessionPPP) both invoke FSM callbacks under the
// session lock.
func (c *Component) onLCPUp(s *Session) {
	s.LCPMagic = s.LCP.LocalConfig().Magic
	lcpCfg := s.LCP.LocalConfig()
	if lcpCfg.WantAuth {
		s.Phase = ppp.PhaseAuthenticate
		c.startCHAPAuth(s)
	} else {
		s.Phase = ppp.PhaseNetwork
		c.startNCP(s)
	}
}

// onLCPDown is called with s.mu held.
func (c *Component) onLCPDown(s *Session) {
	s.Phase = ppp.PhaseEstablish
}

// startCHAPAuth issues the initial CHAP challenge to the peer and
// arms the retry timer. Called with s.mu held.
func (c *Component) startCHAPAuth(s *Session) {
	s.chapRetryCount = 0
	c.sendCHAPChallenge(s)
}

func (c *Component) sendCHAPChallenge(s *Session) {
	s.chapChallenge = make([]byte, 16)
	_, _ = rand.Read(s.chapChallenge)
	s.chapID++
	s.CHAP.SendChallenge(s.chapID, s.chapChallenge, c.localHostname)

	c.stopCHAPRetryTimer(s)
	s.chapRetryTimer = time.AfterFunc(chapRetryTimeout, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		c.handleCHAPTimeout(s)
	})
}

func (c *Component) handleCHAPTimeout(s *Session) {
	if s.Phase != ppp.PhaseAuthenticate {
		return
	}
	s.chapRetryCount++
	if s.chapRetryCount >= chapMaxRetries {
		c.log.Warn("CHAP authentication timeout", "session_id", s.SessionID)
		s.LCP.FSM().Close()
		return
	}
	c.sendCHAPChallenge(s)
}

func (c *Component) stopCHAPRetryTimer(s *Session) {
	if s.chapRetryTimer != nil {
		s.chapRetryTimer.Stop()
		s.chapRetryTimer = nil
	}
}

// handleCHAPPacket consumes inbound CHAP frames. On a Response code,
// the LNS publishes an AAA request carrying the CHAP material and
// stalls the FSM until handleAAAResponse fires.
func (c *Component) handleCHAPPacket(s *Session, code, id uint8, data []byte) error {
	if s.Phase != ppp.PhaseAuthenticate {
		return nil
	}
	if code != ppp.CHAPResponse {
		return nil
	}
	c.stopCHAPRetryTimer(s)
	if len(data) < 1 {
		return nil
	}
	valueLen := int(data[0])
	if len(data) < 1+valueLen {
		return nil
	}
	response := data[1 : 1+valueLen]
	username := string(data[1+valueLen:])

	s.Username = username
	s.pendingAuthType = "chap"
	s.pendingCHAPID = id

	c.publishAAARequest(s, map[string]string{
		aaa.AttrCHAPID:        hex.EncodeToString([]byte{id}),
		aaa.AttrCHAPChallenge: hex.EncodeToString(s.chapChallenge),
		aaa.AttrCHAPResponse:  hex.EncodeToString(response),
	})
	return nil
}

// handlePAPPacket consumes inbound PAP frames. PAP is rarely used on
// L2TP LNS but supporting it for parity is cheap.
func (c *Component) handlePAPPacket(s *Session, code, id uint8, data []byte) error {
	if s.Phase != ppp.PhaseAuthenticate {
		return nil
	}
	if code != ppp.PAPAuthReq {
		return nil
	}
	if len(data) < 2 {
		return nil
	}
	userLen := int(data[0])
	if len(data) < 1+userLen+1 {
		return nil
	}
	username := string(data[1 : 1+userLen])
	passLen := int(data[1+userLen])
	if len(data) < 2+userLen+passLen {
		return nil
	}
	password := string(data[2+userLen : 2+userLen+passLen])

	s.Username = username
	s.pendingAuthType = "pap"
	s.pendingPAPID = id

	c.publishAAARequest(s, map[string]string{aaa.AttrPassword: password})
	return nil
}

// publishAAARequest fires an AAA-request event for the session. The AAA
// component answers on TopicAAAResponseL2TP; handleAAAResponse below
// matches the response back to the session via pendingAuthRequestID.
// Called with s.mu held.
func (c *Component) publishAAARequest(s *Session, attrs map[string]string) {
	if c.eventBus == nil {
		c.log.Warn("L2TP AAA request dropped: no event bus wired",
			"session_id", s.SessionID)
		return
	}
	requestID := uuid.New().String()
	s.pendingAuthRequestID = requestID

	sg := c.resolveLNSSubscriberGroup()
	policyName := ""
	if sg != nil {
		policyName = sg.AAAPolicy
	}

	username := s.Username
	if s.AcctSessionID == "" {
		s.AcctSessionID = uuid.New().String()
	}

	c.eventBus.Publish(events.TopicAAARequest, events.Event{
		Source: c.Name(),
		Data: &events.AAARequestEvent{
			AccessType: models.AccessTypeL2TP,
			Protocol:   models.ProtocolL2TP,
			SessionID:  s.SessionID,
			Request: models.AAARequest{
				RequestID:     requestID,
				Username:      username,
				AcctSessionID: s.AcctSessionID,
				PolicyName:    policyName,
				Attributes:    attrs,
			},
		},
	})
}

// handleAAAResponse routes an AAA response back to its session. The
// session is matched by RequestID rather than SessionID because a
// session can re-authenticate (Phase 5.6) and AAA replies are tagged
// only with the originating request.
func (c *Component) handleAAAResponse(event events.Event) {
	data, ok := event.Data.(*events.AAAResponseEvent)
	if !ok {
		return
	}
	resp := data.Response

	s := c.findSessionByAuthRequestID(resp.RequestID)
	if s == nil {
		c.log.Debug("L2TP AAA response for unknown request", "request_id", resp.RequestID)
		return
	}

	s.mu.Lock()
	if s.pendingAuthRequestID != resp.RequestID {
		s.mu.Unlock()
		return
	}
	c.onAuthResult(s, resp.Allowed, resp.Attributes)
	s.mu.Unlock()
}

// onAuthResult handles the AAA verdict. On Access-Accept the CHAP-
// Success frame is sent and the session advances to PhaseNetwork;
// on Access-Reject the CHAP-Failure frame is sent and LCP closes.
// Called with s.mu held.
func (c *Component) onAuthResult(s *Session, allowed bool, attributes map[string]interface{}) {
	if allowed {
		for k, v := range attributes {
			if str, ok := v.(string); ok {
				s.Attributes[k] = str
			}
		}
		c.extractIPFromAttributes(s)
		c.resolveServiceGroup(s, attributes)
		s.AllocCtx = c.buildAllocContext(s, attributes)

		switch s.pendingAuthType {
		case "pap":
			c.sendPAPAck(s, s.pendingPAPID)
		case "chap":
			c.sendCHAPSuccess(s, s.pendingCHAPID)
		}
		s.Phase = ppp.PhaseNetwork
		c.startNCP(s)
	} else {
		c.log.Warn("L2TP authentication failed",
			"session_id", s.SessionID,
			"auth_type", s.pendingAuthType)
		switch s.pendingAuthType {
		case "pap":
			c.sendPAPNak(s, s.pendingPAPID)
		case "chap":
			c.sendCHAPFailure(s, s.pendingCHAPID)
		}
		s.LCP.FSM().Close()
	}
	s.pendingAuthType = ""
	s.pendingAuthRequestID = ""
}

func (c *Component) sendCHAPSuccess(s *Session, id uint8) {
	s.CHAP.Send(ppp.CHAPSuccess, id, []byte("Welcome"))
}

func (c *Component) sendCHAPFailure(s *Session, id uint8) {
	s.CHAP.Send(ppp.CHAPFailure, id, []byte("Authentication failed"))
}

func (c *Component) sendPAPAck(s *Session, id uint8) {
	msg := "Login OK"
	data := make([]byte, 1+len(msg))
	data[0] = byte(len(msg))
	copy(data[1:], msg)
	s.PAP.Send(ppp.PAPAuthAck, id, data)
}

func (c *Component) sendPAPNak(s *Session, id uint8) {
	msg := "Authentication failed"
	data := make([]byte, 1+len(msg))
	data[0] = byte(len(msg))
	copy(data[1:], msg)
	s.PAP.Send(ppp.PAPAuthNak, id, data)
}

// extractIPFromAttributes pulls Framed-IP-Address / Framed-IPv6-* off
// the AAA reply when present. AAA-provided addresses win over pool
// allocation. Called with s.mu held.
func (c *Component) extractIPFromAttributes(s *Session) {
	if ip, ok := s.Attributes[aaa.AttrIPv4Address]; ok {
		if parsed := net.ParseIP(ip); parsed != nil {
			s.IPv4Address = parsed
		}
	}
	if v6addr, ok := s.Attributes[aaa.AttrIPv6Address]; ok {
		if parsed := net.ParseIP(v6addr); parsed != nil {
			s.IPv6Address = parsed
		}
	}
	if prefix, ok := s.Attributes[aaa.AttrIPv6Prefix]; ok {
		if _, ipnet, err := net.ParseCIDR(prefix); err == nil {
			s.IPv6Prefix = ipnet
		}
	}
}

// resolveServiceGroup merges AAA service-group attributes with the
// configured default-service-group on the LNS subscriber-group.
// Called with s.mu held.
func (c *Component) resolveServiceGroup(s *Session, aaaAttrs map[string]interface{}) {
	if c.svcGroupResolver == nil {
		return
	}
	var sgName, defaultSG string
	if v, ok := aaaAttrs[aaa.AttrServiceGroup]; ok {
		if str, ok := v.(string); ok {
			sgName = str
		}
	}
	if sg := c.resolveLNSSubscriberGroup(); sg != nil {
		defaultSG = sg.DefaultServiceGroup
	}
	s.ServiceGroup = c.svcGroupResolver.Resolve(sgName, defaultSG, aaaAttrs)
	s.VRF = s.ServiceGroup.VRF
}

// buildAllocContext mirrors the PPPoE allocator-context construction.
// The LNS subscriber-group's ipv4-profile / ipv6-profile + AAA-returned
// pool overrides drive the allocation. Called with s.mu held.
func (c *Component) buildAllocContext(s *Session, aaaAttrs map[string]interface{}) *allocator.Context {
	var profileName, ipv6ProfileName string
	if sg := c.resolveLNSSubscriberGroup(); sg != nil {
		profileName = sg.IPv4Profile
		ipv6ProfileName = sg.IPv6Profile
	}
	ctx := allocator.NewContext(s.SessionID, nil, 0, 0, s.VRF,
		s.ServiceGroup.Name, profileName, ipv6ProfileName, aaaAttrs)
	if ctx.PoolOverride == "" && s.ServiceGroup.Pool != "" {
		ctx.PoolOverride = s.ServiceGroup.Pool
	}
	if ctx.IANAPoolOverride == "" && s.ServiceGroup.IANAPool != "" {
		ctx.IANAPoolOverride = s.ServiceGroup.IANAPool
	}
	if ctx.PDPoolOverride == "" && s.ServiceGroup.PDPool != "" {
		ctx.PDPoolOverride = s.ServiceGroup.PDPool
	}
	return ctx
}

// resolveLNSSubscriberGroup returns the running-config's LNS-mode
// subscriber-group. v1 supports a single LNS group; if more than one
// is configured the first match wins. Returns nil if no group matches.
func (c *Component) resolveLNSSubscriberGroup() *subscriber.SubscriberGroup {
	if c.cfgMgr == nil {
		return nil
	}
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil || cfg.SubscriberGroups == nil {
		return nil
	}
	for _, g := range cfg.SubscriberGroups.Groups {
		if g != nil && g.AccessType == "lns" {
			return g
		}
	}
	return nil
}

// startNCP allocates IPv4 / IPv6 / PD from the configured pools, plumbs
// the values into the IPCP / IPv6CP FSMs, and drives both layers Open.
// Called with s.mu held.
func (c *Component) startNCP(s *Session) {
	c.log.Debug("L2TP NCP starting", "session_id", s.SessionID)
	if s.IPv4Address == nil {
		c.allocateIPv4(s)
	}
	c.log.Debug("L2TP NCP allocated", "session_id", s.SessionID, "ipv4", s.IPv4Address)
	if s.IPv6Address == nil {
		c.allocateIANA(s)
	}
	if s.IPv6Prefix == nil {
		c.allocatePD(s)
	}

	if s.IPv4Address != nil {
		s.IPCP.SetPeerAddress(s.IPv4Address)
	}

	c.applyIPv4ProfileToIPCP(s)

	if dns1, ok := s.Attributes[aaa.AttrDNSPrimary]; ok {
		var dns1IP, dns2IP net.IP
		dns1IP = net.ParseIP(dns1)
		if dns2, ok := s.Attributes[aaa.AttrDNSSecondary]; ok {
			dns2IP = net.ParseIP(dns2)
		}
		s.IPCP.SetDNS(dns1IP, dns2IP)
	}

	s.IPCP.FSM().Up()
	s.IPCP.FSM().Open()
	s.IPv6CP.FSM().Up()
	s.IPv6CP.FSM().Open()
}

// applyIPv4ProfileToIPCP seeds the LNS-side IPCP options (local
// gateway address, DNS) from the configured IPv4 profile so the
// outbound ConfReq carries real values; AAA-returned attributes still
// override later in startNCP.
func (c *Component) applyIPv4ProfileToIPCP(s *Session) {
	if s.AllocCtx == nil || s.AllocCtx.ProfileName == "" || c.cfgMgr == nil {
		return
	}
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil {
		return
	}
	profile := cfg.IPv4Profiles[s.AllocCtx.ProfileName]
	if profile == nil {
		return
	}
	if gw := net.ParseIP(profile.Gateway); gw != nil {
		s.IPCP.SetAddress(gw)
	}
	var dns1, dns2 net.IP
	for i, d := range profile.DNS {
		ip := net.ParseIP(d)
		if ip == nil {
			continue
		}
		switch i {
		case 0:
			dns1 = ip
		case 1:
			dns2 = ip
		}
	}
	if dns1 != nil || dns2 != nil {
		s.IPCP.SetDNS(dns1, dns2)
	}
}

func (c *Component) allocateIPv4(s *Session) {
	if c.registry == nil || s.AllocCtx == nil || s.AllocCtx.ProfileName == "" {
		return
	}
	ip, pool, err := c.registry.AllocateFromProfile(
		s.AllocCtx.ProfileName, s.AllocCtx.PoolOverride, s.AllocCtx.VRF, s.SessionID,
	)
	if err != nil {
		c.log.Warn("L2TP IPv4 allocation failed",
			"session_id", s.SessionID, "profile", s.AllocCtx.ProfileName, "error", err)
		return
	}
	s.IPv4Address = ip
	s.allocatedPool = pool
	s.AllocCtx.AllocatedPool = pool
}

func (c *Component) allocateIANA(s *Session) {
	if c.registry == nil || s.AllocCtx == nil || s.AllocCtx.IPv6ProfileName == "" {
		return
	}
	ip, pool, err := c.registry.AllocateIANAFromProfile(
		s.AllocCtx.IPv6ProfileName, s.AllocCtx.IANAPoolOverride, s.AllocCtx.VRF, s.SessionID,
	)
	if err != nil {
		c.log.Warn("L2TP IPv6 IANA allocation failed",
			"session_id", s.SessionID, "profile", s.AllocCtx.IPv6ProfileName, "error", err)
		return
	}
	s.IPv6Address = ip
	s.allocatedIANAPool = pool
	s.AllocCtx.AllocatedIANAPool = pool
}

func (c *Component) allocatePD(s *Session) {
	if c.registry == nil || s.AllocCtx == nil || s.AllocCtx.IPv6ProfileName == "" {
		return
	}
	prefix, pool, err := c.registry.AllocatePDFromProfile(
		s.AllocCtx.IPv6ProfileName, s.AllocCtx.PDPoolOverride, s.AllocCtx.VRF, s.SessionID,
	)
	if err != nil {
		return
	}
	s.IPv6Prefix = prefix
	s.allocatedPDPool = pool
	s.AllocCtx.AllocatedPDPool = pool
}

// onIPCPUp records IPCP convergence, programs the per-session VPP
// interface on first NCP up, and publishes SessionStateActive. Called
// with s.mu held.
func (c *Component) onIPCPUp(s *Session) {
	c.log.Debug("L2TP IPCP up", "session_id", s.SessionID, "peer_addr", s.IPCP.PeerConfig().Address)
	s.IPv4Address = s.IPCP.PeerConfig().Address
	s.ipcpOpen = true
	c.checkSessionOpen(s)
}

// onIPCPDown is called with s.mu held.
func (c *Component) onIPCPDown(s *Session) {
	s.ipcpOpen = false
}

// onIPv6CPUp records IPv6CP convergence. No southbound call is needed
// because the per-session vnet interface handles both address families
// once the session is installed. Called with s.mu held.
func (c *Component) onIPv6CPUp(s *Session) {
	s.ipv6cpOpen = true
	c.checkSessionOpen(s)
}

// onIPv6CPDown is called with s.mu held.
func (c *Component) onIPv6CPDown(s *Session) {
	s.ipv6cpOpen = false
}

// checkSessionOpen runs after every NCP up callback. The session
// transitions to PhaseOpen as soon as either NCP converges and the
// Active lifecycle event is emitted exactly once. Note: VPP install
// happens earlier (HandleICRQ → installLNSSessionVPP), so the gate is
// `lifecyclePublished`, not `programmedInVPP`. Called with s.mu held.
func (c *Component) checkSessionOpen(s *Session) {
	if s.Phase != ppp.PhaseNetwork {
		return
	}
	if !s.ipcpOpen && !s.ipv6cpOpen {
		return
	}
	if s.lifecyclePublished {
		return
	}
	s.Phase = ppp.PhaseOpen
	s.BoundAt = time.Now()
	s.ActivatedAt = time.Now()
	s.lifecyclePublished = true

	c.programSessionVPP(s)
	c.publishSessionLifecycle(s, models.SessionStateActive)
}

// installLNSSessionVPP creates the per-session DECAP_IP entry in the
// VPP L2TPv2 plugin at ICRQ time, before LCP / CHAP / NCP negotiate.
// Without this entry the plugin drops every T=0 data packet (NO_SUCH_SESSION)
// so LCP Configure-Request from the LAC's pppd never reaches userspace.
// VRF defaults to 0 because AAA has not yet returned the subscriber's
// service-group; the FIB binding for the IP address is performed in
// programSessionVPP once NCP converges. encap_if_index ~0 means "FIB
// lookup on the peer IP" which is what we want for any LNS reached via
// a routed dataplane.
func (c *Component) installLNSSessionVPP(s *Session) error {
	if c.vpp == nil {
		return nil
	}
	s.mu.Lock()
	if s.programmedInVPP {
		s.mu.Unlock()
		return nil
	}
	s.EncapIfIndex = ^uint32(0)
	t := s.Tunnel
	s.mu.Unlock()

	swIfIndex, err := c.vpp.AddPPPoL2TPSession(
		t.LocalIP, t.PeerIP,
		t.LocalID, s.LocalID, s.PeerID,
		0, ^uint32(0),
		t.PPPHdrSkip,
	)
	if err != nil {
		c.log.Error("L2TP southbound install failed",
			"session_id", s.SessionID, "error", err)
		return err
	}

	s.mu.Lock()
	s.SwIfIndex = swIfIndex
	s.programmedInVPP = true
	s.mu.Unlock()
	return nil
}

// programSessionVPP runs once per session after NCP converges. Same
// shape as the IPoE / PPPoE bring-up:
//   1. set the per-session vnet iface unnumbered to the service-group's
//      loopback — this enables IPv4 / IPv6 on the iface as a side
//      effect of vnet_sw_interface_update_unnumbered, and gives the
//      iface a source IP (the loopback's) for L3 forwarding;
//   2. install the subscriber's /32 v4, /128 v6 IANA, and PD routes
//      via the per-session iface so FIB lookups forward both directions.
func (c *Component) programSessionVPP(s *Session) {
	if c.vpp == nil || s.SwIfIndex == 0 {
		return
	}

	if loopback := s.ServiceGroup.Unnumbered; loopback != "" {
		c.vpp.SetUnnumberedAsync(s.SwIfIndex, loopback, func(err error) {
			if err != nil {
				c.log.Error("L2TP set-unnumbered failed",
					"session_id", s.SessionID, "loopback", loopback, "error", err)
			}
		})
	} else {
		c.log.Warn("L2TP session has no service-group unnumbered loopback; FIB ingress on per-session iface will drop until one is configured",
			"session_id", s.SessionID, "service_group", s.ServiceGroup.Name)
	}

	if s.IPv4Address != nil {
		if err := c.vpp.PPPoL2TPSetSubscriberIPv4(s.SwIfIndex, s.IPv4Address, true); err != nil {
			c.log.Error("L2TP subscriber IPv4 bind failed",
				"session_id", s.SessionID, "ipv4", s.IPv4Address, "error", err)
		}
	}
	if s.IPv6Address != nil {
		if err := c.vpp.PPPoL2TPSetSubscriberIPv6(s.SwIfIndex, s.IPv6Address, true); err != nil {
			c.log.Error("L2TP subscriber IPv6 bind failed",
				"session_id", s.SessionID, "ipv6", s.IPv6Address, "error", err)
		}
	}
	if s.IPv6Prefix != nil {
		nh := s.IPv6Address
		if nh == nil {
			nh = net.IPv6unspecified
		}
		if err := c.vpp.PPPoL2TPSetDelegatedPrefix(s.SwIfIndex, *s.IPv6Prefix, nh, true); err != nil {
			c.log.Error("L2TP delegated prefix bind failed",
				"session_id", s.SessionID, "prefix", s.IPv6Prefix.String(), "error", err)
		}
	}
}

// publishSessionLifecycle emits a SessionLifecycleEvent carrying a
// PPPoL2TPSession snapshot. Consumers include the AAA accounting
// pipeline and the operational state collector. Called with s.mu held.
func (c *Component) publishSessionLifecycle(s *Session, state models.SessionState) {
	if c.eventBus == nil {
		return
	}
	var v6Prefix string
	if s.IPv6Prefix != nil {
		v6Prefix = s.IPv6Prefix.String()
	}
	t := s.Tunnel
	payload := &models.PPPoL2TPSession{
		SessionID:      s.SessionID,
		State:          state,
		AccessType:     string(models.AccessTypeL2TP),
		Protocol:       string(models.ProtocolL2TP),
		AAASessionID:   s.AcctSessionID,
		LocalIP:        t.LocalIP,
		PeerIP:         t.PeerIP,
		LocalTunnelID:  t.LocalID,
		PeerTunnelID:   t.PeerID,
		LocalSessionID: s.LocalID,
		PeerSessionID:  s.PeerID,
		LACHostname:    t.PeerHostname,
		IfIndex:        s.SwIfIndex,
		VRF:            s.VRF,
		ServiceGroup:   s.ServiceGroup.Name,
		SRGName:        s.SRGName,
		IPv4Address:    s.IPv4Address,
		IPv6Address:    s.IPv6Address,
		IPv6Prefix:     v6Prefix,
		IPv4Pool:       s.allocatedPool,
		IANAPool:       s.allocatedIANAPool,
		LCPMagic:       s.LCPMagic,
		Username:       s.Username,
		ActivatedAt:    s.ActivatedAt,
	}
	c.eventBus.Publish(events.TopicSessionLifecycle, events.Event{
		Source: c.Name(),
		Data: &events.SessionLifecycleEvent{
			AccessType: models.AccessTypeL2TP,
			Protocol:   models.ProtocolL2TP,
			SessionID:  s.SessionID,
			State:      state,
			Session:    payload,
		},
	})
}
