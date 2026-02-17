package pppoe

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"sort"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/uuid"
	"inet.af/netaddr"
	"github.com/veesix-networks/osvbng/pkg/allocator"
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/opdb"
	"github.com/veesix-networks/osvbng/pkg/ppp"
	"github.com/veesix-networks/osvbng/pkg/pppoe"
	"github.com/veesix-networks/osvbng/pkg/session"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/srg"
	"github.com/veesix-networks/osvbng/pkg/svcgroup"
	"github.com/veesix-networks/osvbng/pkg/vrfmgr"
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
	ifMgr    *ifmgr.Manager
	cfgMgr   component.ConfigManager
	vpp      southbound.Southbound
	vrfMgr           *vrfmgr.Manager
	svcGroupResolver *svcgroup.Resolver
	cache    cache.Cache
	opdb     opdb.Store

	acName    string
	cookieMgr *pppoe.CookieManager
	echoGen   *EchoGenerator

	sessions  map[string]*SessionState
	sidIndex  map[uint16]*SessionState
	sessionMu sync.RWMutex

	poolAllocators map[string]allocator.Allocator // pool name → allocator
	profilePools   map[string][]string            // profile name → sorted pool names

	pppoeChan <-chan *dataplane.ParsedPacket

	nextSessionID uint16
	sidMu         sync.Mutex
}

type SessionState struct {
	SessionID      string
	AcctSessionID  string
	PPPoESessionID uint16

	MAC          net.HardwareAddr
	OuterVLAN    uint16
	InnerVLAN    uint16
	SwIfIndex    uint32
	EncapIfIndex uint32

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

	LCPMagic  uint32
	CreatedAt time.Time
	BoundAt   time.Time
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
	chapRetryTimer *time.Timer
	chapRetryCount int

	pendingAuthRequestID string
	pendingAuthType      string
	pendingPAPID         uint8
	pendingCHAPID        uint8

	OuterTPID uint16
	VRF       string
	ServiceGroup svcgroup.ServiceGroup

	AllocCtx      *allocator.Context
	allocatedPool string

	component *Component
	mu        sync.Mutex
}

func New(deps component.Dependencies, srgMgr *srg.Manager, ifMgr *ifmgr.Manager) (component.Component, error) {
	log := logger.Get(logger.PPPoE)

	cookieMgr, err := pppoe.NewCookieManager(cookieTTL)
	if err != nil {
		return nil, fmt.Errorf("create cookie manager: %w", err)
	}

	c := &Component{
		Base:          component.NewBase("pppoe"),
		logger:        log,
		eventBus:      deps.EventBus,
		srgMgr:        srgMgr,
		ifMgr:         ifMgr,
		cfgMgr:        deps.ConfigManager,
		vpp:           deps.Southbound,
		vrfMgr:           deps.VRFManager,
		svcGroupResolver: deps.SvcGroupResolver,
		cache:         deps.Cache,
		opdb:          deps.OpDB,
		acName:    defaultACName,
		cookieMgr: cookieMgr,
		sessions:       make(map[string]*SessionState),
		sidIndex:       make(map[uint16]*SessionState),
		poolAllocators: make(map[string]allocator.Allocator),
		profilePools:   make(map[string][]string),
		pppoeChan:      deps.PPPChan,
		nextSessionID: 1,
	}

	c.echoGen = NewEchoGenerator(DefaultEchoConfig(), c.sendEchoRequest, c.handleDeadPeer)

	return c, nil
}

func (c *Component) resolveOuterTPID(svlan uint16) uint16 {
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil || cfg.SubscriberGroups == nil {
		return 0x88A8
	}
	group, _ := cfg.SubscriberGroups.FindGroupBySVLAN(svlan)
	if group == nil {
		return 0x88A8
	}
	return group.GetOuterTPID()
}

func (c *Component) getSessionOuterTPID(sess *SessionState) uint16 {
	if sess.OuterTPID != 0 {
		return sess.OuterTPID
	}
	tpid := c.resolveOuterTPID(sess.OuterVLAN)
	sess.OuterTPID = tpid
	return tpid
}

func (c *Component) initPoolAllocators() {
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil || cfg.DHCP.Profiles == nil {
		return
	}

	for profileName, profile := range cfg.DHCP.Profiles {
		type poolRef struct {
			name     string
			priority int
		}
		refs := make([]poolRef, len(profile.Pools))
		for i, p := range profile.Pools {
			refs[i] = poolRef{p.Name, p.Priority}
		}
		sort.Slice(refs, func(i, j int) bool {
			return refs[i].priority < refs[j].priority
		})

		poolNames := make([]string, len(refs))
		for i, r := range refs {
			poolNames[i] = r.name
		}
		c.profilePools[profileName] = poolNames

		for _, pool := range profile.Pools {
			if _, exists := c.poolAllocators[pool.Name]; exists {
				continue
			}

			prefix, err := netaddr.ParseIPPrefix(pool.Network)
			if err != nil {
				c.logger.Warn("Invalid pool network", "pool", pool.Name, "error", err)
				continue
			}

			rangeStart := pool.RangeStart
			rangeEnd := pool.RangeEnd
			if rangeStart == "" {
				rangeStart = prefix.Range().From().Next().String()
			}
			if rangeEnd == "" {
				rangeEnd = prefix.Range().To().Prior().String()
			}

			rs, err := netip.ParseAddr(rangeStart)
			if err != nil {
				c.logger.Warn("Invalid pool range start", "pool", pool.Name, "error", err)
				continue
			}
			re, err := netip.ParseAddr(rangeEnd)
			if err != nil {
				c.logger.Warn("Invalid pool range end", "pool", pool.Name, "error", err)
				continue
			}

			gateway := pool.Gateway
			if gateway == "" {
				gateway = profile.Gateway
			}
			var excludeAddrs []netip.Addr
			if gw, err := netip.ParseAddr(gateway); err == nil {
				excludeAddrs = append(excludeAddrs, gw)
			}

			alloc := allocator.NewPoolAllocator(rs, re, excludeAddrs)
			c.poolAllocators[pool.Name] = alloc
			c.logger.Debug("Initialized pool allocator",
				"pool", pool.Name,
				"range", rangeStart+"-"+rangeEnd,
				"available", alloc.Available())
		}
	}
}

func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)
	c.logger.Info("Starting PPPoE component", "ac_name", c.acName)

	c.initPoolAllocators()

	if err := c.restoreSessions(ctx); err != nil {
		c.logger.Warn("Failed to restore sessions from OpDB", "error", err)
	}

	if err := c.eventBus.Subscribe(events.TopicAAAResponsePPPoE, c.handleAAAResponse); err != nil {
		return fmt.Errorf("subscribe to aaa responses: %w", err)
	}

	c.echoGen.Start()
	c.Go(c.consumePPPoEPackets)

	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.logger.Info("Stopping PPPoE component")
	c.echoGen.Stop()
	c.StopContext()
	return nil
}

func (c *Component) consumePPPoEPackets() {
	for {
		select {
		case <-c.Ctx.Done():
			return
		case pkt := <-c.pppoeChan:
			go func(pkt *dataplane.ParsedPacket) {
				if err := c.handlePacket(pkt); err != nil {
					c.logger.Debug("Error handling PPPoE packet", "error", err)
				}
			}(pkt)
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

	c.logger.Debug("Received PADI",
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

	c.logger.Debug("Received PADR",
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
		EncapIfIndex:   pkt.SwIfIndex,
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

	c.logger.Debug("Created PPPoE session",
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

	sess.terminate()

	c.logger.Debug("Session terminated by PADT",
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
	var parentSwIfIndex uint32
	if c.srgMgr != nil {
		if vmac := c.srgMgr.GetVirtualMAC(pkt.OuterVLAN); vmac != nil {
			srcMAC = vmac.String()
		}
	}
	if c.ifMgr != nil {
		if iface := c.ifMgr.Get(pkt.SwIfIndex); iface != nil {
			parentSwIfIndex = iface.SupSwIfIndex
		}
		if srcMAC == "" {
			if parent := c.ifMgr.Get(parentSwIfIndex); parent != nil && len(parent.MAC) >= 6 {
				srcMAC = net.HardwareAddr(parent.MAC[:6]).String()
			}
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
		OuterTPID: c.resolveOuterTPID(pkt.OuterVLAN),
		SwIfIndex: parentSwIfIndex,
		RawData:   rawPPPoE,
	}

	egressEvent := models.Event{
		Type:       models.EventTypeEgress,
		AccessType: models.AccessTypePPPoE,
		Protocol:   models.ProtocolPPPoEDiscovery,
	}
	egressEvent.SetPayload(egressPayload)

	c.logger.Debug("Sending PPPoE discovery packet",
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

	c.logger.Debug("Received AAA response",
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

func (c *Component) sendEchoRequest(sessionID uint16, echoID uint8) {
	c.sessionMu.RLock()
	sess, exists := c.sidIndex[sessionID]
	c.sessionMu.RUnlock()

	if !exists {
		return
	}

	sess.sendLCPEchoRequest(echoID)
}

func (c *Component) handleDeadPeer(sessionID uint16) {
	c.sessionMu.Lock()
	sess, exists := c.sidIndex[sessionID]
	if exists {
		key := c.sessionKey(sess.MAC, sess.OuterVLAN, sess.InnerVLAN)
		delete(c.sessions, key)
		delete(c.sidIndex, sessionID)
	}
	c.sessionMu.Unlock()

	if !exists {
		return
	}

	c.logger.Debug("Terminating session due to dead peer",
		"session_id", sess.SessionID,
		"pppoe_session_id", sessionID)

	c.sendPADT(sess)
	sess.terminate()
}

func (c *Component) publishSessionLifecycle(payload models.SubscriberSession) error {
	lifecycleEvent := models.Event{
		Type:       models.EventTypeSessionLifecycle,
		AccessType: payload.GetAccessType(),
		Protocol:   payload.GetProtocol(),
		SessionID:  payload.GetSessionID(),
	}
	lifecycleEvent.SetPayload(payload)
	return c.eventBus.Publish(events.TopicSessionLifecycle, lifecycleEvent)
}

func (c *Component) checkpointSession(sess *SessionState) {
	if c.opdb == nil {
		return
	}

	data, err := json.Marshal(sess)
	if err != nil {
		c.logger.Warn("Failed to marshal session for checkpoint", "session_id", sess.SessionID, "error", err)
		return
	}

	if err := c.opdb.Put(c.Ctx, opdb.NamespacePPPoESessions, sess.SessionID, data); err != nil {
		c.logger.Warn("Failed to checkpoint session", "session_id", sess.SessionID, "error", err)
	}
}

func (c *Component) deleteSessionCheckpoint(sessionID string) {
	if c.opdb == nil {
		return
	}

	if err := c.opdb.Delete(c.Ctx, opdb.NamespacePPPoESessions, sessionID); err != nil {
		c.logger.Warn("Failed to delete session checkpoint", "session_id", sessionID, "error", err)
	}
}

func (c *Component) restoreSessions(ctx context.Context) error {
	if c.opdb == nil {
		return nil
	}

	var count, expired, stale int
	now := time.Now()

	validIfIndexes := make(map[uint32]bool)
	if c.vpp != nil {
		if ifaces, err := c.vpp.DumpInterfaces(); err == nil {
			for _, iface := range ifaces {
				validIfIndexes[iface.SwIfIndex] = true
			}
		}
	}

	err := c.opdb.Load(ctx, opdb.NamespacePPPoESessions, func(key string, value []byte) error {
		var sess SessionState
		if err := json.Unmarshal(value, &sess); err != nil {
			c.logger.Warn("Failed to unmarshal session from opdb", "key", key, "error", err)
			return nil
		}

		if c.isSessionExpired(&sess, now) {
			c.opdb.Delete(ctx, opdb.NamespacePPPoESessions, key)
			expired++
			return nil
		}

		if sess.SwIfIndex != 0 && !validIfIndexes[sess.SwIfIndex] {
			c.logger.Info("VPP interface not found, deleting stale PPPoE session",
				"session_id", sess.SessionID,
				"stale_sw_if_index", sess.SwIfIndex)
			c.opdb.Delete(ctx, opdb.NamespacePPPoESessions, key)
			stale++
			return nil
		}

		lookupKey := c.sessionKey(sess.MAC, sess.OuterVLAN, sess.InnerVLAN)

		c.sessionMu.Lock()
		sess.component = c
		sess.initPPP()
		if sess.LCPMagic != 0 {
			sess.lcp.SetMagic(sess.LCPMagic)
		}
		c.sessions[lookupKey] = &sess
		if sess.PPPoESessionID > 0 {
			c.sidIndex[sess.PPPoESessionID] = &sess
			if sess.PPPoESessionID >= c.nextSessionID {
				c.nextSessionID = sess.PPPoESessionID + 1
			}
		}
		c.sessionMu.Unlock()

		if sess.Phase == ppp.PhaseOpen {
			c.restoreSessionToCache(ctx, &sess, now)
			if c.echoGen != nil {
				c.echoGen.AddSession(sess.PPPoESessionID, sess.LCPMagic)
			}
		}

		count++
		return nil
	})

	if err != nil {
		return fmt.Errorf("restore pppoe sessions: %w", err)
	}

	c.logger.Info("Restored PPPoE sessions from OpDB", "count", count, "expired", expired, "stale_vpp", stale)
	return nil
}

func (c *Component) isSessionExpired(sess *SessionState, now time.Time) bool {
	if sess.Phase != ppp.PhaseOpen {
		return false
	}

	if sess.BoundAt.IsZero() {
		return false
	}

	// PPPoE sessions don't have a lease time like DHCP, but we can use a reasonable timeout
	// based on echo keepalive expectations. If the session hasn't been seen in a long time,
	// consider it expired. Default to 24 hours if no recent activity.
	maxAge := 24 * time.Hour
	return now.Sub(sess.BoundAt) > maxAge
}

func (c *Component) restoreSessionToCache(ctx context.Context, sess *SessionState, now time.Time) {
	cacheKey := fmt.Sprintf("osvbng:sessions:%s", sess.SessionID)

	pppSess := &models.PPPSession{
		SessionID:       sess.SessionID,
		PPPSessionID:    sess.PPPoESessionID,
		State:           models.SessionStateActive,
		MAC:             sess.MAC,
		OuterVLAN:       sess.OuterVLAN,
		InnerVLAN:       sess.InnerVLAN,
		IfIndex:         sess.SwIfIndex,
		IPv4Address:     sess.IPv4Address,
		IPv6Address:     sess.IPv6Address,
		RADIUSSessionID: sess.AcctSessionID,
	}

	data, err := json.Marshal(pppSess)
	if err != nil {
		c.logger.Warn("Failed to marshal session for cache restore", "session_id", sess.SessionID, "error", err)
		return
	}

	// PPPoE sessions don't have a lease TTL, use remaining time until max age
	maxAge := 24 * time.Hour
	ttl := maxAge - now.Sub(sess.BoundAt)
	if ttl < 0 {
		ttl = 0
	}

	if err := c.cache.Set(ctx, cacheKey, data, ttl); err != nil {
		c.logger.Warn("Failed to restore session to cache", "session_id", sess.SessionID, "error", err)
	}
}
