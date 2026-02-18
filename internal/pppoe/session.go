package pppoe

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/uuid"
	"github.com/veesix-networks/osvbng/pkg/aaa"
	"github.com/veesix-networks/osvbng/pkg/allocator"
	aaacfg "github.com/veesix-networks/osvbng/pkg/config/aaa"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/svcgroup"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/ppp"
)

func (s *SessionState) initPPP() {
	s.lcp = ppp.NewLCP(ppp.Callbacks{
		Send:     s.sendLCP,
		LayerUp:  s.onLCPUp,
		LayerDown: s.onLCPDown,
	})

	s.lcp.SetAuthProto(ppp.ProtoCHAP, ppp.CHAPMD5)

	s.ipcp = ppp.NewIPCP(ppp.Callbacks{
		Send:     s.sendIPCP,
		LayerUp:  s.onIPCPUp,
		LayerDown: s.onIPCPDown,
	})

	s.ipv6cp = ppp.NewIPv6CP(ppp.Callbacks{
		Send:     s.sendIPv6CP,
		LayerUp:  s.onIPv6CPUp,
		LayerDown: s.onIPv6CPDown,
	})

	s.pap = &ppp.PAPHandler{
		Send: s.sendPAP,
	}

	s.chap = &ppp.CHAPHandler{
		Send: s.sendCHAP,
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
	payload := pppLayer.Payload

	if len(payload) < 4 {
		return fmt.Errorf("PPP packet too short")
	}

	code := payload[0]
	id := payload[1]
	length := binary.BigEndian.Uint16(payload[2:4])
	if int(length) > len(payload) {
		return fmt.Errorf("PPP packet length mismatch")
	}
	data := payload[4:length]

	s.component.logger.Debug("Received PPP packet",
		"pppoe_session_id", s.PPPoESessionID,
		"proto", fmt.Sprintf("0x%04x", proto),
		"code", code,
		"phase", s.Phase.String())

	switch proto {
	case ppp.ProtoLCP:
		return s.handleLCP(code, id, data)
	case ppp.ProtoPAP:
		return s.handlePAPPacket(code, id, data)
	case ppp.ProtoCHAP:
		return s.handleCHAPPacket(code, id, data)
	case ppp.ProtoIPCP:
		if s.Phase != ppp.PhaseNetwork && s.Phase != ppp.PhaseOpen {
			return nil
		}
		s.ipcp.FSM().Input(code, id, data)
	case ppp.ProtoIPv6CP:
		if s.Phase != ppp.PhaseNetwork && s.Phase != ppp.PhaseOpen {
			return nil
		}
		s.ipv6cp.FSM().Input(code, id, data)
	default:
		s.component.logger.Debug("Unknown PPP protocol", "proto", fmt.Sprintf("0x%04x", proto))
		s.sendProtocolReject(proto, payload)
	}

	return nil
}

func (s *SessionState) handleLCP(code, id uint8, data []byte) error {
	switch code {
	case ppp.EchoReq:
		if s.Phase == ppp.PhaseOpen || s.Phase == ppp.PhaseNetwork {
			s.sendLCPEchoReply(id, data)
		}
	case ppp.EchoRep:
		if s.component.echoGen != nil {
			s.component.echoGen.HandleEchoReply(s.PPPoESessionID, id)
		}
	case ppp.ProtoRej:
		// RFC 1661 Section 5.7 - stop sending the rejected protocol
		if len(data) >= 2 {
			rejectedProto := binary.BigEndian.Uint16(data[0:2])
			s.handleProtocolReject(rejectedProto)
		}
	default:
		s.lcp.FSM().Input(code, id, data)
	}
	return nil
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
	if cfg != nil && cfg.SubscriberGroups != nil {
		if group, _ := cfg.SubscriberGroups.FindGroupBySVLAN(s.OuterVLAN); group != nil {
			policyName = group.AAAPolicy
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
			username = policy.ExpandFormat(ctx)
			s.component.logger.Debug("Built username from policy",
				"policy", policyName,
				"format", policy.Format,
				"username", username)
		}
	}

	aaaPayload := &models.AAARequest{
		RequestID:     requestID,
		Username:      username,
		MAC:           s.MAC.String(),
		AcctSessionID: s.AcctSessionID,
		SVLAN:         s.OuterVLAN,
		CVLAN:         s.InnerVLAN,
		PolicyName:    policyName,
		Attributes:    attrs,
	}

	aaaEvent := models.Event{
		Type:       models.EventTypeAAARequest,
		AccessType: models.AccessTypePPPoE,
		Protocol:   models.ProtocolPPPoESession,
		SessionID:  s.SessionID,
	}
	aaaEvent.SetPayload(aaaPayload)

	s.component.logger.Debug("Publishing AAA request",
		"session_id", s.SessionID,
		"username", username,
		"auth_type", s.pendingAuthType)

	if err := s.component.eventBus.Publish(events.TopicAAARequest, aaaEvent); err != nil {
		s.component.logger.Error("Failed to publish AAA request", "error", err)
	}
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
		s.AllocCtx = s.buildAllocContext(attributes)

		logArgs := []any{"session_id", s.SessionID}
		for _, attr := range resolved.LogAttrs() {
			logArgs = append(logArgs, attr.Key, attr.Value.Any())
		}
		s.component.logger.Info("Resolved service group", logArgs...)

		if s.pendingAuthType == "pap" {
			s.sendPAPAck(s.pendingPAPID)
		} else if s.pendingAuthType == "chap" {
			s.sendCHAPSuccess(s.pendingCHAPID)
		}

		s.onAuthSuccess()
	} else {
		s.component.logger.Warn("Authentication failed",
			"session_id", s.SessionID,
			"auth_type", s.pendingAuthType)

		if s.pendingAuthType == "pap" {
			s.sendPAPNak(s.pendingPAPID)
		} else if s.pendingAuthType == "chap" {
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
	cfg, _ := s.component.cfgMgr.GetRunning()
	if cfg != nil && cfg.SubscriberGroups != nil {
		if group, _ := cfg.SubscriberGroups.FindGroupBySVLAN(s.OuterVLAN); group != nil {
			defaultSG = group.DefaultServiceGroup
		}
	}

	return s.component.svcGroupResolver.Resolve(sgName, defaultSG, aaaAttrs)
}

func (s *SessionState) buildAllocContext(aaaAttrs map[string]interface{}) *allocator.Context {
	var profileName string
	cfg, err := s.component.cfgMgr.GetRunning()
	if err == nil && cfg != nil && cfg.SubscriberGroups != nil {
		if group, _ := cfg.SubscriberGroups.FindGroupBySVLAN(s.OuterVLAN); group != nil {
			profileName = group.IPv4Profile
		}
	}

	return allocator.NewContext(s.SessionID, s.MAC, s.OuterVLAN, s.InnerVLAN, s.VRF, s.ServiceGroup.Name, profileName, aaaAttrs)
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
		registry.ReserveIP(s.IPv4Address, s.SessionID)
	}

	if s.IPv6Prefix != nil {
		if registry := s.component.registry; registry != nil {
			registry.ReservePD(s.IPv6Prefix, s.SessionID)
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
	s.component.logger.Debug("Allocated IPv4 from pool",
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
	s.checkOpen()
}

func (s *SessionState) onIPv6CPDown() {
	s.component.logger.Debug("IPv6CP down", "session_id", s.SessionID)
	s.ipv6cpOpen = false
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

			s.component.checkpointSession(s)

			s.component.publishSessionLifecycle(&models.PPPSession{
				SessionID:       s.SessionID,
				State:           models.SessionStateActive,
				AccessType:      string(models.AccessTypePPPoE),
				Protocol:        string(models.ProtocolPPPoESession),
				PPPSessionID:    s.PPPoESessionID,
				MAC:             s.MAC,
				OuterVLAN:       s.OuterVLAN,
				InnerVLAN:       s.InnerVLAN,
				IfIndex:         s.SwIfIndex,
				VRF:             s.VRF,
				IPv4Address:     s.IPv4Address,
				IPv6Address:     s.IPv6Address,
				Username:        s.Username,
				RADIUSSessionID: s.AcctSessionID,
			})

			if s.component.vpp != nil && s.IPv4Address != nil {
				var localMAC net.HardwareAddr
				if s.component.ifMgr != nil {
					if iface := s.component.ifMgr.Get(s.EncapIfIndex); iface != nil && len(iface.MAC) >= 6 {
						localMAC = net.HardwareAddr(iface.MAC[:6])
					}
				}
				if localMAC == nil {
					s.component.logger.Error("Failed to get local MAC",
						"session_id", s.SessionID,
						"sw_if_index", s.EncapIfIndex)
					return
				}

				var decapVrfID uint32
				if s.VRF != "" && s.component.vrfMgr != nil {
					tableID, _, _, err := s.component.vrfMgr.ResolveVRF(s.VRF)
					if err != nil {
						s.component.logger.Error("Failed to resolve VRF for session",
							"session_id", s.SessionID,
							"vrf", s.VRF,
							"error", err)
						return
					}
					decapVrfID = tableID
				}

				s.component.vpp.AddPPPoESessionAsync(
					s.PPPoESessionID,
					s.IPv4Address,
					s.MAC,
					localMAC,
					s.EncapIfIndex,
					s.OuterVLAN,
					s.InnerVLAN,
					decapVrfID,
					s.onVPPSessionCreated,
				)
			} else if s.component.echoGen != nil {
				magic := s.lcp.LocalConfig().Magic
				s.component.echoGen.AddSession(s.PPPoESessionID, magic)
			}
		}
	}
}

func (s *SessionState) onVPPSessionCreated(swIfIndex uint32, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err != nil {
		s.component.logger.Error("Failed to add PPPoE session to VPP",
			"session_id", s.SessionID,
			"error", err)
		return
	}

	s.SwIfIndex = swIfIndex
	s.component.logger.Debug("Programmed PPPoE session in VPP",
		"session_id", s.SessionID,
		"sw_if_index", swIfIndex,
		"outer_vlan", s.OuterVLAN,
		"inner_vlan", s.InnerVLAN)

	if s.component.echoGen != nil {
		magic := s.lcp.LocalConfig().Magic
		s.component.echoGen.AddSession(s.PPPoESessionID, magic)
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

	var srcMAC string
	var parentSwIfIndex uint32
	if s.component.srgMgr != nil {
		if vmac := s.component.srgMgr.GetVirtualMAC(s.OuterVLAN); vmac != nil {
			srcMAC = vmac.String()
		}
	}
	if s.component.ifMgr != nil {
		if iface := s.component.ifMgr.Get(s.EncapIfIndex); iface != nil {
			parentSwIfIndex = iface.SupSwIfIndex
		}
		if srcMAC == "" {
			if parent := s.component.ifMgr.Get(parentSwIfIndex); parent != nil && len(parent.MAC) >= 6 {
				srcMAC = net.HardwareAddr(parent.MAC[:6]).String()
			}
		}
	}
	if srcMAC == "" {
		s.component.logger.Error("No source MAC available", "svlan", s.OuterVLAN)
		return
	}

	egressPayload := &models.EgressPacketPayload{
		DstMAC:    s.MAC.String(),
		SrcMAC:    srcMAC,
		OuterVLAN: s.OuterVLAN,
		InnerVLAN: s.InnerVLAN,
		OuterTPID: s.component.getSessionOuterTPID(s),
		SwIfIndex: parentSwIfIndex,
		RawData:   buf.Bytes(),
	}

	egressEvent := models.Event{
		Type:       models.EventTypeEgress,
		AccessType: models.AccessTypePPPoE,
		Protocol:   models.ProtocolPPPoESession,
	}
	egressEvent.SetPayload(egressPayload)

	s.component.logger.Debug("Sending PPP packet",
		"proto", fmt.Sprintf("0x%04x", proto),
		"code", code,
		"id", id,
		"pppoe_session_id", s.PPPoESessionID)

	if err := s.component.eventBus.Publish(events.TopicEgress, egressEvent); err != nil {
		s.component.logger.Error("Failed to publish egress", "error", err)
	}
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
		SessionID:       s.SessionID,
		State:           models.SessionStateReleased,
		AccessType:      string(models.AccessTypePPPoE),
		Protocol:        string(models.ProtocolPPPoESession),
		PPPSessionID:    s.PPPoESessionID,
		MAC:             s.MAC,
		OuterVLAN:       s.OuterVLAN,
		InnerVLAN:       s.InnerVLAN,
		Username:        s.Username,
		RADIUSSessionID: s.AcctSessionID,
	})
}
