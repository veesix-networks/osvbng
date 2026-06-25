package pppoe

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/uuid"
	pppdisp "github.com/veesix-networks/osvbng/internal/ppp"
	"github.com/veesix-networks/osvbng/internal/ra"
	"github.com/veesix-networks/osvbng/pkg/aaa"
	"github.com/veesix-networks/osvbng/pkg/allocator"
	aaacfg "github.com/veesix-networks/osvbng/pkg/config/aaa"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/ppp"
	"github.com/veesix-networks/osvbng/pkg/svcgroup"
)

func (s *SessionState) initPPP() {
	s.lcp = ppp.NewLCP(ppp.Callbacks{
		Send:      s.sendLCP,
		LayerUp:   s.onLCPUp,
		LayerDown: s.onLCPDown,
	})

	s.lcp.SetAuthProto(ppp.ProtoCHAP, ppp.CHAPMD5)

	s.ipcp = ppp.NewIPCP(ppp.Callbacks{
		Send:      s.sendIPCP,
		LayerUp:   s.onIPCPUp,
		LayerDown: s.onIPCPDown,
	})

	s.ipv6cp = ppp.NewIPv6CP(ppp.Callbacks{
		Send:      s.sendIPv6CP,
		LayerUp:   s.onIPv6CPUp,
		LayerDown: s.onIPv6CPDown,
	})

	s.pap = &ppp.PAPHandler{
		Send: s.sendPAP,
	}

	s.chap = &ppp.CHAPHandler{
		Send: s.sendCHAP,
	}

	s.dispatcher = &pppdisp.Dispatcher{
		LCP:        s.lcp,
		IPCP:       s.ipcp,
		IPv6CP:     s.ipv6cp,
		PhaseFn:    func() ppp.Phase { return s.Phase },
		HandlePAP:  s.handlePAPPacket,
		HandleCHAP: s.handleCHAPPacket,
		OnEchoReq: func(id uint8, data []byte) {
			if s.Phase == ppp.PhaseOpen || s.Phase == ppp.PhaseNetwork {
				s.sendLCPEchoReply(id, data)
			}
		},
		OnEchoRep: func(id uint8, _ []byte) {
			if s.component.echoGen != nil {
				s.component.echoGen.HandleEchoReply(s.PPPoESessionID, id)
			}
		},
		OnProtocolReject:   s.handleProtocolReject,
		SendProtocolReject: s.sendProtocolReject,
		HandleIPv6:         s.handleIPv6Packet,
	}
}

func (s *SessionState) up() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Phase = ppp.PhaseEstablish
	s.lcp.FSM().Up()
	s.lcp.FSM().Open()
}

func (s *SessionState) handlePPP(pppLayer *layers.PPP) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	proto := uint16(pppLayer.PPPType)
	s.component.logger.Debug("Received PPP packet",
		"pppoe_session_id", s.PPPoESessionID,
		"proto", fmt.Sprintf("0x%04x", proto),
		"phase", s.Phase.String())

	return s.dispatcher.HandleFrame(proto, pppLayer.Payload)
}

func (s *SessionState) handlePAPPacket(code, id uint8, data []byte) error {
	if s.Phase != ppp.PhaseAuthenticate {
		return nil
	}

	switch code {
	case ppp.PAPAuthReq:
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

		s.publishAAARequest(map[string]string{aaa.AttrPassword: password})
	}
	return nil
}

func (s *SessionState) handleCHAPPacket(code, id uint8, data []byte) error {
	if s.Phase != ppp.PhaseAuthenticate {
		return nil
	}

	switch code {
	case ppp.CHAPResponse:
		s.stopCHAPRetryTimer()
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
		s.chapResponse = append(s.chapResponse[:0], response...)

		s.publishAAARequest(map[string]string{
			aaa.AttrCHAPID:        hex.EncodeToString([]byte{id}),
			aaa.AttrCHAPChallenge: hex.EncodeToString(s.chapChallenge),
			aaa.AttrCHAPResponse:  hex.EncodeToString(response),
		})
	}
	return nil
}

func (s *SessionState) publishAAARequest(attrs map[string]string) {
	requestID := uuid.New().String()
	s.pendingAuthRequestID = requestID

	cfg, _ := s.component.cfgMgr.GetRunning()

	username := s.Username
	var policyName string
	var groupName string
	if cfg != nil && cfg.SubscriberGroups != nil {
		if match, ok := s.component.cfgMgr.LookupSubscriberGroup(s.OuterVLAN, s.InnerVLAN); ok {
			groupName = match.Name
			policyName = match.Group.AAAPolicy
		}
	}
	if policyName != "" && cfg != nil {
		if policy := cfg.AAA.GetPolicyByType(policyName, aaacfg.PolicyTypePPP); policy != nil {
			ctx := &aaacfg.PolicyContext{
				MACAddress:     s.MAC,
				SVLAN:          s.OuterVLAN,
				CVLAN:          s.InnerVLAN,
				AgentCircuitID: s.AgentCircuitID,
				AgentRemoteID:  s.AgentRemoteID,
			}
			expanded, ok := policy.ExpandFormatChecked(ctx)
			if !ok && policy.Format != "" {
				s.component.logger.Warn("AAA username empty after policy expansion; failing PPPoE auth",
					"session_id", s.SessionID, "policy", policyName, "group", groupName,
					"mac", s.MAC.String(), "svlan", s.OuterVLAN, "cvlan", s.InnerVLAN,
					"format", policy.Format, "agent_circuit_id", s.AgentCircuitID,
					"agent_remote_id", s.AgentRemoteID, "auth_type", s.pendingAuthType)
				aaa.UsernameEmptyDrops.WithLabelValues(policyName, groupName, "pppoe").Inc()
				switch s.pendingAuthType {
				case "pap":
					s.sendPAPNak(s.pendingPAPID)
				case "chap":
					s.sendCHAPFailure(s.pendingCHAPID)
				}
				s.lcp.FSM().Close()
				s.pendingAuthType = ""
				s.pendingAuthRequestID = ""
				return
			}
			if ok {
				username = expanded
			}
			s.component.logger.Debug("Built username from policy",
				"policy", policyName,
				"format", policy.Format,
				"username", username)
		}
	}

	if s.AgentCircuitID != "" {
		attrs[aaa.AttrCircuitID] = s.AgentCircuitID
	}
	if s.AgentRemoteID != "" {
		attrs[aaa.AttrRemoteID] = s.AgentRemoteID
	}

	aaaPayload := &models.AAARequest{
		RequestID:       requestID,
		Username:        username,
		MAC:             s.MAC.String(),
		AcctSessionID:   s.AcctSessionID,
		SVLAN:           s.OuterVLAN,
		CVLAN:           s.InnerVLAN,
		AccessIfIndex:   s.EncapIfIndex,
		AccessInterface: s.component.accessInterfaceName(s.EncapIfIndex),
		PolicyName:      policyName,
		Attributes:      attrs,
	}

	s.component.logger.Debug("Publishing AAA request",
		"session_id", s.SessionID,
		"username", username,
		"auth_type", s.pendingAuthType)

	s.component.eventBus.Publish(events.TopicAAARequest, events.Event{
		Source: s.component.Name(),
		Data: &events.AAARequestEvent{
			AccessType: models.AccessTypePPPoE,
			Protocol:   models.ProtocolPPPoESession,
			SessionID:  s.SessionID,
			Request:    *aaaPayload,
		},
	})
}

func (s *SessionState) onLCPUp() {
	s.component.logger.Debug("LCP up",
		"session_id", s.SessionID,
		"pppoe_session_id", s.PPPoESessionID)

	lcpCfg := s.lcp.LocalConfig()
	if lcpCfg.WantAuth {
		s.Phase = ppp.PhaseAuthenticate
		s.startAuth(lcpCfg.AuthProto)
	} else {
		s.Phase = ppp.PhaseNetwork
		s.startNCP()
	}
}

func (s *SessionState) onLCPDown() {
	s.component.logger.Debug("LCP down",
		"session_id", s.SessionID,
		"pppoe_session_id", s.PPPoESessionID)

	s.Phase = ppp.PhaseEstablish
}

// we can set this as a config variable at some point
const (
	chapRetryTimeout = 3 * time.Second
	chapMaxRetries   = 10
)

func (s *SessionState) startAuth(authProto uint16) {
	s.component.logger.Debug("Starting authentication",
		"session_id", s.SessionID,
		"auth_proto", fmt.Sprintf("0x%04x", authProto))

	if authProto == ppp.ProtoCHAP {
		s.chapRetryCount = 0
		s.sendCHAPChallenge()
	}
}

func (s *SessionState) sendCHAPChallenge() {
	s.chapChallenge = make([]byte, 16)
	rand.Read(s.chapChallenge)
	s.chapID++
	s.chap.SendChallenge(s.chapID, s.chapChallenge, s.component.acName)

	s.stopCHAPRetryTimer()
	s.chapRetryTimer = time.AfterFunc(chapRetryTimeout, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.handleCHAPTimeout()
	})
}

func (s *SessionState) handleCHAPTimeout() {
	if s.Phase != ppp.PhaseAuthenticate {
		return
	}

	s.chapRetryCount++
	if s.chapRetryCount >= chapMaxRetries {
		s.component.logger.Warn("CHAP authentication timeout",
			"session_id", s.SessionID,
			"pppoe_session_id", s.PPPoESessionID)
		s.lcp.FSM().Close()
		return
	}

	s.component.logger.Debug("Retransmitting CHAP challenge",
		"session_id", s.SessionID,
		"retry", s.chapRetryCount)
	s.sendCHAPChallenge()
}

func (s *SessionState) stopCHAPRetryTimer() {
	if s.chapRetryTimer != nil {
		s.chapRetryTimer.Stop()
		s.chapRetryTimer = nil
	}
}

func (s *SessionState) onAuthResult(allowed bool, attributes map[string]interface{}) {
	if allowed {
		for k, v := range attributes {
			if str, ok := v.(string); ok {
				s.Attributes[k] = str
			}
		}

		s.extractIPFromAttributes()
		resolved := s.resolveServiceGroup(attributes)
		s.VRF = resolved.VRF
		s.ServiceGroup = resolved
		s.SRGName = s.component.resolveSRGName(s.OuterVLAN, s.InnerVLAN)
		s.AllocCtx = s.buildAllocContext(attributes)

		if s.shouldTunnelToLAC() {
			s.component.logger.Info("Handing subscriber off to LAC",
				"session_id", s.SessionID, "username", s.Username)
			s.Phase = ppp.PhaseLACTunnelPending
			// Keep pendingAuthType / pendingPAPID / pendingCHAPID
			// populated — handleLACDecision needs them to send PAP-Ack
			// / CHAP-Success once the L2TP tunnel comes up. Clear only
			// the AAA request correlation ID.
			s.pendingAuthRequestID = ""
			s.startLACVPPSessionAdd()
			return
		}

		switch s.pendingAuthType {
		case "pap":
			s.sendPAPAck(s.pendingPAPID)
		case "chap":
			s.sendCHAPSuccess(s.pendingCHAPID)
		}

		s.onAuthSuccess()
	} else {
		s.component.logger.Warn("Authentication failed",
			"session_id", s.SessionID,
			"auth_type", s.pendingAuthType)

		switch s.pendingAuthType {
		case "pap":
			s.sendPAPNak(s.pendingPAPID)
		case "chap":
			s.sendCHAPFailure(s.pendingCHAPID)
		}

		s.lcp.FSM().Close()
	}

	s.pendingAuthType = ""
	s.pendingAuthRequestID = ""
}

func (s *SessionState) extractIPFromAttributes() {
	if ip, ok := s.Attributes[aaa.AttrIPv4Address]; ok {
		if parsed := net.ParseIP(ip); parsed != nil {
			s.IPv4Address = parsed
			s.component.logger.Debug("Got IPv4 from AAA", "ip", ip)
		}
	}

	if dns, ok := s.Attributes[aaa.AttrDNSPrimary]; ok {
		if parsed := net.ParseIP(dns); parsed != nil {
			s.DNS1 = parsed
		}
	}

	if dns, ok := s.Attributes[aaa.AttrDNSSecondary]; ok {
		if parsed := net.ParseIP(dns); parsed != nil {
			s.DNS2 = parsed
		}
	}

	if v6addr, ok := s.Attributes[aaa.AttrIPv6Address]; ok {
		if parsed := net.ParseIP(v6addr); parsed != nil {
			s.IPv6Address = parsed
			s.component.logger.Debug("Got IPv6 address from AAA", "ip", v6addr)
		}
	}

	if prefix, ok := s.Attributes[aaa.AttrIPv6Prefix]; ok {
		if _, ipnet, err := net.ParseCIDR(prefix); err == nil {
			s.IPv6Prefix = ipnet
			s.component.logger.Debug("Got IPv6 prefix from AAA", "prefix", prefix)
		}
	}
}

func (s *SessionState) resolveServiceGroup(aaaAttrs map[string]interface{}) svcgroup.ServiceGroup {
	var sgName string
	if v, ok := aaaAttrs[aaa.AttrServiceGroup]; ok {
		if str, ok := v.(string); ok {
			sgName = str
		}
	}

	var defaultSG string
	if match, ok := s.component.cfgMgr.LookupSubscriberGroup(s.OuterVLAN, s.InnerVLAN); ok {
		defaultSG = match.Group.DefaultServiceGroup
	}

	return s.component.svcGroupResolver.Resolve(sgName, defaultSG, aaaAttrs)
}

func (s *SessionState) buildAllocContext(aaaAttrs map[string]interface{}) *allocator.Context {
	var profileName, ipv6ProfileName string
	if match, ok := s.component.cfgMgr.LookupSubscriberGroup(s.OuterVLAN, s.InnerVLAN); ok {
		profileName = match.Group.IPv4Profile
		ipv6ProfileName = match.Group.IPv6Profile
	}

	ctx := allocator.NewContext(s.SessionID, s.MAC, s.OuterVLAN, s.InnerVLAN, s.VRF, s.ServiceGroup.Name, profileName, ipv6ProfileName, aaaAttrs)

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

func (s *SessionState) sendPAPAck(id uint8) {
	msg := "Login OK"
	data := make([]byte, 1+len(msg))
	data[0] = byte(len(msg))
	copy(data[1:], msg)
	s.pap.Send(ppp.PAPAuthAck, id, data)
}

func (s *SessionState) sendPAPNak(id uint8) {
	msg := "Authentication failed"
	data := make([]byte, 1+len(msg))
	data[0] = byte(len(msg))
	copy(data[1:], msg)
	s.pap.Send(ppp.PAPAuthNak, id, data)
}

func (s *SessionState) sendCHAPSuccess(id uint8) {
	msg := "Welcome"
	s.chap.Send(ppp.CHAPSuccess, id, []byte(msg))
}

func (s *SessionState) sendCHAPFailure(id uint8) {
	msg := "Authentication failed"
	s.chap.Send(ppp.CHAPFailure, id, []byte(msg))
}

func (s *SessionState) onAuthSuccess() {
	s.component.logger.Debug("Authentication successful",
		"session_id", s.SessionID,
		"username", s.Username)

	s.Phase = ppp.PhaseNetwork
	s.startNCP()
}

func (s *SessionState) startNCP() {
	s.component.logger.Debug("Starting NCP",
		"session_id", s.SessionID)

	if s.IPv4Address == nil {
		s.allocateFromPool()
	} else if registry := s.component.registry; registry != nil {
		if err := registry.ReserveIP(s.IPv4Address, s.SessionID); err != nil {
			s.component.logger.Error("IPv4 reservation conflict",
				"session_id", s.SessionID,
				"address", s.IPv4Address,
				"error", err)
			s.IPv4Address = nil
		}
	}

	if s.IPv6Address == nil {
		s.allocateIANAFromPool()
	} else if registry := s.component.registry; registry != nil {
		if err := registry.ReserveIANA(s.IPv6Address, s.SessionID); err != nil {
			s.component.logger.Error("IPv6 IANA reservation conflict",
				"session_id", s.SessionID,
				"address", s.IPv6Address,
				"error", err)
			s.IPv6Address = nil
		}
	}

	if s.IPv6Prefix != nil {
		if registry := s.component.registry; registry != nil {
			if err := registry.ReservePD(s.IPv6Prefix, s.SessionID); err != nil {
				s.component.logger.Error("PD reservation conflict",
					"session_id", s.SessionID,
					"prefix", s.IPv6Prefix,
					"error", err)
				s.IPv6Prefix = nil
			}
		}
	}

	if s.DNS1 == nil {
		s.applyProfileDNS()
	}

	if s.IPv4Address == nil {
		s.IPv4Address = net.ParseIP("100.64.0.1")
	}
	if s.DNS1 == nil {
		s.DNS1 = net.ParseIP("8.8.8.8")
	}
	if s.DNS2 == nil {
		s.DNS2 = net.ParseIP("8.8.4.4")
	}

	s.ipcp.SetPeerAddress(s.IPv4Address)
	s.ipcp.SetDNS(s.DNS1, s.DNS2)

	s.ipcp.FSM().Up()
	s.ipcp.FSM().Open()

	// Set the IPv6CP local interface identifier deterministically from the
	// BNG-side MAC so the negotiated link-local matches the source the RA / NA
	// path advertises (RFC 5072 §5); without this the default IID is random and
	// the advertised gateway would not match the PPP interface identity.
	if bngMAC, _ := s.bngSourceMAC(); len(bngMAC) == 6 {
		s.ipv6cp.SetInterfaceID(ppp.IPv6CPConfigFromMAC(bngMAC).InterfaceID)
	}

	s.ipv6cp.FSM().Up()
	s.ipv6cp.FSM().Open()
}

func (s *SessionState) allocateFromPool() {
	if s.AllocCtx == nil || s.AllocCtx.ProfileName == "" {
		return
	}

	registry := s.component.registry
	if registry == nil {
		return
	}

	allocated, poolName, err := registry.AllocateFromProfile(
		s.AllocCtx.ProfileName,
		s.AllocCtx.PoolOverride,
		s.AllocCtx.VRF,
		s.SessionID,
	)
	if err != nil {
		s.component.logger.Warn("No available pool IPs",
			"session_id", s.SessionID,
			"profile", s.AllocCtx.ProfileName)
		return
	}

	s.IPv4Address = allocated
	s.allocatedPool = poolName
	s.AllocCtx.AllocatedPool = poolName
	s.component.logger.Debug("Allocated IPv4 from pool",
		"session_id", s.SessionID,
		"pool", poolName,
		"ip", allocated)
}

func (s *SessionState) allocateIANAFromPool() {
	if s.AllocCtx == nil || s.AllocCtx.IPv6ProfileName == "" {
		return
	}

	registry := s.component.registry
	if registry == nil {
		return
	}

	allocated, poolName, err := registry.AllocateIANAFromProfile(
		s.AllocCtx.IPv6ProfileName,
		s.AllocCtx.IANAPoolOverride,
		s.AllocCtx.VRF,
		s.SessionID,
	)
	if err != nil {
		s.component.logger.Warn("No available IANA pool addresses",
			"session_id", s.SessionID,
			"profile", s.AllocCtx.IPv6ProfileName)
		return
	}

	s.IPv6Address = allocated
	s.allocatedIANAPool = poolName
	s.AllocCtx.AllocatedIANAPool = poolName
	// Make DHCPv6 reuse this address rather than allocating a second one: when
	// a client later solicits, dhcp.ResolveV6 sees ctx.IPv6Address already set
	// and binds it instead of pulling a fresh address from the pool.
	s.AllocCtx.IPv6Address = allocated
	s.component.logger.Debug("Allocated IPv6 IANA from pool",
		"session_id", s.SessionID,
		"pool", poolName,
		"ip", allocated)
}

func (s *SessionState) applyProfileDNS() {
	if s.AllocCtx == nil || s.AllocCtx.ProfileName == "" {
		return
	}
	cfg, err := s.component.cfgMgr.GetRunning()
	if err != nil || cfg == nil {
		return
	}
	profile := cfg.IPv4Profiles[s.AllocCtx.ProfileName]
	if profile == nil || len(profile.DNS) == 0 {
		return
	}
	for i, d := range profile.DNS {
		if dnsIP := net.ParseIP(d); dnsIP != nil {
			switch i {
			case 0:
				s.DNS1 = dnsIP
			case 1:
				s.DNS2 = dnsIP
				return
			}
		}
	}
}

func (s *SessionState) onIPCPUp() {
	s.component.logger.Debug("IPCP up",
		"session_id", s.SessionID,
		"ipv4", s.ipcp.PeerConfig().Address)

	s.IPv4Address = s.ipcp.PeerConfig().Address
	s.ipcpOpen = true
	s.checkOpen()
}

func (s *SessionState) onIPCPDown() {
	s.component.logger.Debug("IPCP down", "session_id", s.SessionID)
	s.ipcpOpen = false
}

func (s *SessionState) onIPv6CPUp() {
	s.component.logger.Debug("IPv6CP up", "session_id", s.SessionID)
	s.ipv6cpOpen = true
	s.component.placeSessionInRABucket(s)
	s.checkOpen()
}

func (s *SessionState) onIPv6CPDown() {
	s.component.logger.Debug("IPv6CP down", "session_id", s.SessionID)
	s.ipv6cpOpen = false
	s.component.removeSessionFromRABucket(s)
	s.nextRADue = time.Time{}
}

func (s *SessionState) checkOpen() {
	s.component.logger.Debug("checkOpen called",
		"session_id", s.SessionID,
		"phase", s.Phase,
		"ipcp_open", s.ipcpOpen,
		"ipv6cp_open", s.ipv6cpOpen)

	if s.Phase == ppp.PhaseNetwork {
		if s.ipcpOpen || s.ipv6cpOpen {
			s.Phase = ppp.PhaseOpen
			s.BoundAt = time.Now()
			s.LCPMagic = s.lcp.LocalConfig().Magic
			s.component.logger.Debug("Session open",
				"session_id", s.SessionID,
				"pppoe_session_id", s.PPPoESessionID,
				"ipv4", s.IPv4Address)

			pppMTU, policy := s.component.resolveMSSClampPolicy(s)
			s.NegotiatedPPPMTU = pppMTU
			if policy.Enabled {
				s.IPv4MSS = policy.IPv4MSS
				s.IPv6MSS = policy.IPv6MSS
			}

			s.component.checkpointSession(s)

			s.component.publishSessionLifecycle(&models.PPPSession{
				SessionID:        s.SessionID,
				State:            models.SessionStateActive,
				AccessType:       string(models.AccessTypePPPoE),
				Protocol:         string(models.ProtocolPPPoESession),
				PPPSessionID:     s.PPPoESessionID,
				MAC:              s.MAC,
				OuterVLAN:        s.OuterVLAN,
				InnerVLAN:        s.InnerVLAN,
				IfIndex:          s.SwIfIndex,
				VRF:              s.VRF,
				ServiceGroup:     s.ServiceGroup.Name,
				SRGName:          s.SRGName,
				IPv4Address:      s.IPv4Address,
				IPv6Address:      s.IPv6Address,
				Username:         s.Username,
				AAASessionID:     s.AcctSessionID,
				ActivatedAt:      time.Now(),
				IPv4Pool:         s.allocatedPool,
				IANAPool:         s.allocatedIANAPool,
				NegotiatedPPPMTU: s.NegotiatedPPPMTU,
				IPv4MSS:          s.IPv4MSS,
				IPv6MSS:          s.IPv6MSS,
			})

			if err := s.component.setupSession(context.TODO(), s, SetupModeFresh); err != nil {
				s.component.logger.Error("setupSession (fresh) failed",
					"session_id", s.SessionID, "error", err)
			}
		}
	}
}

// snapshotForTeardown copies the SessionState fields needed for an out-of-lock
// failure teardown. Caller MUST hold s.mu. The snapshot returns immutable
// copies of the fields tearDownSessionAfterVPPFailure reads, so it can run
// without re-acquiring s.mu and without racing against concurrent PPP frame
// handlers that may still be inside handlePPP() at the moment of failure.
func (s *SessionState) snapshotForTeardown() sessionTeardownSnapshot {
	snap := sessionTeardownSnapshot{
		SessionID:    s.SessionID,
		PPPSessionID: s.PPPoESessionID,
		OuterVLAN:    s.OuterVLAN,
		InnerVLAN:    s.InnerVLAN,
		VRF:          s.VRF,
		ServiceGroup: s.ServiceGroup.Name,
		SRGName:      s.SRGName,
		Username:     s.Username,
		AAASessionID: s.AcctSessionID,
	}
	if len(s.MAC) > 0 {
		snap.MAC = make(net.HardwareAddr, len(s.MAC))
		copy(snap.MAC, s.MAC)
	}
	if s.IPv4Address != nil {
		snap.IPv4Address = make(net.IP, len(s.IPv4Address))
		copy(snap.IPv4Address, s.IPv4Address)
	}
	if s.IPv6Address != nil {
		snap.IPv6Address = make(net.IP, len(s.IPv6Address))
		copy(snap.IPv6Address, s.IPv6Address)
	}
	return snap
}

func (s *SessionState) onVPPSessionCreated(swIfIndex uint32, err error) {
	s.mu.Lock()

	if err != nil {
		snap := s.snapshotForTeardown()
		s.mu.Unlock()
		s.component.logger.Error("Failed to program PPPoE session in VPP, tearing down",
			"session_id", snap.SessionID,
			"pppoe_session_id", snap.PPPSessionID,
			"mac", snap.MAC.String(),
			"error", err)
		s.component.tearDownSessionAfterVPPFailure(s, snap, err)
		return
	}

	defer s.mu.Unlock()

	s.SwIfIndex = swIfIndex
	s.component.logger.Debug("Programmed PPPoE session in VPP",
		"session_id", s.SessionID,
		"sw_if_index", swIfIndex,
		"outer_vlan", s.OuterVLAN,
		"inner_vlan", s.InnerVLAN,
		"ppp_mtu", s.NegotiatedPPPMTU,
		"ipv4_mss", s.IPv4MSS,
		"ipv6_mss", s.IPv6MSS)

	s.component.checkpointSession(s)

	s.component.publishSessionProgrammed(&models.PPPSession{
		SessionID:        s.SessionID,
		State:            models.SessionStateActive,
		AccessType:       string(models.AccessTypePPPoE),
		Protocol:         string(models.ProtocolPPPoESession),
		PPPSessionID:     s.PPPoESessionID,
		MAC:              s.MAC,
		OuterVLAN:        s.OuterVLAN,
		InnerVLAN:        s.InnerVLAN,
		IfIndex:          s.SwIfIndex,
		VRF:              s.VRF,
		ServiceGroup:     s.ServiceGroup.Name,
		SRGName:          s.SRGName,
		IPv4Address:      s.IPv4Address,
		IPv6Address:      s.IPv6Address,
		Username:         s.Username,
		AAASessionID:     s.AcctSessionID,
		IPv4Pool:         s.allocatedPool,
		IANAPool:         s.allocatedIANAPool,
		NegotiatedPPPMTU: s.NegotiatedPPPMTU,
		IPv4MSS:          s.IPv4MSS,
		IPv6MSS:          s.IPv6MSS,
	})

	s.component.setupSessionUnnumbered(s.SessionID, swIfIndex,
		s.component.resolveUnnumberedLoopback(s))

	if s.component.echoGen != nil {
		magic := s.lcp.LocalConfig().Magic
		s.component.echoGen.AddSession(s.PPPoESessionID, magic, 0)
	}
}

func (s *SessionState) sendLCP(code, id uint8, data []byte) {
	s.sendPPPPacket(ppp.ProtoLCP, code, id, data)
}

func (s *SessionState) sendIPCP(code, id uint8, data []byte) {
	s.sendPPPPacket(ppp.ProtoIPCP, code, id, data)
}

func (s *SessionState) sendIPv6CP(code, id uint8, data []byte) {
	s.sendPPPPacket(ppp.ProtoIPv6CP, code, id, data)
}

func (s *SessionState) sendPAP(code, id uint8, data []byte) {
	s.sendPPPPacket(ppp.ProtoPAP, code, id, data)
}

func (s *SessionState) sendCHAP(code, id uint8, data []byte) {
	s.sendPPPPacket(ppp.ProtoCHAP, code, id, data)
}

func (s *SessionState) handleProtocolReject(proto uint16) {
	s.component.logger.Debug("Received Protocol-Reject",
		"pppoe_session_id", s.PPPoESessionID,
		"rejected_proto", fmt.Sprintf("0x%04x", proto))

	switch proto {
	case ppp.ProtoIPCP:
		s.ipcp.FSM().Close()
	case ppp.ProtoIPv6CP:
		s.ipv6cp.FSM().Close()
	}
}

func (s *SessionState) sendLCPEchoReply(id uint8, data []byte) {
	magic := s.lcp.LocalConfig().Magic
	resp := make([]byte, 4)
	binary.BigEndian.PutUint32(resp, magic)
	if len(data) > 4 {
		resp = append(resp, data[4:]...)
	}
	s.sendPPPPacket(ppp.ProtoLCP, ppp.EchoRep, id, resp)
}

func (s *SessionState) sendLCPEchoRequest(id uint8) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// RFC 1661 Section 5.8
	if s.Phase != ppp.PhaseOpen && s.Phase != ppp.PhaseNetwork {
		return
	}

	magic := s.lcp.LocalConfig().Magic
	data := make([]byte, 4)
	binary.BigEndian.PutUint32(data, magic)
	s.sendPPPPacket(ppp.ProtoLCP, ppp.EchoReq, id, data)
}

func (s *SessionState) sendProtocolReject(proto uint16, data []byte) {
	rejData := make([]byte, 2+len(data))
	binary.BigEndian.PutUint16(rejData, proto)
	copy(rejData[2:], data)
	s.sendPPPPacket(ppp.ProtoLCP, ppp.ProtoRej, 0, rejData)
}

func (s *SessionState) sendPPPPacket(proto uint16, code, id uint8, data []byte) {
	pktLen := 4 + len(data)
	pppPayload := make([]byte, 2+pktLen)
	binary.BigEndian.PutUint16(pppPayload[0:2], proto)
	pppPayload[2] = code
	pppPayload[3] = id
	binary.BigEndian.PutUint16(pppPayload[4:6], uint16(pktLen))
	copy(pppPayload[6:], data)
	s.publishSessionFrame(proto, pppPayload)
}

// sendIPv6Packet sends a raw IPv6 datagram over the session as PPP protocol
// 0x0057 (RFC 5072 §2 — a single IPv6 packet, no PPP control header), for the
// in-band RA / NA / DHCPv6 replies the dataplane does not originate.
func (s *SessionState) sendIPv6Packet(ipv6 []byte) {
	pppPayload := make([]byte, 2+len(ipv6))
	binary.BigEndian.PutUint16(pppPayload[0:2], ppp.ProtoIPv6)
	copy(pppPayload[2:], ipv6)
	s.publishSessionFrame(ppp.ProtoIPv6, pppPayload)
}

func (s *SessionState) publishSessionFrame(proto uint16, pppPayload []byte) {
	pppoeLayer := &layers.PPPoE{
		Version:   pppoeVersion,
		Type:      pppoeType,
		Code:      layers.PPPoECodeSession,
		SessionId: s.PPPoESessionID,
		Length:    uint16(len(pppPayload)),
	}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true}
	if err := gopacket.SerializeLayers(buf, opts, pppoeLayer, gopacket.Payload(pppPayload)); err != nil {
		s.component.logger.Error("Failed to serialize PPP packet", "error", err)
		return
	}

	srcMAC, parentSwIfIndex := s.bngSourceMAC()
	if srcMAC == nil {
		s.component.logger.Error("No source MAC available", "svlan", s.OuterVLAN)
		return
	}

	egressPayload := models.EgressPacketPayload{
		DstMAC:    s.MAC.String(),
		SrcMAC:    srcMAC.String(),
		OuterVLAN: s.OuterVLAN,
		InnerVLAN: s.InnerVLAN,
		OuterTPID: s.component.ifMgr.OuterTPID(s.EncapIfIndex),
		SwIfIndex: parentSwIfIndex,
		RawData:   buf.Bytes(),
	}

	s.component.logger.Debug("Sending PPP session frame",
		"proto", fmt.Sprintf("0x%04x", proto),
		"pppoe_session_id", s.PPPoESessionID)

	s.component.eventBus.Publish(events.TopicEgress, events.Event{
		Source: s.component.Name(),
		Data: &events.EgressEvent{
			Protocol: models.ProtocolPPPoESession,
			Packet:   egressPayload,
		},
	})
}

// bngSourceMAC resolves the BNG-side source MAC for egress (the SRG virtual MAC
// when set, else the access parent's hardware MAC) and the parent sw_if_index.
func (s *SessionState) bngSourceMAC() (net.HardwareAddr, uint32) {
	var srcMAC net.HardwareAddr
	var parentSwIfIndex uint32
	if s.component.srgMgr != nil {
		srgName := s.SRGName
		if srgName == "" {
			srgName = s.component.resolveSRGName(s.OuterVLAN, s.InnerVLAN)
		}
		if vmac := s.component.srgMgr.GetVirtualMAC(srgName); vmac != nil {
			srcMAC = vmac
		}
	}
	if s.component.ifMgr != nil {
		if iface := s.component.ifMgr.Get(s.EncapIfIndex); iface != nil {
			parentSwIfIndex = iface.SupSwIfIndex
		}
		if srcMAC == nil {
			if parent := s.component.ifMgr.Get(parentSwIfIndex); parent != nil && len(parent.MAC) >= 6 {
				srcMAC = net.HardwareAddr(parent.MAC[:6])
			}
		}
	}
	return srcMAC, parentSwIfIndex
}

// handleIPv6Packet receives a raw IPv6 datagram punted from the in-band PPP
// 0x0057 path and routes the control-plane messages: Router Solicitation -> RA,
// Neighbor Solicitation -> NA, DHCPv6 (UDP/547) -> the DHCPv6 handler. Bulk IPv6
// data is forwarded by the dataplane and never reaches here. Runs under s.mu.
func (s *SessionState) handleIPv6Packet(payload []byte) error {
	pkt := gopacket.NewPacket(payload, layers.LayerTypeIPv6, gopacket.Default)
	ip6, _ := pkt.Layer(layers.LayerTypeIPv6).(*layers.IPv6)
	if ip6 == nil {
		return nil
	}
	if pkt.Layer(layers.LayerTypeICMPv6RouterSolicitation) != nil {
		return s.processRSPacket(ip6.SrcIP)
	}
	if nsLayer := pkt.Layer(layers.LayerTypeICMPv6NeighborSolicitation); nsLayer != nil {
		return s.processNSPacket(ip6.SrcIP, nsLayer.(*layers.ICMPv6NeighborSolicitation).TargetAddress)
	}
	if udp, _ := pkt.Layer(layers.LayerTypeUDP).(*layers.UDP); udp != nil && udp.DstPort == 547 {
		return s.handleDHCPv6(ip6.SrcIP, udp.Payload)
	}
	return nil
}

// processRSPacket answers a Router Solicitation with an RA sourced from the
// BNG-side PPP link-local (off-link PIO by default, no Source Link-Layer Address
// option on the point-to-point link).
func (s *SessionState) processRSPacket(peerLinkLocal net.IP) error {
	cfg, err := s.component.cfgMgr.GetRunning()
	if err != nil || cfg == nil {
		return nil
	}
	match, ok := s.component.cfgMgr.LookupSubscriberGroup(s.OuterVLAN, s.InnerVLAN)
	if !ok {
		return nil
	}
	bngMAC, _ := s.bngSourceMAC()
	if bngMAC == nil {
		return nil
	}

	dstIP := peerLinkLocal
	if len(dstIP) == 0 || dstIP.IsUnspecified() {
		dstIP = net.ParseIP("ff02::1")
	}

	raConfig, prefixes := ra.ResolveGroupRA(cfg, match.Group)
	raw, err := ra.BuildRARawData(raConfig, prefixes, bngMAC, ra.LinkLocalFromMAC(bngMAC), dstIP, false, s.component.logger)
	if err != nil {
		return err
	}
	s.sendIPv6Packet(raw)
	return nil
}

// processNSPacket answers a Neighbor Solicitation for the BNG-side PPP
// link-local with an NA, serving NUD of the gateway over the point-to-point
// link. Solicitations for any other target are ignored.
func (s *SessionState) processNSPacket(peer, target net.IP) error {
	bngMAC, _ := s.bngSourceMAC()
	if bngMAC == nil {
		return nil
	}
	bngLL := ra.LinkLocalFromMAC(bngMAC)
	if !target.Equal(bngLL) {
		return nil
	}

	solicited := len(peer) != 0 && !peer.IsUnspecified()
	dstIP := peer
	if !solicited {
		dstIP = net.ParseIP("ff02::1")
	}

	raw, err := ra.BuildNARawData(bngLL, bngLL, dstIP, bngMAC, solicited, false)
	if err != nil {
		return err
	}
	s.sendIPv6Packet(raw)
	return nil
}

func (s *SessionState) terminate() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stopCHAPRetryTimer()

	if registry := s.component.registry; registry != nil {
		if s.allocatedPool != "" && s.IPv4Address != nil {
			registry.Release(s.allocatedPool, s.IPv4Address)
		} else if s.IPv4Address != nil {
			registry.ReleaseIP(s.IPv4Address)
		}
		if s.allocatedIANAPool != "" && s.IPv6Address != nil {
			registry.ReleaseIANA(s.allocatedIANAPool, s.IPv6Address)
		} else if s.IPv6Address != nil {
			registry.ReleaseIANAByIP(s.IPv6Address)
		}
		if s.IPv6Prefix != nil {
			registry.ReleasePDByPrefix(s.IPv6Prefix)
		}
	}

	if s.component.echoGen != nil {
		s.component.echoGen.RemoveSession(s.PPPoESessionID)
	}

	if s.Phase == ppp.PhaseOpen || s.Phase == ppp.PhaseNetwork {
		s.ipcp.FSM().Kill()
		s.ipv6cp.FSM().Kill()
	}
	s.lcp.FSM().Kill()
	s.Phase = ppp.PhaseTerminate

	if s.component.vpp != nil && s.SwIfIndex != 0 {
		sessionID := s.SessionID
		s.component.vpp.DeletePPPoESessionAsync(s.PPPoESessionID, s.IPv4Address, s.MAC, func(err error) {
			if err != nil {
				s.component.logger.Warn("Failed to delete PPPoE session from VPP",
					"session_id", sessionID,
					"error", err)
			}
		})
	}

	s.component.deleteSessionCheckpoint(s.SessionID)

	s.component.publishSessionLifecycle(&models.PPPSession{
		SessionID:    s.SessionID,
		State:        models.SessionStateReleased,
		AccessType:   string(models.AccessTypePPPoE),
		Protocol:     string(models.ProtocolPPPoESession),
		PPPSessionID: s.PPPoESessionID,
		MAC:          s.MAC,
		OuterVLAN:    s.OuterVLAN,
		InnerVLAN:    s.InnerVLAN,
		SRGName:      s.SRGName,
		Username:     s.Username,
		AAASessionID: s.AcctSessionID,
	})
}
