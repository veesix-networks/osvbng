package pppoe

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/uuid"
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/ppp"
	"github.com/veesix-networks/osvbng/pkg/pppoe"
	"github.com/veesix-networks/osvbng/pkg/session"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/srg"
)

const (
	defaultACName  = "osvbng"
	cookieTTL      = 60 * time.Second
	pppoeVersion   = 1
	pppoeType      = 1
)

type Component struct {
	*component.Base

	logger   *slog.Logger
	eventBus events.Bus
	srgMgr   *srg.Manager
	cfgMgr   component.ConfigManager
	vpp      *southbound.VPP
	cache    cache.Cache

	acName        string
	cookieMgr     *pppoe.CookieManager
	poolAllocator *PoolAllocator

	sessions  map[string]*SessionState
	sidIndex  map[uint16]*SessionState
	sessionMu sync.RWMutex

	pppoeChan <-chan *dataplane.ParsedPacket

	nextSessionID uint16
	sidMu         sync.Mutex
}

type SessionState struct {
	SessionID      string
	AcctSessionID  string
	PPPoESessionID uint16

	MAC       net.HardwareAddr
	OuterVLAN uint16
	InnerVLAN uint16
	SwIfIndex uint32

	Phase          ppp.Phase
	ServiceName    string
	HostUniq       []byte
	AgentCircuitID string
	AgentRemoteID  string

	IPv4Address net.IP
	IPv6Address net.IP
	IPv6Prefix  *net.IPNet
	DNS1        net.IP
	DNS2        net.IP

	Username   string
	Attributes map[string]string

	CreatedAt time.Time
	LastSeen  time.Time

	lcp    *ppp.LCP
	ipcp   *ppp.IPCP
	ipv6cp *ppp.IPv6CP

	ipcpOpen   bool
	ipv6cpOpen bool

	pap           *ppp.PAPHandler
	chap          *ppp.CHAPHandler
	chapChallenge []byte
	chapID        uint8

	pendingAuthRequestID string
	pendingAuthType      string
	pendingPAPID         uint8
	pendingCHAPID        uint8

	component *Component
	mu        sync.Mutex
}

func New(deps component.Dependencies, srgMgr *srg.Manager) (component.Component, error) {
	log := logger.Component(logger.ComponentPPPoE)

	cookieMgr, err := pppoe.NewCookieManager(cookieTTL)
	if err != nil {
		return nil, fmt.Errorf("create cookie manager: %w", err)
	}

	c := &Component{
		Base:          component.NewBase("pppoe"),
		logger:        log,
		eventBus:      deps.EventBus,
		srgMgr:        srgMgr,
		cfgMgr:        deps.ConfigManager,
		vpp:           deps.VPP,
		cache:         deps.Cache,
		acName:        defaultACName,
		cookieMgr:     cookieMgr,
		poolAllocator: NewPoolAllocator(),
		sessions:      make(map[string]*SessionState),
		sidIndex:      make(map[uint16]*SessionState),
		pppoeChan:     deps.PPPChan,
		nextSessionID: 1,
	}

	return c, nil
}

func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)
	c.logger.Info("Starting PPPoE component", "ac_name", c.acName)

	if err := c.eventBus.Subscribe(events.TopicAAAResponsePPPoE, c.handleAAAResponse); err != nil {
		return fmt.Errorf("subscribe to aaa responses: %w", err)
	}

	c.Go(c.consumePPPoEPackets)

	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.logger.Info("Stopping PPPoE component")
	c.StopContext()
	return nil
}

func (c *Component) consumePPPoEPackets() {
	for {
		select {
		case <-c.Ctx.Done():
			return
		case pkt := <-c.pppoeChan:
			if err := c.handlePacket(pkt); err != nil {
				c.logger.Debug("Error handling PPPoE packet", "error", err)
			}
		}
	}
}

func (c *Component) handlePacket(pkt *dataplane.ParsedPacket) error {
	if pkt.PPPoE == nil {
		return fmt.Errorf("no PPPoE layer")
	}

	switch pkt.Protocol {
	case models.ProtocolPPPoEDiscovery:
		return c.handleDiscovery(pkt)
	case models.ProtocolPPPoESession:
		return c.handleSession(pkt)
	default:
		return fmt.Errorf("unexpected protocol: %v", pkt.Protocol)
	}
}

func (c *Component) handleDiscovery(pkt *dataplane.ParsedPacket) error {
	switch pkt.PPPoE.Code {
	case layers.PPPoECodePADI:
		return c.handlePADI(pkt)
	case layers.PPPoECodePADR:
		return c.handlePADR(pkt)
	case layers.PPPoECodePADT:
		return c.handlePADT(pkt)
	default:
		c.logger.Debug("Ignoring PPPoE discovery code", "code", pkt.PPPoE.Code)
		return nil
	}
}

func (c *Component) handleSession(pkt *dataplane.ParsedPacket) error {
	sid := pkt.PPPoE.SessionId

	c.sessionMu.RLock()
	sess, exists := c.sidIndex[sid]
	c.sessionMu.RUnlock()

	if !exists {
		c.logger.Debug("PPPoE session packet for unknown session", "session_id", sid)
		return nil
	}

	sess.LastSeen = time.Now()

	if pkt.PPP == nil {
		return fmt.Errorf("no PPP layer in session packet")
	}

	return sess.handlePPP(pkt.PPP)
}

func (c *Component) handlePADI(pkt *dataplane.ParsedPacket) error {
	tags, err := pppoe.ParseTags(pkt.PPPoE.Payload)
	if err != nil {
		return fmt.Errorf("parse PADI tags: %w", err)
	}

	c.logger.Info("Received PADI",
		"mac", pkt.MAC.String(),
		"svlan", pkt.OuterVLAN,
		"cvlan", pkt.InnerVLAN,
		"service_name", tags.ServiceName,
		"host_uniq_len", len(tags.HostUniq))

	cookie := c.cookieMgr.Generate(pkt.MAC, pkt.OuterVLAN, pkt.InnerVLAN)

	return c.sendPADO(pkt, tags, cookie)
}

func (c *Component) handlePADR(pkt *dataplane.ParsedPacket) error {
	tags, err := pppoe.ParseTags(pkt.PPPoE.Payload)
	if err != nil {
		return fmt.Errorf("parse PADR tags: %w", err)
	}

	c.logger.Info("Received PADR",
		"mac", pkt.MAC.String(),
		"svlan", pkt.OuterVLAN,
		"cvlan", pkt.InnerVLAN,
		"service_name", tags.ServiceName,
		"ac_cookie_len", len(tags.ACCookie))

	if !c.cookieMgr.Validate(tags.ACCookie, pkt.MAC, pkt.OuterVLAN, pkt.InnerVLAN) {
		c.logger.Warn("Invalid or expired AC-Cookie in PADR",
			"mac", pkt.MAC.String(),
			"svlan", pkt.OuterVLAN)
		return nil
	}

	sessionID := c.allocateSessionID()

	sessID := uuid.New().String()
	sess := &SessionState{
		SessionID:      sessID,
		AcctSessionID:  session.ToAcctSessionID(sessID),
		PPPoESessionID: sessionID,
		MAC:            pkt.MAC,
		OuterVLAN:      pkt.OuterVLAN,
		InnerVLAN:      pkt.InnerVLAN,
		SwIfIndex:      pkt.SwIfIndex,
		Phase:          ppp.PhaseDead,
		ServiceName:    tags.ServiceName,
		HostUniq:       tags.HostUniq,
		AgentCircuitID: tags.AgentCircuitID,
		AgentRemoteID:  tags.AgentRemoteID,
		Attributes:     make(map[string]string),
		CreatedAt:      time.Now(),
		LastSeen:       time.Now(),
		component:      c,
	}

	sess.initPPP()

	key := c.sessionKey(pkt.MAC, pkt.OuterVLAN, pkt.InnerVLAN)

	c.sessionMu.Lock()
	c.sessions[key] = sess
	c.sidIndex[sessionID] = sess
	c.sessionMu.Unlock()

	c.logger.Info("Created PPPoE session",
		"session_id", sess.SessionID,
		"pppoe_session_id", sessionID,
		"mac", pkt.MAC.String(),
		"svlan", pkt.OuterVLAN,
		"cvlan", pkt.InnerVLAN)

	if err := c.sendPADS(pkt, tags, sess); err != nil {
		return err
	}

	sess.up()

	return nil
}

func (c *Component) handlePADT(pkt *dataplane.ParsedPacket) error {
	sid := pkt.PPPoE.SessionId

	c.sessionMu.Lock()
	sess, exists := c.sidIndex[sid]
	if exists {
		key := c.sessionKey(sess.MAC, sess.OuterVLAN, sess.InnerVLAN)
		delete(c.sessions, key)
		delete(c.sidIndex, sid)
	}
	c.sessionMu.Unlock()

	if !exists {
		c.logger.Debug("Received PADT for unknown session", "pppoe_session_id", sid)
		return nil
	}

	c.poolAllocator.Release(sess.SessionID)

	c.logger.Info("Session terminated by PADT",
		"session_id", sess.SessionID,
		"pppoe_session_id", sid,
		"mac", sess.MAC.String())

	return nil
}

func (c *Component) sendPADO(pkt *dataplane.ParsedPacket, reqTags *pppoe.Tags, cookie []byte) error {
	tagBuilder := pppoe.NewTagBuilder().
		AddServiceName(reqTags.ServiceName).
		AddACName(c.acName).
		AddACCookie(cookie)

	if len(reqTags.HostUniq) > 0 {
		tagBuilder.AddHostUniq(reqTags.HostUniq)
	}
	if len(reqTags.RelaySessionID) > 0 {
		tagBuilder.AddRelaySessionID(reqTags.RelaySessionID)
	}

	payload := tagBuilder.Build()

	return c.sendDiscoveryPacket(pkt, layers.PPPoECodePADO, 0, payload)
}

func (c *Component) sendPADS(pkt *dataplane.ParsedPacket, reqTags *pppoe.Tags, sess *SessionState) error {
	tagBuilder := pppoe.NewTagBuilder().
		AddServiceName(reqTags.ServiceName)

	if len(reqTags.HostUniq) > 0 {
		tagBuilder.AddHostUniq(reqTags.HostUniq)
	}
	if len(reqTags.RelaySessionID) > 0 {
		tagBuilder.AddRelaySessionID(reqTags.RelaySessionID)
	}

	payload := tagBuilder.Build()

	return c.sendDiscoveryPacket(pkt, layers.PPPoECodePADS, sess.PPPoESessionID, payload)
}

func (c *Component) sendPADT(sess *SessionState) error {
	payload := pppoe.NewTagBuilder().Build()

	pkt := &dataplane.ParsedPacket{
		MAC:       sess.MAC,
		OuterVLAN: sess.OuterVLAN,
		InnerVLAN: sess.InnerVLAN,
	}

	return c.sendDiscoveryPacket(pkt, layers.PPPoECodePADT, sess.PPPoESessionID, payload)
}

func (c *Component) sendDiscoveryPacket(pkt *dataplane.ParsedPacket, code layers.PPPoECode, sessionID uint16, payload []byte) error {
	pppoeLayer := &layers.PPPoE{
		Version:   pppoeVersion,
		Type:      pppoeType,
		Code:      code,
		SessionId: sessionID,
		Length:    uint16(len(payload)),
	}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true}

	if err := gopacket.SerializeLayers(buf, opts, pppoeLayer, gopacket.Payload(payload)); err != nil {
		return fmt.Errorf("serialize PPPoE: %w", err)
	}

	rawPPPoE := buf.Bytes()

	var srcMAC string
	if c.srgMgr != nil {
		if vmac := c.srgMgr.GetVirtualMAC(pkt.OuterVLAN); vmac != nil {
			srcMAC = vmac.String()
		}
	}
	if srcMAC == "" && c.vpp != nil {
		if ifMac := c.vpp.GetParentInterfaceMAC(); ifMac != nil {
			srcMAC = ifMac.String()
		}
	}
	if srcMAC == "" {
		return fmt.Errorf("no source MAC available for SVLAN %d", pkt.OuterVLAN)
	}

	egressPayload := &models.EgressPacketPayload{
		DstMAC:    pkt.MAC.String(),
		SrcMAC:    srcMAC,
		OuterVLAN: pkt.OuterVLAN,
		InnerVLAN: pkt.InnerVLAN,
		RawData:   rawPPPoE,
	}

	egressEvent := models.Event{
		Type:       models.EventTypeEgress,
		AccessType: models.AccessTypePPPoE,
		Protocol:   models.ProtocolPPPoEDiscovery,
	}
	egressEvent.SetPayload(egressPayload)

	c.logger.Info("Sending PPPoE discovery packet",
		"code", code.String(),
		"session_id", sessionID,
		"dst_mac", pkt.MAC.String(),
		"svlan", pkt.OuterVLAN,
		"payload_len", len(payload))

	if err := c.eventBus.Publish(events.TopicEgress, egressEvent); err != nil {
		return fmt.Errorf("publish egress: %w", err)
	}

	return nil
}

func (c *Component) handleAAAResponse(event models.Event) error {
	var resp models.AAAResponse
	if err := event.GetPayload(&resp); err != nil {
		return fmt.Errorf("failed to decode AAA response: %w", err)
	}

	c.sessionMu.RLock()
	var sess *SessionState
	for _, s := range c.sessions {
		if s.pendingAuthRequestID == resp.RequestID {
			sess = s
			break
		}
	}
	c.sessionMu.RUnlock()

	if sess == nil {
		c.logger.Debug("AAA response for unknown request", "request_id", resp.RequestID)
		return nil
	}

	c.logger.Info("Received AAA response",
		"session_id", sess.SessionID,
		"request_id", resp.RequestID,
		"allowed", resp.Allowed)

	sess.mu.Lock()
	if sess.pendingAuthRequestID != resp.RequestID {
		sess.mu.Unlock()
		return nil
	}
	sess.onAuthResult(resp.Allowed, resp.Attributes)
	sess.mu.Unlock()

	return nil
}

func (c *Component) allocateSessionID() uint16 {
	c.sidMu.Lock()
	defer c.sidMu.Unlock()

	startID := c.nextSessionID
	for {
		sid := c.nextSessionID
		c.nextSessionID++
		if c.nextSessionID == 0 {
			c.nextSessionID = 1
		}

		c.sessionMu.RLock()
		_, exists := c.sidIndex[sid]
		c.sessionMu.RUnlock()

		if !exists {
			return sid
		}

		if c.nextSessionID == startID {
			c.logger.Error("No available PPPoE session IDs")
			return 0
		}
	}
}

func (c *Component) sessionKey(mac net.HardwareAddr, svlan, cvlan uint16) string {
	return fmt.Sprintf("%s:%d:%d", mac.String(), svlan, cvlan)
}
