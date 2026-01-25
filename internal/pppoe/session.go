package pppoe

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/uuid"
	"github.com/veesix-networks/osvbng/pkg/events"
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
		// handled by echo generator
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

		s.publishAAARequest(map[string]string{"password": password})
	}
	return nil
}

func (s *SessionState) handleCHAPPacket(code, id uint8, data []byte) error {
	if s.Phase != ppp.PhaseAuthenticate {
		return nil
	}

	switch code {
	case ppp.CHAPResponse:
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
			"chap-id":        hex.EncodeToString([]byte{id}),
			"chap-challenge": hex.EncodeToString(s.chapChallenge),
			"chap-response":  hex.EncodeToString(response),
		})
	}
	return nil
}

func (s *SessionState) publishAAARequest(attrs map[string]string) {
	requestID := uuid.New().String()
	s.pendingAuthRequestID = requestID

	var nasIP string
	if cfg, err := s.component.cfgMgr.GetRunning(); err == nil && cfg != nil {
		nasIP = cfg.AAA.NASIP
	}

	aaaPayload := &models.AAARequest{
		RequestID:     requestID,
		Username:      s.Username,
		MAC:           s.MAC.String(),
		NASIPAddress:  nasIP,
		NASPort:       uint32(s.OuterVLAN),
		AcctSessionID: s.AcctSessionID,
		Attributes:    attrs,
	}

	aaaEvent := models.Event{
		Type:       models.EventTypeAAARequest,
		AccessType: models.AccessTypePPPoE,
		Protocol:   models.ProtocolPPPoESession,
		SessionID:  s.SessionID,
	}
	aaaEvent.SetPayload(aaaPayload)

	s.component.logger.Info("Publishing AAA request",
		"session_id", s.SessionID,
		"username", s.Username,
		"auth_type", s.pendingAuthType)

	if err := s.component.eventBus.Publish(events.TopicAAARequest, aaaEvent); err != nil {
		s.component.logger.Error("Failed to publish AAA request", "error", err)
	}
}

func (s *SessionState) onLCPUp() {
	s.component.logger.Info("LCP up",
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
	s.component.logger.Info("LCP down",
		"session_id", s.SessionID,
		"pppoe_session_id", s.PPPoESessionID)

	s.Phase = ppp.PhaseEstablish
}

func (s *SessionState) startAuth(authProto uint16) {
	s.component.logger.Info("Starting authentication",
		"session_id", s.SessionID,
		"auth_proto", fmt.Sprintf("0x%04x", authProto))

	if authProto == ppp.ProtoCHAP {
		s.chapChallenge = make([]byte, 16)
		rand.Read(s.chapChallenge)
		s.chapID++
		s.chap.SendChallenge(s.chapID, s.chapChallenge, s.component.acName)
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

		if s.pendingAuthType == "pap" {
			s.sendPAPAck(s.pendingPAPID)
		} else if s.pendingAuthType == "chap" {
			s.sendCHAPSuccess(s.pendingCHAPID)
		}

		s.onAuthSuccess()
	} else {
		s.component.logger.Warn("Authentication failed",
			"session_id", s.SessionID,
			"username", s.Username,
			"auth_type", s.pendingAuthType)

		if s.pendingAuthType == "pap" {
			s.sendPAPNak(s.pendingPAPID)
		} else if s.pendingAuthType == "chap" {
			s.sendCHAPFailure(s.pendingCHAPID)
		}
	}

	s.pendingAuthType = ""
	s.pendingAuthRequestID = ""
}

func (s *SessionState) extractIPFromAttributes() {
	// we need to normalize these attributes (and the map itself) at some point when we refactor the aaa internals to not be so "radius-y"...
	// ideally we then have a translation layer for radius, radius would be a plugin implementing the AuthProvider interface
	if ip, ok := s.Attributes["ipv4_address"]; ok {
		if parsed := net.ParseIP(ip); parsed != nil {
			s.IPv4Address = parsed
			s.component.logger.Debug("Got IPv4 from AAA", "ip", ip)
		}
	}

	if dns, ok := s.Attributes["dns_primary"]; ok {
		if parsed := net.ParseIP(dns); parsed != nil {
			s.DNS1 = parsed
		}
	}

	if dns, ok := s.Attributes["dns_secondary"]; ok {
		if parsed := net.ParseIP(dns); parsed != nil {
			s.DNS2 = parsed
		}
	}

	if prefix, ok := s.Attributes["ipv6_prefix"]; ok {
		if _, ipnet, err := net.ParseCIDR(prefix); err == nil {
			s.IPv6Prefix = ipnet
			s.component.logger.Debug("Got IPv6 prefix from AAA", "prefix", prefix)
		}
	}
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
	s.component.logger.Info("Authentication successful",
		"session_id", s.SessionID,
		"username", s.Username)

	s.Phase = ppp.PhaseNetwork
	s.startNCP()
}

func (s *SessionState) startNCP() {
	s.component.logger.Info("Starting NCP",
		"session_id", s.SessionID)

	if s.IPv4Address == nil {
		s.allocateFromPool()
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
	cfg, err := s.component.cfgMgr.GetRunning()
	if err != nil || cfg == nil || cfg.SubscriberGroups == nil {
		return
	}

	group, _ := cfg.SubscriberGroups.FindGroupBySVLAN(s.OuterVLAN)
	if group == nil || len(group.AddressPools) == 0 {
		return
	}

	ip, dns1, dns2, err := s.component.poolAllocator.Allocate(s.SessionID, group)
	if err != nil {
		s.component.logger.Warn("Pool allocation failed",
			"session_id", s.SessionID,
			"error", err)
		return
	}

	s.IPv4Address = ip
	if dns1 != nil {
		s.DNS1 = dns1
	}
	if dns2 != nil {
		s.DNS2 = dns2
	}

	s.component.logger.Info("Allocated IP from pool",
		"session_id", s.SessionID,
		"ip", ip.String())
}

func (s *SessionState) onIPCPUp() {
	s.component.logger.Info("IPCP up",
		"session_id", s.SessionID,
		"ipv4", s.ipcp.PeerConfig().Address)

	s.IPv4Address = s.ipcp.PeerConfig().Address
	s.checkOpen()
}

func (s *SessionState) onIPCPDown() {
	s.component.logger.Info("IPCP down", "session_id", s.SessionID)
}

func (s *SessionState) onIPv6CPUp() {
	s.component.logger.Info("IPv6CP up", "session_id", s.SessionID)
	s.checkOpen()
}

func (s *SessionState) onIPv6CPDown() {
	s.component.logger.Info("IPv6CP down", "session_id", s.SessionID)
}

func (s *SessionState) checkOpen() {
	if s.Phase == ppp.PhaseNetwork {
		ipcpOpen := s.ipcp.FSM().State() == ppp.Opened
		ipv6cpOpen := s.ipv6cp.FSM().State() == ppp.Opened

		if ipcpOpen || ipv6cpOpen {
			s.Phase = ppp.PhaseOpen
			s.component.logger.Info("Session open",
				"session_id", s.SessionID,
				"pppoe_session_id", s.PPPoESessionID,
				"ipv4", s.IPv4Address)

			// TODO: program VPP dataplane
		}
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

func (s *SessionState) sendLCPEchoReply(id uint8, data []byte) {
	magic := s.lcp.LocalConfig().Magic
	resp := make([]byte, 4)
	binary.BigEndian.PutUint32(resp, magic)
	if len(data) > 4 {
		resp = append(resp, data[4:]...)
	}
	s.sendPPPPacket(ppp.ProtoLCP, ppp.EchoRep, id, resp)
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
	if s.component.srgMgr != nil {
		if vmac := s.component.srgMgr.GetVirtualMAC(s.OuterVLAN); vmac != nil {
			srcMAC = vmac.String()
		}
	}
	if srcMAC == "" && s.component.vpp != nil {
		if ifMac := s.component.vpp.GetParentInterfaceMAC(); ifMac != nil {
			srcMAC = ifMac.String()
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

	if s.Phase == ppp.PhaseOpen || s.Phase == ppp.PhaseNetwork {
		s.ipcp.FSM().Close()
		s.ipv6cp.FSM().Close()
	}
	s.lcp.FSM().Close()
	s.Phase = ppp.PhaseTerminate
	s.component.poolAllocator.Release(s.SessionID)
}
