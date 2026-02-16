package ipoe

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
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
	"github.com/veesix-networks/osvbng/pkg/config/aaa"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/dhcp4"
	"github.com/veesix-networks/osvbng/pkg/dhcp6"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/opdb"
	"github.com/veesix-networks/osvbng/pkg/session"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/southbound/vpp"
	"github.com/veesix-networks/osvbng/pkg/srg"
	"github.com/veesix-networks/osvbng/pkg/svcgroup"
	"github.com/veesix-networks/osvbng/pkg/vrfmgr"
)

type Component struct {
	*component.Base

	logger        *slog.Logger
	eventBus      events.Bus
	srgMgr        *srg.Manager
	ifMgr         *ifmgr.Manager
	cfgMgr        component.ConfigManager
	vpp           *vpp.VPP
	vrfMgr           *vrfmgr.Manager
	svcGroupResolver *svcgroup.Resolver
	cache         cache.Cache
	opdb          opdb.Store
	dhcp4Provider dhcp4.DHCPProvider
	dhcp6Provider dhcp6.DHCPProvider
	sessions      map[string]*SessionState
	xidIndex      map[uint32]*SessionState
	xid6Index     map[[3]byte]*SessionState
	sessionIndex  map[string]*SessionState
	sessionMu     sync.RWMutex

	dhcpChan    <-chan *dataplane.ParsedPacket
	dhcp6Chan   <-chan *dataplane.ParsedPacket
	ipv6NDChan  <-chan *dataplane.ParsedPacket
}

type SessionState struct {
	SessionID           string
	AcctSessionID       string
	MAC                 net.HardwareAddr
	OuterVLAN           uint16
	InnerVLAN           uint16
	EncapIfIndex        uint32
	IPoESwIfIndex       uint32
	State               string
	IPv4                net.IP
	LeaseTime           uint32
	BoundAt             time.Time
	XID                 uint32
	Hostname            string
	ClientID            []byte
	CircuitID           []byte
	RemoteID            []byte
	LastSeen            time.Time
	AAAApproved         bool
	IPoESessionCreated  bool
	PendingDHCPDiscover []byte
	PendingDHCPRequest  []byte
	PendingIPv4Binding  net.IP
	PendingIPv6Binding  net.IP
	PendingPDBinding    *net.IPNet

	IPv6Address          net.IP
	IPv6Prefix           *net.IPNet
	ClientLinkLocal      net.IP
	DHCPv6DUID           []byte
	DHCPv6XID            [3]byte
	IPv6LeaseTime        uint32
	IPv6BoundAt          time.Time
	IPv6Bound            bool
	PendingDHCPv6Solicit []byte
	PendingDHCPv6Request []byte

	Username string
	OuterTPID uint16
	VRF       string
	ServiceGroup svcgroup.ServiceGroup
}

func (c *Component) resolveServiceGroup(svlan uint16, aaaAttrs map[string]interface{}) svcgroup.ServiceGroup {
	var sgName string
	if v, ok := aaaAttrs["service-group"]; ok {
		if s, ok := v.(string); ok {
			sgName = s
		}
	}

	var defaultSG string
	cfg, err := c.cfgMgr.GetRunning()
	if err == nil && cfg != nil && cfg.SubscriberGroups != nil {
		if group, _ := cfg.SubscriberGroups.FindGroupBySVLAN(svlan); group != nil {
			defaultSG = group.DefaultServiceGroup
		}
	}

	return c.svcGroupResolver.Resolve(sgName, defaultSG, aaaAttrs)
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

type raPrefixInfo struct {
	network       string
	validTime     uint32
	preferredTime uint32
}

func New(deps component.Dependencies, srgMgr *srg.Manager, ifMgr *ifmgr.Manager, dhcp4Provider dhcp4.DHCPProvider, dhcp6Provider dhcp6.DHCPProvider) (component.Component, error) {
	log := logger.Get(logger.IPoE)

	c := &Component{
		Base:          component.NewBase("ipoe"),
		logger:        log,
		eventBus:      deps.EventBus,
		srgMgr:        srgMgr,
		ifMgr:         ifMgr,
		vrfMgr:           deps.VRFManager,
		svcGroupResolver: deps.SvcGroupResolver,
		cfgMgr:        deps.ConfigManager,
		vpp:           deps.VPP,
		cache:         deps.Cache,
		opdb:          deps.OpDB,
		dhcp4Provider: dhcp4Provider,
		dhcp6Provider: dhcp6Provider,
		sessions:      make(map[string]*SessionState),
		xidIndex:      make(map[uint32]*SessionState),
		xid6Index:     make(map[[3]byte]*SessionState),
		sessionIndex:  make(map[string]*SessionState),
		dhcpChan:      deps.DHCPChan,
		dhcp6Chan:     deps.DHCPv6Chan,
		ipv6NDChan:    deps.IPv6NDChan,
	}

	return c, nil
}

func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)
	c.logger.Info("Starting IPoE component")

	if err := c.restoreSessions(ctx); err != nil {
		c.logger.Warn("Failed to restore sessions from OpDB", "error", err)
	}

	if err := c.eventBus.Subscribe(events.TopicAAAResponseIPoE, c.handleAAAResponse); err != nil {
		return fmt.Errorf("subscribe to aaa responses: %w", err)
	}

	c.Go(c.cleanupSessions)
	c.Go(c.consumeDHCPPackets)
	c.Go(c.consumeDHCPv6Packets)
	c.Go(c.consumeIPv6NDPackets)

	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.logger.Info("Stopping IPoE component")

	c.eventBus.Unsubscribe(events.TopicAAAResponseIPoE, c.handleAAAResponse)

	c.StopContext()

	return nil
}

func (c *Component) consumeDHCPPackets() {
	for {
		select {
		case <-c.Ctx.Done():
			return
		case pkt := <-c.dhcpChan:
			go func(pkt *dataplane.ParsedPacket) {
				if err := c.processDHCPPacket(pkt); err != nil {
					c.logger.Error("Error processing DHCP packet", "error", err)
				}
			}(pkt)
		}
	}
}

func (c *Component) processDHCPPacket(pkt *dataplane.ParsedPacket) error {

	if pkt.DHCPv4 == nil {
		return fmt.Errorf("no DHCPv4 layer")
	}

	if pkt.OuterVLAN == 0 {
		return fmt.Errorf("packet rejected: S-VLAN required (untagged not supported)")
	}

	isDF := true
	if c.srgMgr != nil {
		isDF = c.srgMgr.IsDF(pkt.OuterVLAN, pkt.MAC.String(), pkt.InnerVLAN)
	}
	if !isDF {
		return nil
	}

	msgType := getDHCPMessageType(pkt.DHCPv4.Options)
	if msgType == layers.DHCPMsgTypeUnspecified {
		return fmt.Errorf("missing DHCP message type")
	}

	c.logger.WithGroup(logger.IPoEDHCP4).Debug("[DF] Received DHCP packet",
		"message_type", msgType.String(),
		"mac", pkt.MAC.String(),
		"xid", fmt.Sprintf("0x%x", pkt.DHCPv4.Xid))

	switch msgType {
	case layers.DHCPMsgTypeDiscover:
		return c.handleDiscover(pkt)
	case layers.DHCPMsgTypeRequest:
		return c.handleRequest(pkt)
	case layers.DHCPMsgTypeRelease:
		return c.handleRelease(pkt)
	case layers.DHCPMsgTypeOffer, layers.DHCPMsgTypeAck, layers.DHCPMsgTypeNak:
		return c.handleServerResponse(pkt)
	}

	return nil
}

func getDHCPMessageType(options layers.DHCPOptions) layers.DHCPMsgType {
	for _, opt := range options {
		if opt.Type == layers.DHCPOptMessageType {
			if len(opt.Data) == 1 {
				return layers.DHCPMsgType(opt.Data[0])
			}
		}
	}
	return layers.DHCPMsgTypeUnspecified
}

func getDHCPOption(options layers.DHCPOptions, optType layers.DHCPOpt) []byte {
	for _, opt := range options {
		if opt.Type == optType {
			return opt.Data
		}
	}
	return nil
}

func parseOption82(data []byte) (circuitID, remoteID []byte) {
	i := 0
	for i < len(data) {
		if i+1 >= len(data) {
			break
		}

		subOptCode := data[i]
		subOptLen := int(data[i+1])

		if i+2+subOptLen > len(data) {
			break
		}

		subOptData := data[i+2 : i+2+subOptLen]

		switch subOptCode {
		case 1:
			circuitID = subOptData
		case 2:
			remoteID = subOptData
		}

		i += 2 + subOptLen
	}
	return
}

func parseDHCPv6RelayOptions(dhcp *layers.DHCPv6) (interfaceID, remoteID []byte) {
	for _, opt := range dhcp.Options {
		switch opt.Code {
		case 18:
			interfaceID = opt.Data
		case 37:
			if len(opt.Data) >= 4 {
				remoteID = opt.Data[4:]
			}
		}
	}
	return
}

func (c *Component) unwrapDHCPv6Relay(pkt *dataplane.ParsedPacket) (*layers.DHCPv6, []byte, []byte) {
	relay := pkt.DHCPv6
	interfaceID, remoteID := parseDHCPv6RelayOptions(relay)

	c.logger.Info("DHCPv6 relay message",
		"hop_count", relay.HopCount,
		"link_addr", relay.LinkAddr,
		"peer_addr", relay.PeerAddr,
		"interface_id", string(interfaceID),
		"remote_id", string(remoteID))

	for _, opt := range relay.Options {
		if opt.Code == 9 {
			innerPkt := gopacket.NewPacket(opt.Data, layers.LayerTypeDHCPv6, gopacket.Default)
			if layer := innerPkt.Layer(layers.LayerTypeDHCPv6); layer != nil {
				inner := layer.(*layers.DHCPv6)
				c.logger.Info("Unwrapped inner DHCPv6 message",
					"inner_type", inner.MsgType.String(),
					"xid", fmt.Sprintf("0x%x", inner.TransactionID))
				return inner, interfaceID, remoteID
			}
		}
	}
	return nil, interfaceID, remoteID
}

func (c *Component) handleDiscover(pkt *dataplane.ParsedPacket) error {
	lookupKey := c.makeSessionKeyV4(pkt.MAC, pkt.OuterVLAN, pkt.InnerVLAN)

	c.sessionMu.RLock()
	sess := c.sessions[lookupKey]
	c.sessionMu.RUnlock()

	if sess == nil {
		if err := c.checkSessionLimit(pkt.MAC, pkt.OuterVLAN, pkt.InnerVLAN); err != nil {
			c.logger.WithGroup(logger.IPoEDHCP4).Info("DHCPDISCOVER rejected", "error", err)
			return nil
		}

		sessID := session.GenerateID()
		newSess := &SessionState{
			SessionID:     sessID,
			AcctSessionID: session.ToAcctSessionID(sessID),
			MAC:           pkt.MAC,
			OuterVLAN:     pkt.OuterVLAN,
			InnerVLAN:     pkt.InnerVLAN,
			EncapIfIndex:  pkt.SwIfIndex,
			State:         "discovering",
		}

		c.sessionMu.Lock()
		if existing := c.sessions[lookupKey]; existing != nil {
			sess = existing
		} else {
			sess = newSess
			c.sessions[lookupKey] = sess
			c.sessionIndex[sessID] = sess
		}
		c.sessionMu.Unlock()
	}

	hostname := string(getDHCPOption(pkt.DHCPv4.Options, layers.DHCPOptHostname))
	clientID := getDHCPOption(pkt.DHCPv4.Options, layers.DHCPOptClientID)
	circuitID, remoteID := parseOption82(getDHCPOption(pkt.DHCPv4.Options, 82))

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}
	if err := pkt.DHCPv4.SerializeTo(buf, opts); err != nil {
		return fmt.Errorf("serialize DHCP: %w", err)
	}

	c.sessionMu.Lock()
	sess.XID = pkt.DHCPv4.Xid
	sess.Hostname = hostname
	sess.ClientID = clientID
	sess.CircuitID = circuitID
	sess.RemoteID = remoteID
	sess.LastSeen = time.Now()
	sess.PendingDHCPDiscover = buf.Bytes()
	c.xidIndex[pkt.DHCPv4.Xid] = sess
	alreadyApproved := sess.AAAApproved
	ipoeCreated := sess.IPoESessionCreated
	v6AaaPending := sess.PendingDHCPv6Solicit != nil || sess.PendingDHCPv6Request != nil
	c.sessionMu.Unlock()

	c.logger.WithGroup(logger.IPoEDHCP4).Info("Session discovering", "session_id", sess.SessionID, "circuit_id", string(circuitID), "remote_id", string(remoteID))

	if alreadyApproved && ipoeCreated {
		c.logger.WithGroup(logger.IPoEDHCP4).Info("Session already approved, forwarding DISCOVER to provider", "session_id", sess.SessionID)
		pkt := &dhcp4.Packet{
			SessionID: sess.SessionID,
			MAC:       sess.MAC.String(),
			SVLAN:     sess.OuterVLAN,
			CVLAN:     sess.InnerVLAN,
			Raw:       buf.Bytes(),
		}
		response, err := c.dhcp4Provider.HandlePacket(c.Ctx, pkt)
		if err != nil {
			return fmt.Errorf("dhcp provider failed: %w", err)
		}
		if response != nil && len(response.Raw) > 0 {
			return c.sendDHCPResponse(sess.SessionID, sess.OuterVLAN, sess.InnerVLAN, sess.EncapIfIndex, sess.MAC, response.Raw, "OFFER")
		}
		return nil
	}

	if alreadyApproved && !ipoeCreated {
		c.logger.WithGroup(logger.IPoEDHCP4).Info("DHCP DISCOVER received, AAA approved but IPoE session pending", "session_id", sess.SessionID)
		return nil
	}

	if v6AaaPending {
		c.logger.WithGroup(logger.IPoEDHCP4).Info("DHCP DISCOVER received, waiting for v6 AAA response", "session_id", sess.SessionID)
		return nil
	}

	cfg, _ := c.cfgMgr.GetRunning()
	username := pkt.MAC.String()
	var policyName string
	var accessInterface string
	if cfg != nil {
		accessInterface, _ = cfg.GetAccessInterface()
		if cfg.SubscriberGroups != nil {
			if group, _ := cfg.SubscriberGroups.FindGroupBySVLAN(pkt.OuterVLAN); group != nil {
				policyName = group.AAAPolicy
			}
		}
	}
	if policyName != "" {
		if policy := cfg.AAA.GetPolicyByType(policyName, aaa.PolicyTypeDHCP); policy != nil {
			ctx := &aaa.PolicyContext{
				MACAddress: pkt.MAC,
				SVLAN:      pkt.OuterVLAN,
				CVLAN:      pkt.InnerVLAN,
				RemoteID:   string(remoteID),
				CircuitID:  string(circuitID),
				Hostname:   hostname,
			}
			username = policy.ExpandFormat(ctx)
			c.logger.WithGroup(logger.IPoEDHCP4).Info("Built username from policy", "policy", policyName, "format", policy.Format, "username", username)
		}
	}

	sess.Username = username

	c.logger.WithGroup(logger.IPoEDHCP4).Info("Publishing AAA request for DISCOVER", "session_id", sess.SessionID, "username", username)
	requestID := uuid.New().String()

	aaaPayload := &models.AAARequest{
		RequestID:     requestID,
		Username:      username,
		MAC:           pkt.MAC.String(),
		AcctSessionID: sess.AcctSessionID,
		SVLAN:         pkt.OuterVLAN,
		CVLAN:         pkt.InnerVLAN,
		Interface:     accessInterface,
		PolicyName:    policyName,
	}

	aaaEvent := models.Event{
		Type:       models.EventTypeAAARequest,
		AccessType: models.AccessTypeIPoE,
		Protocol:   models.ProtocolDHCPv4,
		SessionID:  sess.SessionID,
	}
	aaaEvent.SetPayload(aaaPayload)

	return c.eventBus.Publish(events.TopicAAARequest, aaaEvent)
}

func (c *Component) handleRequest(pkt *dataplane.ParsedPacket) error {
	lookupKey := c.makeSessionKeyV4(pkt.MAC, pkt.OuterVLAN, pkt.InnerVLAN)

	c.sessionMu.Lock()
	sess := c.sessions[lookupKey]
	if sess == nil {
		sessID := session.GenerateID()
		sess = &SessionState{
			SessionID:     sessID,
			AcctSessionID: session.ToAcctSessionID(sessID),
			MAC:           pkt.MAC,
			OuterVLAN:     pkt.OuterVLAN,
			InnerVLAN:     pkt.InnerVLAN,
			EncapIfIndex:  pkt.SwIfIndex,
			State:         "requesting",
		}
		c.sessions[lookupKey] = sess
		c.sessionIndex[sessID] = sess
	} else {
		sess.State = "requesting"
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}
	if err := pkt.DHCPv4.SerializeTo(buf, opts); err != nil {
		return fmt.Errorf("serialize DHCP: %w", err)
	}

	sess.XID = pkt.DHCPv4.Xid
	sess.LastSeen = time.Now()
	sess.PendingDHCPRequest = buf.Bytes()
	c.xidIndex[pkt.DHCPv4.Xid] = sess
	alreadyApproved := sess.AAAApproved
	c.sessionMu.Unlock()

	if alreadyApproved {
		c.logger.WithGroup(logger.IPoEDHCP4).Info("Session already AAA approved, processing REQUEST with DHCP provider", "session_id", sess.SessionID)

		buf := gopacket.NewSerializeBuffer()
		opts := gopacket.SerializeOptions{
			ComputeChecksums: true,
			FixLengths:       true,
		}
		if err := pkt.DHCPv4.SerializeTo(buf, opts); err != nil {
			return fmt.Errorf("serialize DHCP: %w", err)
		}

		dhcpPkt := &dhcp4.Packet{
			SessionID: sess.SessionID,
			MAC:       pkt.MAC.String(),
			SVLAN:     pkt.OuterVLAN,
			CVLAN:     pkt.InnerVLAN,
			Raw:       buf.Bytes(),
		}

		response, err := c.dhcp4Provider.HandlePacket(c.Ctx, dhcpPkt)
		if err != nil {
			c.logger.WithGroup(logger.IPoEDHCP4).Error("DHCP provider failed for REQUEST", "session_id", sess.SessionID, "error", err)
			return fmt.Errorf("dhcp provider failed: %w", err)
		}

		if response != nil && len(response.Raw) > 0 {
			if err := c.sendDHCPResponse(sess.SessionID, pkt.OuterVLAN, pkt.InnerVLAN, sess.EncapIfIndex, pkt.MAC, response.Raw, "ACK"); err != nil {
				return err
			}

			parsedResponse := &layers.DHCPv4{}
			if err := parsedResponse.DecodeFromBytes(response.Raw[28:], gopacket.NilDecodeFeedback); err == nil {
				msgType := layers.DHCPMsgTypeUnspecified
				for _, opt := range parsedResponse.Options {
					if opt.Type == layers.DHCPOptMessageType && len(opt.Data) == 1 {
						msgType = layers.DHCPMsgType(opt.Data[0])
						break
					}
				}

				if msgType == layers.DHCPMsgTypeAck {
					parsedPkt := &dataplane.ParsedPacket{
						DHCPv4: parsedResponse,
					}
					return c.handleAck(sess, parsedPkt)
				}
			}
		}

		return nil
	}

	c.logger.WithGroup(logger.IPoEDHCP4).Info("Session requesting, waiting for AAA approval", "session_id", sess.SessionID)

	hostname := string(getDHCPOption(pkt.DHCPv4.Options, layers.DHCPOptHostname))
	circuitID, remoteID := parseOption82(getDHCPOption(pkt.DHCPv4.Options, 82))

	cfg, _ := c.cfgMgr.GetRunning()
	username := pkt.MAC.String()
	var policyName string
	var accessInterface string
	if cfg != nil {
		accessInterface, _ = cfg.GetAccessInterface()
		if cfg.SubscriberGroups != nil {
			if group, _ := cfg.SubscriberGroups.FindGroupBySVLAN(pkt.OuterVLAN); group != nil {
				policyName = group.AAAPolicy
			}
		}
	}
	if policyName != "" {
		if policy := cfg.AAA.GetPolicyByType(policyName, aaa.PolicyTypeDHCP); policy != nil {
			ctx := &aaa.PolicyContext{
				MACAddress: pkt.MAC,
				SVLAN:      pkt.OuterVLAN,
				CVLAN:      pkt.InnerVLAN,
				RemoteID:   string(remoteID),
				CircuitID:  string(circuitID),
				Hostname:   hostname,
			}
			username = policy.ExpandFormat(ctx)
			c.logger.WithGroup(logger.IPoEDHCP4).Info("Built username from policy", "policy", policyName, "format", policy.Format, "username", username)
		}
	}

	sess.Username = username

	c.logger.WithGroup(logger.IPoEDHCP4).Info("Publishing AAA request", "session_id", sess.SessionID, "username", username)
	requestID := uuid.New().String()

	aaaPayload := &models.AAARequest{
		RequestID:     requestID,
		Username:      username,
		MAC:           pkt.MAC.String(),
		AcctSessionID: sess.AcctSessionID,
		SVLAN:         pkt.OuterVLAN,
		CVLAN:         pkt.InnerVLAN,
		Interface:     accessInterface,
		PolicyName:    policyName,
	}

	aaaEvent := models.Event{
		Type:       models.EventTypeAAARequest,
		AccessType: models.AccessTypeIPoE,
		Protocol:   models.ProtocolDHCPv4,
		SessionID:  sess.SessionID,
	}
	aaaEvent.SetPayload(aaaPayload)

	return c.eventBus.Publish(events.TopicAAARequest, aaaEvent)
}

func (c *Component) handleRelease(pkt *dataplane.ParsedPacket) error {
	lookupKey := c.makeSessionKeyV4(pkt.MAC, pkt.OuterVLAN, pkt.InnerVLAN)

	c.sessionMu.Lock()
	sess := c.sessions[lookupKey]
	if sess == nil {
		c.sessionMu.Unlock()
		c.logger.Info("Received DHCPRELEASE for unknown session", "mac", pkt.MAC.String(), "svlan", pkt.OuterVLAN, "cvlan", pkt.InnerVLAN)
		return nil
	}

	sessID := sess.SessionID
	acctSessionID := sess.AcctSessionID
	xid := sess.XID
	ipoeSwIfIndex := sess.IPoESwIfIndex
	ipv4 := sess.IPv4
	mac := sess.MAC
	encapIfIndex := sess.EncapIfIndex
	innerVLAN := sess.InnerVLAN
	ipv6Bound := sess.IPv6Bound

	sess.IPv4 = nil
	sess.State = "released"
	delete(c.xidIndex, xid)

	sessionMode := c.getSessionMode(pkt.OuterVLAN)
	deleteSession := true
	if sessionMode == subscriber.SessionModeUnified && ipv6Bound {
		deleteSession = false
	}

	if deleteSession {
		delete(c.sessionIndex, sessID)
		delete(c.sessions, lookupKey)
	}
	c.sessionMu.Unlock()

	c.logger.Info("IPv4 released by client", "session_id", sessID, "delete_session", deleteSession)

	if c.vpp != nil && ipoeSwIfIndex != 0 {
		if ipv4 != nil {
			c.vpp.IPoESetSessionIPv4Async(ipoeSwIfIndex, ipv4, false, func(err error) {
				if err != nil {
					c.logger.Warn("Failed to unbind IPv4 from IPoE session", "session_id", sessID, "error", err)
				}
			})
		}
		if deleteSession {
			c.vpp.DeleteIPoESessionAsync(mac, encapIfIndex, innerVLAN, func(err error) {
				if err != nil {
					c.logger.Warn("Failed to delete IPoE session", "session_id", sessID, "error", err)
				} else {
					c.logger.Info("Deleted IPoE session from VPP", "session_id", sessID, "sw_if_index", ipoeSwIfIndex)
				}
			})
		}
	}

	if deleteSession {
		counterKey := fmt.Sprintf("osvbng:session_count:%s:%d:%d", pkt.MAC.String(), pkt.OuterVLAN, pkt.InnerVLAN)
		newCount, err := c.cache.Decr(c.Ctx, counterKey)
		if err != nil {
			c.logger.Warn("Failed to decrement session counter", "error", err, "key", counterKey)
		} else if newCount <= 0 {
			c.cache.Delete(c.Ctx, counterKey)
		}
		c.deleteSessionCheckpoint(sessID)
	} else if sess != nil {
		c.checkpointSession(sess)
	}

	return c.publishSessionLifecycle(&models.IPoESession{
		SessionID:        sessID,
		State:            models.SessionStateReleased,
		AccessType:       string(models.AccessTypeIPoE),
		Protocol:         string(models.ProtocolDHCPv4),
		RADIUSSessionID:  acctSessionID,
		MAC:              mac,
		OuterVLAN:        pkt.OuterVLAN,
		InnerVLAN:        pkt.InnerVLAN,
		VRF:              sess.VRF,
		Username:         sess.Username,
		RADIUSAttributes: make(map[string]string),
	})
}

func (c *Component) handleServerResponse(pkt *dataplane.ParsedPacket) error {
	c.sessionMu.RLock()
	sess := c.xidIndex[pkt.DHCPv4.Xid]
	c.sessionMu.RUnlock()

	if sess == nil {
		msgType := getDHCPMessageType(pkt.DHCPv4.Options)
		c.logger.Info("Received DHCP response but no session found", "message_type", msgType.String(), "xid", fmt.Sprintf("0x%x", pkt.DHCPv4.Xid))
		return nil
	}

	msgType := getDHCPMessageType(pkt.DHCPv4.Options)
	c.logger.Debug("Forwarding DHCP to client", "message_type", msgType.String(), "mac", sess.MAC.String(), "session_id", sess.SessionID, "xid", fmt.Sprintf("0x%x", pkt.DHCPv4.Xid))

	var vmac net.HardwareAddr
	var parentSwIfIndex uint32
	if c.srgMgr != nil {
		vmac = c.srgMgr.GetVirtualMAC(sess.OuterVLAN)
	}
	if c.ifMgr != nil {
		if iface := c.ifMgr.Get(sess.EncapIfIndex); iface != nil {
			parentSwIfIndex = iface.SupSwIfIndex
		}
		if vmac == nil {
			if parent := c.ifMgr.Get(parentSwIfIndex); parent != nil && len(parent.MAC) >= 6 {
				vmac = net.HardwareAddr(parent.MAC[:6])
			}
		}
	}
	if vmac == nil {
		return fmt.Errorf("no virtual MAC for S-VLAN %d", sess.OuterVLAN)
	}

	modifiedDHCP := *pkt.DHCPv4
	modifiedDHCP.RelayAgentIP = net.IPv4zero

	broadcast := (pkt.DHCPv4.Flags & 0x8000) != 0
	dstIP := pkt.DHCPv4.YourClientIP
	if broadcast || dstIP.IsUnspecified() {
		dstIP = net.IPv4bcast
	}

	udpLayer := &layers.UDP{
		SrcPort: 67,
		DstPort: 68,
	}
	udpLayer.SetNetworkLayerForChecksum(&layers.IPv4{
		SrcIP: net.IPv4zero,
		DstIP: dstIP,
	})

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}

	if err := gopacket.SerializeLayers(buf, opts, &modifiedDHCP, udpLayer); err != nil {
		return fmt.Errorf("serialize DHCP/UDP: %w", err)
	}

	ipLayer := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: layers.IPProtocolUDP,
		SrcIP:    net.IPv4zero,
		DstIP:    dstIP,
	}

	finalBuf := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(finalBuf, opts, ipLayer, udpLayer, &modifiedDHCP); err != nil {
		return fmt.Errorf("serialize IP/UDP/DHCP: %w", err)
	}

	dstMAC := sess.MAC.String()
	if broadcast {
		dstMAC = "ff:ff:ff:ff:ff:ff"
	}

	egressPayload := &models.EgressPacketPayload{
		DstMAC:    dstMAC,
		SrcMAC:    vmac.String(),
		OuterVLAN: sess.OuterVLAN,
		InnerVLAN: sess.InnerVLAN,
		OuterTPID: c.getSessionOuterTPID(sess),
		SwIfIndex: parentSwIfIndex,
		RawData:   finalBuf.Bytes(),
	}

	egressEvent := models.Event{
		Type:       models.EventTypeEgress,
		AccessType: models.AccessTypeIPoE,
		Protocol:   models.ProtocolDHCPv4,
	}
	egressEvent.SetPayload(egressPayload)

	c.logger.Debug("Sending DHCP via egress", "message_type", msgType.String(), "dst_mac", dstMAC, "svlan", sess.OuterVLAN, "cvlan", sess.InnerVLAN)

	if err := c.eventBus.Publish(events.TopicEgress, egressEvent); err != nil {
		return err
	}

	if msgType == layers.DHCPMsgTypeAck {
		return c.handleAck(sess, pkt)
	}

	return nil
}

func (c *Component) handleAck(sess *SessionState, pkt *dataplane.ParsedPacket) error {
	leaseTime := uint32(0)
	if leaseOpt := getDHCPOption(pkt.DHCPv4.Options, 51); len(leaseOpt) == 4 {
		leaseTime = binary.BigEndian.Uint32(leaseOpt)
	}

	c.sessionMu.Lock()
	sess.State = "bound"
	sess.IPv4 = pkt.DHCPv4.YourClientIP
	sess.LeaseTime = leaseTime
	sess.BoundAt = time.Now()
	mac := sess.MAC
	svlan := sess.OuterVLAN
	cvlan := sess.InnerVLAN
	ipoeSwIfIndex := sess.IPoESwIfIndex
	c.sessionMu.Unlock()

	c.logger.WithGroup(logger.IPoEDHCP4).Info("Session bound", "session_id", sess.SessionID, "ipv4", sess.IPv4.String())

	if c.vpp != nil {
		sessID := sess.SessionID
		ipv4 := sess.IPv4
		if ipoeSwIfIndex != 0 {
			c.vpp.IPoESetSessionIPv4Async(ipoeSwIfIndex, ipv4, true, func(err error) {
				if err != nil {
					if errors.Is(err, vpp.ErrVPPUnavailable) {
						c.logger.WithGroup(logger.IPoEDHCP4).Debug("VPP unavailable, cannot bind IPv4", "session_id", sessID)
					} else {
						c.logger.WithGroup(logger.IPoEDHCP4).Error("Failed to bind IPv4 to IPoE session", "session_id", sessID, "error", err)
					}
					return
				}
				c.logger.WithGroup(logger.IPoEDHCP4).Info("Bound IPv4 to IPoE session", "session_id", sessID, "sw_if_index", ipoeSwIfIndex, "ipv4", ipv4.String())
			})
		} else {
			c.sessionMu.Lock()
			sess.PendingIPv4Binding = ipv4
			c.sessionMu.Unlock()
			c.logger.WithGroup(logger.IPoEDHCP4).Debug("IPoE session not ready, queued IPv4 binding", "session_id", sessID, "ipv4", ipv4.String())
		}
	}

	counterKey := fmt.Sprintf("osvbng:session_count:%s:%d:%d", mac.String(), svlan, cvlan)
	if _, err := c.cache.Incr(c.Ctx, counterKey); err != nil {
		c.logger.Warn("Failed to increment session counter", "error", err, "key", counterKey)
	}
	expiry := time.Duration(sess.LeaseTime*2) * time.Second
	if expiry == 0 || expiry > 24*time.Hour {
		expiry = 24 * time.Hour
	}
	c.cache.Expire(c.Ctx, counterKey, expiry)

	c.checkpointSession(sess)

	c.logger.Info("Publishing session lifecycle event", "session_id", sess.SessionID, "sw_if_index", ipoeSwIfIndex, "ipv4", sess.IPv4.String())

	ipoeSess := &models.IPoESession{
		SessionID:        sess.SessionID,
		State:            models.SessionStateActive,
		AccessType:       string(models.AccessTypeIPoE),
		Protocol:         string(models.ProtocolDHCPv4),
		MAC:              sess.MAC,
		OuterVLAN:        sess.OuterVLAN,
		InnerVLAN:        sess.InnerVLAN,
		VLANCount:        c.getVLANCount(sess.OuterVLAN, sess.InnerVLAN),
		IfIndex:          ipoeSwIfIndex,
		VRF:              sess.VRF,
		IPv4Address:      sess.IPv4,
		LeaseTime:        sess.LeaseTime,
		IPv6Address:      sess.IPv6Address,
		IPv6LeaseTime:    sess.IPv6LeaseTime,
		DUID:             sess.DHCPv6DUID,
		Username:         sess.Username,
		RADIUSSessionID:  sess.AcctSessionID,
		RADIUSAttributes: make(map[string]string),
	}
	if sess.IPv6Prefix != nil {
		ipoeSess.IPv6Prefix = sess.IPv6Prefix.String()
	}

	return c.publishSessionLifecycle(ipoeSess)
}

func (c *Component) handleAAAResponse(event models.Event) error {
	var payload models.AAAResponse
	if err := event.GetPayload(&payload); err != nil {
		return fmt.Errorf("failed to decode AAA response: %w", err)
	}

	sessID := event.SessionID
	allowed := payload.Allowed

	c.sessionMu.Lock()
	sess := c.sessionIndex[sessID]
	if sess == nil {
		c.sessionMu.Unlock()
		return fmt.Errorf("session %s not found for AAA response", sessID)
	}

	sess.AAAApproved = allowed
	pendingDiscover := sess.PendingDHCPDiscover
	pendingRequest := sess.PendingDHCPRequest
	sess.PendingDHCPDiscover = nil
	sess.PendingDHCPRequest = nil
	mac := sess.MAC
	svlan := sess.OuterVLAN
	cvlan := sess.InnerVLAN
	encapIfIndex := sess.EncapIfIndex
	ipoeCreated := sess.IPoESessionCreated
	c.sessionMu.Unlock()

	if !allowed {
		c.logger.Info("Session AAA rejected, DHCP not forwarded", "session_id", sessID)
		return nil
	}

	c.logger.Info("Session AAA approved", "session_id", sessID)

	resolved := c.resolveServiceGroup(svlan, payload.Attributes)

	logArgs := []any{"session_id", sessID}
	for _, attr := range resolved.LogAttrs() {
		logArgs = append(logArgs, attr.Key, attr.Value.Any())
	}
	c.logger.Info("Resolved service group", logArgs...)

	vrfName := resolved.VRF
	var decapVrfID uint32
	if vrfName != "" {
		if c.vrfMgr != nil {
			tableID, _, _, err := c.vrfMgr.ResolveVRF(vrfName)
			if err != nil {
				c.logger.Error("Failed to resolve VRF for session", "session_id", sessID, "vrf", vrfName, "error", err)
				return fmt.Errorf("resolve VRF %q: %w", vrfName, err)
			}
			decapVrfID = tableID
		}
		c.sessionMu.Lock()
		sess.VRF = vrfName
		sess.ServiceGroup = resolved
		c.sessionMu.Unlock()
	}

	if !ipoeCreated && c.vpp != nil {
		c.sessionMu.Lock()
		if sess.IPoESessionCreated {
			c.sessionMu.Unlock()
			c.logger.Debug("IPoE session already created by another handler", "session_id", sessID)
		} else {
			c.sessionMu.Unlock()

			localMAC := c.getLocalMAC(svlan, encapIfIndex)
			if localMAC == nil {
				c.logger.Error("No local MAC available for IPoE session", "session_id", sessID, "svlan", svlan)
				return fmt.Errorf("no local MAC for svlan %d", svlan)
			}

			c.vpp.AddIPoESessionAsync(mac, localMAC, encapIfIndex, svlan, cvlan, decapVrfID, func(swIfIndex uint32, err error) {
				c.sessionMu.Lock()
				if sess.IPoESessionCreated {
					c.sessionMu.Unlock()
					c.logger.Debug("IPoE session already created by concurrent handler", "session_id", sessID)
					return
				}
				if err != nil {
					c.sessionMu.Unlock()
					if errors.Is(err, vpp.ErrVPPUnavailable) {
						c.logger.Debug("VPP unavailable, cannot create IPoE session", "session_id", sessID)
					} else {
						c.logger.Error("Failed to create IPoE session in VPP", "session_id", sessID, "error", err)
					}
					return
				}
				sess.IPoESwIfIndex = swIfIndex
				sess.IPoESessionCreated = true
				pendingIPv4 := sess.PendingIPv4Binding
				pendingIPv6 := sess.PendingIPv6Binding
				pendingPD := sess.PendingPDBinding
				sess.PendingIPv4Binding = nil
				sess.PendingIPv6Binding = nil
				sess.PendingPDBinding = nil
				c.sessionMu.Unlock()
				c.logger.Info("Created IPoE session in VPP", "session_id", sessID, "sw_if_index", swIfIndex)

				if pendingIPv4 != nil {
					c.vpp.IPoESetSessionIPv4Async(swIfIndex, pendingIPv4, true, func(err error) {
						if err != nil {
							if errors.Is(err, vpp.ErrVPPUnavailable) {
							c.logger.Debug("VPP unavailable, cannot bind pending IPv4", "session_id", sessID)
						} else {
							c.logger.Error("Failed to bind pending IPv4", "session_id", sessID, "error", err)
						}
							return
						}
						c.logger.Info("Bound pending IPv4 to IPoE session", "session_id", sessID, "ipv4", pendingIPv4.String())
					})
				}
				if pendingIPv6 != nil {
					c.vpp.IPoESetSessionIPv6Async(swIfIndex, pendingIPv6, true, func(err error) {
						if err != nil {
							if errors.Is(err, vpp.ErrVPPUnavailable) {
							c.logger.Debug("VPP unavailable, cannot bind pending IPv6", "session_id", sessID)
						} else {
							c.logger.Error("Failed to bind pending IPv6", "session_id", sessID, "error", err)
						}
							return
						}
						c.logger.Info("Bound pending IPv6 to IPoE session", "session_id", sessID, "ipv6", pendingIPv6.String())
					})
				}
				if pendingPD != nil {
					nextHop := pendingIPv6
					if nextHop == nil {
						nextHop = net.ParseIP("::")
					}
					c.vpp.IPoESetDelegatedPrefixAsync(swIfIndex, *pendingPD, nextHop, true, func(err error) {
						if err != nil {
							if errors.Is(err, vpp.ErrVPPUnavailable) {
							c.logger.Debug("VPP unavailable, cannot bind pending PD", "session_id", sessID)
						} else {
							c.logger.Error("Failed to bind pending delegated prefix", "session_id", sessID, "error", err)
						}
							return
						}
						c.logger.Info("Bound pending delegated prefix", "session_id", sessID, "prefix", pendingPD.String())
					})
				}
			})
		}
	}

	c.sessionMu.Lock()
	if pendingDiscover == nil {
		pendingDiscover = sess.PendingDHCPDiscover
		sess.PendingDHCPDiscover = nil
	}
	if pendingRequest == nil {
		pendingRequest = sess.PendingDHCPRequest
		sess.PendingDHCPRequest = nil
	}
	pendingDHCPv6Solicit := sess.PendingDHCPv6Solicit
	pendingDHCPv6Request := sess.PendingDHCPv6Request
	dhcpv6DUID := sess.DHCPv6DUID
	sess.PendingDHCPv6Solicit = nil
	sess.PendingDHCPv6Request = nil
	c.sessionMu.Unlock()

	if pendingDiscover != nil {
		c.logger.Info("Forwarding pending DHCP DISCOVER", "session_id", sessID)

		pkt := &dhcp4.Packet{
			SessionID: sessID,
			MAC:       mac.String(),
			SVLAN:     svlan,
			CVLAN:     cvlan,
			Raw:       pendingDiscover,
		}

		response, err := c.dhcp4Provider.HandlePacket(c.Ctx, pkt)
		if err != nil {
			c.logger.Error("DHCP provider failed for DISCOVER", "session_id", sessID, "error", err)
			return fmt.Errorf("dhcp provider failed: %w", err)
		}

		if response != nil && len(response.Raw) > 0 {
			if err := c.sendDHCPResponse(sessID, svlan, cvlan, encapIfIndex, mac, response.Raw, "OFFER"); err != nil {
				return err
			}
		}
	}

	if pendingRequest != nil {
		c.logger.Info("Forwarding pending DHCP REQUEST", "session_id", sessID)

		pkt := &dhcp4.Packet{
			SessionID: sessID,
			MAC:       mac.String(),
			SVLAN:     svlan,
			CVLAN:     cvlan,
			Raw:       pendingRequest,
		}

		response, err := c.dhcp4Provider.HandlePacket(c.Ctx, pkt)
		if err != nil {
			c.logger.Error("DHCP provider failed for REQUEST", "session_id", sessID, "error", err)
			return fmt.Errorf("dhcp provider failed: %w", err)
		}

		if response != nil && len(response.Raw) > 0 {
			if err := c.sendDHCPResponse(sessID, svlan, cvlan, encapIfIndex, mac, response.Raw, "ACK"); err != nil {
				return err
			}
		}
	}

	if pendingDHCPv6Solicit != nil && c.dhcp6Provider != nil {
		c.logger.Info("Forwarding pending DHCPv6 SOLICIT", "session_id", sessID)

		pkt := &dhcp6.Packet{
			SessionID: sessID,
			MAC:       mac.String(),
			SVLAN:     svlan,
			CVLAN:     cvlan,
			DUID:      dhcpv6DUID,
			Raw:       pendingDHCPv6Solicit,
		}

		response, err := c.dhcp6Provider.HandlePacket(c.Ctx, pkt)
		if err != nil {
			c.logger.Error("DHCPv6 provider failed for SOLICIT", "session_id", sessID, "error", err)
		} else if response == nil || len(response.Raw) == 0 {
			c.logger.Warn("DHCPv6 provider returned empty response for SOLICIT", "session_id", sessID)
		} else {
			c.logger.Info("Sending DHCPv6 ADVERTISE", "session_id", sessID, "size", len(response.Raw))
			if err := c.sendDHCPv6Response(sess, response.Raw); err != nil {
				c.logger.Error("Failed to send DHCPv6 ADVERTISE", "session_id", sessID, "error", err)
			}
		}
	}

	if pendingDHCPv6Request != nil && c.dhcp6Provider != nil {
		c.logger.Info("Forwarding pending DHCPv6 REQUEST", "session_id", sessID)

		pkt := &dhcp6.Packet{
			SessionID: sessID,
			MAC:       mac.String(),
			SVLAN:     svlan,
			CVLAN:     cvlan,
			DUID:      dhcpv6DUID,
			Raw:       pendingDHCPv6Request,
		}

		response, err := c.dhcp6Provider.HandlePacket(c.Ctx, pkt)
		if err != nil {
			c.logger.Error("DHCPv6 provider failed for REQUEST", "session_id", sessID, "error", err)
		} else if response == nil || len(response.Raw) == 0 {
			c.logger.Warn("DHCPv6 provider returned empty response for REQUEST", "session_id", sessID)
		} else {
			c.logger.Info("Sending DHCPv6 REPLY", "session_id", sessID, "size", len(response.Raw))
			if err := c.sendDHCPv6Response(sess, response.Raw); err != nil {
				c.logger.Error("Failed to send DHCPv6 REPLY", "session_id", sessID, "error", err)
			}

			dhcpResp := gopacket.NewPacket(response.Raw, layers.LayerTypeDHCPv6, gopacket.Default)
			if layer := dhcpResp.Layer(layers.LayerTypeDHCPv6); layer != nil {
				dhcp := layer.(*layers.DHCPv6)
				if dhcp.MsgType == layers.DHCPv6MsgTypeReply {
					c.handleDHCPv6Reply(sess, dhcp)
				}
			}
		}
	}

	return nil
}

func (c *Component) getVLANCount(svlan, cvlan uint16) int {
	if cvlan == 0 {
		return 1
	}
	return 2
}

func (c *Component) checkSessionLimit(mac net.HardwareAddr, svlan, cvlan uint16) error {
	cfg, _ := c.cfgMgr.GetRunning()
	if cfg == nil {
		return nil
	}

	var policyName string
	if cfg.SubscriberGroups != nil {
		if group, vlanCfg := cfg.SubscriberGroups.FindGroupBySVLAN(svlan); group != nil {
			if vlanCfg != nil && vlanCfg.AAA != nil && vlanCfg.AAA.Policy != "" {
				policyName = vlanCfg.AAA.Policy
			} else {
				policyName = group.AAAPolicy
			}
		}
	}

	if policyName == "" {
		return nil
	}

	policy := cfg.AAA.GetPolicy(policyName)
	if policy == nil {
		return nil
	}

	maxSessions := policy.MaxConcurrentSessions
	if maxSessions <= 0 {
		return nil
	}

	count, err := c.countExistingSessions(mac, svlan, cvlan)
	if err != nil {
		c.logger.Warn("Failed to count sessions", "error", err)
		return nil
	}

	if count >= maxSessions {
		return fmt.Errorf("session limit reached (%d/%d) for %s on VLAN %d:%d",
			count, maxSessions, mac.String(), svlan, cvlan)
	}

	c.logger.Info("Session limit check passed", "current", count, "max", maxSessions, "mac", mac.String(), "svlan", svlan, "cvlan", cvlan)

	return nil
}

func (c *Component) countExistingSessions(mac net.HardwareAddr, svlan, cvlan uint16) (int, error) {
	counterKey := fmt.Sprintf("osvbng:session_count:%s:%d:%d", mac.String(), svlan, cvlan)

	val, err := c.cache.Get(c.Ctx, counterKey)
	if err != nil {
		return 0, nil
	}

	var count int64
	if _, err := fmt.Sscanf(string(val), "%d", &count); err != nil {
		return 0, nil
	}

	return int(count), nil
}

func (c *Component) sendDHCPResponse(sessID string, svlan, cvlan uint16, encapIfIndex uint32, mac net.HardwareAddr, rawData []byte, msgType string) error {
	var srcMAC string
	var parentSwIfIndex uint32
	var outerTPID uint16

	c.sessionMu.RLock()
	if sess := c.sessionIndex[sessID]; sess != nil {
		outerTPID = c.getSessionOuterTPID(sess)
	}
	c.sessionMu.RUnlock()
	if outerTPID == 0 {
		outerTPID = c.resolveOuterTPID(svlan)
	}

	if c.srgMgr != nil {
		if vmac := c.srgMgr.GetVirtualMAC(svlan); vmac != nil {
			srcMAC = vmac.String()
		}
	}
	if c.ifMgr != nil {
		if iface := c.ifMgr.Get(encapIfIndex); iface != nil {
			parentSwIfIndex = iface.SupSwIfIndex
		}
		if srcMAC == "" {
			if parent := c.ifMgr.Get(parentSwIfIndex); parent != nil && len(parent.MAC) >= 6 {
				srcMAC = net.HardwareAddr(parent.MAC[:6]).String()
			}
		}
	}

	egressPayload := &models.EgressPacketPayload{
		DstMAC:    mac.String(),
		SrcMAC:    srcMAC,
		OuterVLAN: svlan,
		InnerVLAN: cvlan,
		OuterTPID: outerTPID,
		SwIfIndex: parentSwIfIndex,
		RawData:   rawData,
	}

	egressEvent := models.Event{
		Type:       models.EventTypeEgress,
		AccessType: models.AccessTypeIPoE,
		Protocol:   models.ProtocolDHCPv4,
		SessionID:  sessID,
	}
	egressEvent.SetPayload(egressPayload)

	c.logger.Debug("Sending DHCP "+msgType+" to client", "session_id", sessID, "size", len(rawData))

	if err := c.eventBus.Publish(events.TopicEgress, egressEvent); err != nil {
		c.logger.Error("Failed to publish DHCP "+msgType+" to egress", "session_id", sessID, "error", err)
		return fmt.Errorf("publish egress: %w", err)
	}

	return nil
}

func (c *Component) cleanupSessions() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-c.Ctx.Done():
			return
		case <-ticker.C:
			c.sessionMu.Lock()
			now := time.Now()
			var toDelete []struct {
				key           string
				sess          *SessionState
			}
			for sessionID, session := range c.sessions {
				if now.Sub(session.LastSeen) > 30*time.Minute {
					toDelete = append(toDelete, struct {
						key  string
						sess *SessionState
					}{sessionID, session})
				}
			}
			for _, item := range toDelete {
				c.logger.Info("Cleaning up stale session", "session_id", item.sess.SessionID)
				delete(c.xidIndex, item.sess.XID)
				delete(c.sessionIndex, item.sess.SessionID)
				delete(c.sessions, item.key)
			}
			c.sessionMu.Unlock()

			for _, item := range toDelete {
				if c.vpp != nil && item.sess.IPoESwIfIndex != 0 {
					sessID := item.sess.SessionID
					if item.sess.IPv4 != nil {
						c.vpp.IPoESetSessionIPv4Async(item.sess.IPoESwIfIndex, item.sess.IPv4, false, func(err error) {
							if err != nil {
								c.logger.Warn("Failed to unbind IPv4 from stale IPoE session", "session_id", sessID, "error", err)
							}
						})
					}
					c.vpp.DeleteIPoESessionAsync(item.sess.MAC, item.sess.EncapIfIndex, item.sess.InnerVLAN, func(err error) {
						if err != nil {
							c.logger.Warn("Failed to delete stale IPoE session", "session_id", sessID, "error", err)
						}
					})
				}
			}
		}
	}
}

func (c *Component) getLocalMAC(svlan uint16, encapIfIndex uint32) net.HardwareAddr {
	if c.srgMgr != nil {
		if vmac := c.srgMgr.GetVirtualMAC(svlan); vmac != nil {
			return vmac
		}
	}
	if c.ifMgr != nil {
		if iface := c.ifMgr.Get(encapIfIndex); iface != nil && len(iface.MAC) >= 6 {
			return net.HardwareAddr(iface.MAC[:6])
		}
	}
	return nil
}

func (c *Component) getSessionMode(svlan uint16) subscriber.SessionMode {
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil || cfg.SubscriberGroups == nil {
		return subscriber.SessionModeUnified
	}

	group, _ := cfg.SubscriberGroups.FindGroupBySVLAN(svlan)
	if group == nil {
		return subscriber.SessionModeUnified
	}

	return group.GetSessionMode()
}


func (c *Component) makeSessionKeyV4(mac net.HardwareAddr, svlan, cvlan uint16) string {
	mode := c.getSessionMode(svlan)
	if mode == subscriber.SessionModeUnified {
		return fmt.Sprintf("ipoe:%s:%d:%d", mac.String(), svlan, cvlan)
	}
	return fmt.Sprintf("ipoe-v4:%s:%d:%d", mac.String(), svlan, cvlan)
}

func (c *Component) makeSessionKeyV6(mac net.HardwareAddr, svlan, cvlan uint16) string {
	mode := c.getSessionMode(svlan)
	if mode == subscriber.SessionModeUnified {
		return fmt.Sprintf("ipoe:%s:%d:%d", mac.String(), svlan, cvlan)
	}
	return fmt.Sprintf("ipoe-v6:%s:%d:%d", mac.String(), svlan, cvlan)
}

func (c *Component) consumeDHCPv6Packets() {
	if c.dhcp6Chan == nil {
		c.logger.Debug("DHCPv6 channel not configured, skipping DHCPv6 consumer")
		return
	}

	for {
		select {
		case <-c.Ctx.Done():
			return
		case pkt := <-c.dhcp6Chan:
			go func(pkt *dataplane.ParsedPacket) {
				if err := c.processDHCPv6Packet(pkt); err != nil {
					c.logger.Error("Error processing DHCPv6 packet", "error", err)
				}
			}(pkt)
		}
	}
}

func (c *Component) processDHCPv6Packet(pkt *dataplane.ParsedPacket) error {
	if pkt.DHCPv6 == nil {
		return fmt.Errorf("no DHCPv6 layer")
	}

	if pkt.OuterVLAN == 0 {
		return fmt.Errorf("packet rejected: S-VLAN required")
	}

	if c.dhcp6Provider == nil {
		return fmt.Errorf("no DHCPv6 provider configured")
	}

	isDF := true
	if c.srgMgr != nil {
		isDF = c.srgMgr.IsDF(pkt.OuterVLAN, pkt.MAC.String(), pkt.InnerVLAN)
	}
	if !isDF {
		return nil
	}

	c.logger.Info("Received DHCPv6 packet",
		"message_type", pkt.DHCPv6.MsgType.String(),
		"mac", pkt.MAC.String(),
		"xid", fmt.Sprintf("0x%x", pkt.DHCPv6.TransactionID))

	if pkt.DHCPv6.MsgType == layers.DHCPv6MsgTypeRelayForward {
		inner, interfaceID, remoteID := c.unwrapDHCPv6Relay(pkt)
		if inner == nil {
			return fmt.Errorf("failed to unwrap relay message")
		}
		pkt.DHCPv6 = inner
		return c.processDHCPv6Message(pkt, interfaceID, remoteID)
	}

	return c.processDHCPv6Message(pkt, nil, nil)
}

func (c *Component) processDHCPv6Message(pkt *dataplane.ParsedPacket, relayInterfaceID, relayRemoteID []byte) error {
	switch pkt.DHCPv6.MsgType {
	case layers.DHCPv6MsgTypeSolicit:
		return c.handleDHCPv6Solicit(pkt, relayInterfaceID, relayRemoteID)
	case layers.DHCPv6MsgTypeRequest, layers.DHCPv6MsgTypeRenew, layers.DHCPv6MsgTypeRebind:
		return c.handleDHCPv6Request(pkt)
	case layers.DHCPv6MsgTypeRelease, layers.DHCPv6MsgTypeDecline:
		return c.handleDHCPv6Release(pkt)
	}

	return nil
}

func (c *Component) handleDHCPv6Solicit(pkt *dataplane.ParsedPacket, relayInterfaceID, relayRemoteID []byte) error {
	lookupKey := c.makeSessionKeyV6(pkt.MAC, pkt.OuterVLAN, pkt.InnerVLAN)

	c.sessionMu.RLock()
	sess := c.sessions[lookupKey]
	c.sessionMu.RUnlock()

	if sess == nil {
		sessID := session.GenerateID()
		newSess := &SessionState{
			SessionID:     sessID,
			AcctSessionID: session.ToAcctSessionID(sessID),
			MAC:           pkt.MAC,
			OuterVLAN:     pkt.OuterVLAN,
			InnerVLAN:     pkt.InnerVLAN,
			EncapIfIndex:  pkt.SwIfIndex,
			State:         "soliciting",
		}

		c.sessionMu.Lock()
		if existing := c.sessions[lookupKey]; existing != nil {
			sess = existing
		} else {
			sess = newSess
			c.sessions[lookupKey] = sess
			c.sessionIndex[sessID] = sess
		}
		c.sessionMu.Unlock()
	}

	clientDUID := c.extractDHCPv6ClientDUID(pkt.DHCPv6)

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true}
	if err := pkt.DHCPv6.SerializeTo(buf, opts); err != nil {
		return fmt.Errorf("serialize DHCPv6: %w", err)
	}

	var xid [3]byte
	copy(xid[:], pkt.DHCPv6.TransactionID[:])

	c.sessionMu.Lock()
	sess.DHCPv6XID = xid
	sess.DHCPv6DUID = clientDUID
	sess.LastSeen = time.Now()
	sess.PendingDHCPv6Solicit = buf.Bytes()
	if pkt.IPv6 != nil {
		sess.ClientLinkLocal = pkt.IPv6.SrcIP
	}
	c.xid6Index[xid] = sess
	alreadyApproved := sess.AAAApproved
	ipoeCreated := sess.IPoESessionCreated
	circuitID := sess.CircuitID
	remoteID := sess.RemoteID
	v4AaaPending := sess.PendingDHCPDiscover != nil || sess.PendingDHCPRequest != nil
	c.sessionMu.Unlock()

	if len(circuitID) == 0 && len(relayInterfaceID) > 0 {
		circuitID = relayInterfaceID
		c.logger.Info("Using DHCPv6 relay interface-id as circuit-id", "interface_id", string(relayInterfaceID))
	}
	if len(remoteID) == 0 && len(relayRemoteID) > 0 {
		remoteID = relayRemoteID
		c.logger.Info("Using DHCPv6 relay remote-id as remote-id", "remote_id", string(relayRemoteID))
	}

	if alreadyApproved && ipoeCreated {
		return c.forwardDHCPv6ToProvider(sess, pkt, buf.Bytes())
	}

	if alreadyApproved && !ipoeCreated {
		c.logger.Info("DHCPv6 SOLICIT received, AAA approved but IPoE session pending", "session_id", sess.SessionID)
		return nil
	}

	if v4AaaPending {
		c.logger.Info("DHCPv6 SOLICIT received, waiting for v4 AAA response", "session_id", sess.SessionID)
		return nil
	}

	c.logger.Info("DHCPv6 SOLICIT received, requesting AAA", "session_id", sess.SessionID)

	cfg, _ := c.cfgMgr.GetRunning()
	username := pkt.MAC.String()
	var policyName string
	var accessInterface string
	if cfg != nil {
		accessInterface, _ = cfg.GetAccessInterface()
		if cfg.SubscriberGroups != nil {
			if group, _ := cfg.SubscriberGroups.FindGroupBySVLAN(pkt.OuterVLAN); group != nil {
				policyName = group.AAAPolicy
			}
		}
	}
	if policyName != "" {
		if policy := cfg.AAA.GetPolicyByType(policyName, aaa.PolicyTypeDHCP); policy != nil {
			ctx := &aaa.PolicyContext{
				MACAddress: pkt.MAC,
				SVLAN:      pkt.OuterVLAN,
				CVLAN:      pkt.InnerVLAN,
				RemoteID:   string(remoteID),
				CircuitID:  string(circuitID),
			}
			username = policy.ExpandFormat(ctx)
			c.logger.Info("Built username from policy", "policy", policyName, "format", policy.Format, "username", username)
		}
	}

	sess.Username = username

	requestID := uuid.New().String()
	aaaPayload := &models.AAARequest{
		RequestID:     requestID,
		Username:      username,
		MAC:           pkt.MAC.String(),
		AcctSessionID: sess.AcctSessionID,
		SVLAN:         pkt.OuterVLAN,
		CVLAN:         pkt.InnerVLAN,
		Interface:     accessInterface,
		PolicyName:    policyName,
	}

	aaaEvent := models.Event{
		Type:       models.EventTypeAAARequest,
		AccessType: models.AccessTypeIPoE,
		Protocol:   models.ProtocolDHCPv6,
		SessionID:  sess.SessionID,
	}
	aaaEvent.SetPayload(aaaPayload)

	c.logger.Info("Publishing AAA request for DHCPv6 SOLICIT", "session_id", sess.SessionID, "username", username)

	return c.eventBus.Publish(events.TopicAAARequest, aaaEvent)
}

func (c *Component) handleDHCPv6Request(pkt *dataplane.ParsedPacket) error {
	lookupKey := c.makeSessionKeyV6(pkt.MAC, pkt.OuterVLAN, pkt.InnerVLAN)

	c.sessionMu.Lock()
	sess := c.sessions[lookupKey]
	if sess == nil {
		c.sessionMu.Unlock()
		return fmt.Errorf("no session for DHCPv6 REQUEST")
	}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true}
	if err := pkt.DHCPv6.SerializeTo(buf, opts); err != nil {
		c.sessionMu.Unlock()
		return fmt.Errorf("serialize DHCPv6: %w", err)
	}

	var xid [3]byte
	copy(xid[:], pkt.DHCPv6.TransactionID[:])
	sess.DHCPv6XID = xid
	sess.LastSeen = time.Now()
	sess.PendingDHCPv6Request = buf.Bytes()
	if pkt.IPv6 != nil && sess.ClientLinkLocal == nil {
		sess.ClientLinkLocal = pkt.IPv6.SrcIP
	}
	c.xid6Index[xid] = sess
	alreadyApproved := sess.AAAApproved
	ipoeCreated := sess.IPoESessionCreated
	c.sessionMu.Unlock()

	if alreadyApproved && ipoeCreated {
		return c.forwardDHCPv6ToProvider(sess, pkt, buf.Bytes())
	}

	c.logger.Info("DHCPv6 REQUEST received, session awaiting AAA", "session_id", sess.SessionID)

	return nil
}

func (c *Component) handleDHCPv6Release(pkt *dataplane.ParsedPacket) error {
	lookupKey := c.makeSessionKeyV6(pkt.MAC, pkt.OuterVLAN, pkt.InnerVLAN)

	c.sessionMu.Lock()
	sess := c.sessions[lookupKey]
	if sess == nil {
		c.sessionMu.Unlock()
		return nil
	}

	sessID := sess.SessionID
	ipv6Address := sess.IPv6Address
	ipv6Prefix := sess.IPv6Prefix
	ipoeSwIfIndex := sess.IPoESwIfIndex
	mac := sess.MAC
	encapIfIndex := sess.EncapIfIndex
	innerVLAN := sess.InnerVLAN
	ipv4Bound := sess.IPv4 != nil
	xid6 := sess.DHCPv6XID

	sess.IPv6Bound = false
	sess.IPv6Address = nil
	sess.IPv6Prefix = nil
	delete(c.xid6Index, xid6)

	sessionMode := c.getSessionMode(pkt.OuterVLAN)
	deleteSession := true
	if sessionMode == subscriber.SessionModeUnified && ipv4Bound {
		deleteSession = false
	}

	if deleteSession {
		delete(c.sessionIndex, sessID)
		delete(c.sessions, lookupKey)
	}
	c.sessionMu.Unlock()

	c.logger.Info("IPv6 released by client", "session_id", sessID, "delete_session", deleteSession)

	if c.vpp != nil && ipoeSwIfIndex != 0 {
		if ipv6Address != nil {
			c.vpp.IPoESetSessionIPv6Async(ipoeSwIfIndex, ipv6Address, false, func(err error) {
				if err != nil {
					c.logger.Warn("Failed to unbind IPv6 from IPoE session", "session_id", sessID, "error", err)
				}
			})
		}
		if ipv6Prefix != nil {
			c.vpp.IPoESetDelegatedPrefixAsync(ipoeSwIfIndex, *ipv6Prefix, net.ParseIP("::"), false, func(err error) {
				if err != nil {
					c.logger.Warn("Failed to unbind delegated prefix from IPoE session", "session_id", sessID, "error", err)
				}
			})
		}
		if deleteSession {
			c.vpp.DeleteIPoESessionAsync(mac, encapIfIndex, innerVLAN, func(err error) {
				if err != nil {
					c.logger.Warn("Failed to delete IPoE session", "session_id", sessID, "error", err)
				} else {
					c.logger.Info("Deleted IPoE session from VPP", "session_id", sessID, "sw_if_index", ipoeSwIfIndex)
				}
			})
		}
	}

	if deleteSession {
		c.deleteSessionCheckpoint(sessID)
	} else if sess != nil {
		c.checkpointSession(sess)
	}

	if sessionMode != subscriber.SessionModeUnified {
		var prefixStr string
		if ipv6Prefix != nil {
			prefixStr = ipv6Prefix.String()
		}

		return c.publishSessionLifecycle(&models.IPoESession{
			SessionID:       sessID,
			State:           models.SessionStateReleased,
			AccessType:      string(models.AccessTypeIPoE),
			Protocol:        string(models.ProtocolDHCPv6),
			MAC:             mac,
			OuterVLAN:       pkt.OuterVLAN,
			InnerVLAN:       pkt.InnerVLAN,
			IfIndex:         ipoeSwIfIndex,
			VRF:             sess.VRF,
			IPv6Address:     ipv6Address,
			IPv6Prefix:      prefixStr,
			Username:        sess.Username,
			RADIUSSessionID: "",
		})
	}

	return nil
}

func (c *Component) forwardDHCPv6ToProvider(sess *SessionState, pkt *dataplane.ParsedPacket, raw []byte) error {
	dhcpPkt := &dhcp6.Packet{
		SessionID: sess.SessionID,
		MAC:       sess.MAC.String(),
		SVLAN:     sess.OuterVLAN,
		CVLAN:     sess.InnerVLAN,
		DUID:      sess.DHCPv6DUID,
		Raw:       raw,
	}

	response, err := c.dhcp6Provider.HandlePacket(c.Ctx, dhcpPkt)
	if err != nil {
		return fmt.Errorf("dhcp6 provider failed: %w", err)
	}

	if response == nil || len(response.Raw) == 0 {
		return nil
	}

	if err := c.sendDHCPv6Response(sess, response.Raw); err != nil {
		return err
	}

	dhcpResp := gopacket.NewPacket(response.Raw, layers.LayerTypeDHCPv6, gopacket.Default)
	if layer := dhcpResp.Layer(layers.LayerTypeDHCPv6); layer != nil {
		dhcp := layer.(*layers.DHCPv6)
		if dhcp.MsgType == layers.DHCPv6MsgTypeReply {
			return c.handleDHCPv6Reply(sess, dhcp)
		}
	}

	return nil
}

func (c *Component) handleDHCPv6Reply(sess *SessionState, dhcp *layers.DHCPv6) error {
	var ianaAddr net.IP
	var pdPrefix *net.IPNet
	var validTime uint32

	for _, opt := range dhcp.Options {
		if opt.Code == layers.DHCPv6OptIANA && len(opt.Data) >= 12 {
			iaData := opt.Data[12:]
			for len(iaData) >= 4 {
				subCode := binary.BigEndian.Uint16(iaData[0:2])
				subLen := binary.BigEndian.Uint16(iaData[2:4])
				if len(iaData) < int(4+subLen) {
					break
				}
				if subCode == 5 && subLen >= 24 {
					ianaAddr = net.IP(iaData[4:20])
					validTime = binary.BigEndian.Uint32(iaData[24:28])
				}
				iaData = iaData[4+subLen:]
			}
		}

		if opt.Code == layers.DHCPv6OptIAPD && len(opt.Data) >= 12 {
			pdData := opt.Data[12:]
			for len(pdData) >= 4 {
				subCode := binary.BigEndian.Uint16(pdData[0:2])
				subLen := binary.BigEndian.Uint16(pdData[2:4])
				if len(pdData) < int(4+subLen) {
					break
				}
				if subCode == 26 && subLen >= 25 {
					prefixLen := pdData[12]
					prefixIP := net.IP(pdData[13:29])
					pdPrefix = &net.IPNet{
						IP:   prefixIP,
						Mask: net.CIDRMask(int(prefixLen), 128),
					}
				}
				pdData = pdData[4+subLen:]
			}
		}
	}

	c.sessionMu.Lock()
	sess.IPv6Address = ianaAddr
	sess.IPv6Prefix = pdPrefix
	sess.IPv6LeaseTime = validTime
	sess.IPv6BoundAt = time.Now()
	sess.IPv6Bound = true
	ipoeSwIfIndex := sess.IPoESwIfIndex
	c.sessionMu.Unlock()

	c.logger.Info("DHCPv6 session bound", "session_id", sess.SessionID, "ipv6", ianaAddr, "prefix", pdPrefix)

	if c.vpp != nil {
		sessID := sess.SessionID
		if ipoeSwIfIndex != 0 {
			if ianaAddr != nil {
				c.vpp.IPoESetSessionIPv6Async(ipoeSwIfIndex, ianaAddr, true, func(err error) {
					if err != nil {
						if errors.Is(err, vpp.ErrVPPUnavailable) {
							c.logger.Debug("VPP unavailable, cannot bind IPv6", "session_id", sessID)
						} else {
							c.logger.Error("Failed to bind IPv6 to IPoE session", "session_id", sessID, "error", err)
						}
					} else {
						c.logger.Info("Bound IPv6 to IPoE session", "session_id", sessID, "ipv6", ianaAddr.String())
					}
				})
			}

			if pdPrefix != nil {
				nextHop := ianaAddr
				if nextHop == nil {
					nextHop = net.ParseIP("::")
				}
				c.vpp.IPoESetDelegatedPrefixAsync(ipoeSwIfIndex, *pdPrefix, nextHop, true, func(err error) {
					if err != nil {
						if errors.Is(err, vpp.ErrVPPUnavailable) {
							c.logger.Debug("VPP unavailable, cannot set delegated prefix", "session_id", sessID)
						} else {
							c.logger.Error("Failed to set delegated prefix", "session_id", sessID, "error", err)
						}
					} else {
						c.logger.Info("Set delegated prefix", "session_id", sessID, "prefix", pdPrefix.String())
					}
				})
			}
		} else {
			c.sessionMu.Lock()
			sess.PendingIPv6Binding = ianaAddr
			sess.PendingPDBinding = pdPrefix
			c.sessionMu.Unlock()
			c.logger.Debug("IPoE session not ready, queued IPv6 bindings", "session_id", sessID)
		}
	}

	c.checkpointSession(sess)

	var prefixStr string
	if pdPrefix != nil {
		prefixStr = pdPrefix.String()
	}

	ipoeSess := &models.IPoESession{
		SessionID:       sess.SessionID,
		State:           models.SessionStateActive,
		AccessType:      string(models.AccessTypeIPoE),
		Protocol:        string(models.ProtocolDHCPv6),
		MAC:             sess.MAC,
		OuterVLAN:       sess.OuterVLAN,
		InnerVLAN:       sess.InnerVLAN,
		VLANCount:       c.getVLANCount(sess.OuterVLAN, sess.InnerVLAN),
		IfIndex:         ipoeSwIfIndex,
		VRF:             sess.VRF,
		IPv4Address:     sess.IPv4,
		LeaseTime:       sess.LeaseTime,
		IPv6Address:     ianaAddr,
		IPv6Prefix:      prefixStr,
		IPv6LeaseTime:   sess.IPv6LeaseTime,
		DUID:            sess.DHCPv6DUID,
		Username:        sess.Username,
		RADIUSSessionID: sess.AcctSessionID,
	}

	return c.publishSessionLifecycle(ipoeSess)
}

func (c *Component) sendDHCPv6Response(sess *SessionState, rawDHCPv6 []byte) error {
	var srcMACBytes net.HardwareAddr
	var parentSwIfIndex uint32

	if c.srgMgr != nil {
		srcMACBytes = c.srgMgr.GetVirtualMAC(sess.OuterVLAN)
	}
	if c.ifMgr != nil {
		if iface := c.ifMgr.Get(sess.EncapIfIndex); iface != nil {
			parentSwIfIndex = iface.SupSwIfIndex
		}
		if srcMACBytes == nil {
			if parent := c.ifMgr.Get(parentSwIfIndex); parent != nil && len(parent.MAC) >= 6 {
				srcMACBytes = net.HardwareAddr(parent.MAC[:6])
			}
		}
	}
	if srcMACBytes == nil {
		return fmt.Errorf("no source MAC available")
	}

	srcMAC := srcMACBytes.String()
	srcIP := c.getLoopbackIPv6(sess.OuterVLAN)
	if srcIP == nil {
		return fmt.Errorf("no IPv6 source address available for S-VLAN %d", sess.OuterVLAN)
	}
	dstIP := sess.ClientLinkLocal
	if dstIP == nil {
		return fmt.Errorf("no client link-local address for session %s", sess.SessionID)
	}

	udpLayer := &layers.UDP{
		SrcPort: 547,
		DstPort: 546,
	}
	ipv6Layer := &layers.IPv6{
		Version:    6,
		HopLimit:   64,
		NextHeader: layers.IPProtocolUDP,
		SrcIP:      srcIP,
		DstIP:      dstIP,
	}
	udpLayer.SetNetworkLayerForChecksum(ipv6Layer)

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}

	payload := gopacket.Payload(rawDHCPv6)
	if err := gopacket.SerializeLayers(buf, opts, ipv6Layer, udpLayer, payload); err != nil {
		return fmt.Errorf("serialize IPv6/UDP/DHCPv6: %w", err)
	}

	egressPayload := &models.EgressPacketPayload{
		DstMAC:    sess.MAC.String(),
		SrcMAC:    srcMAC,
		OuterVLAN: sess.OuterVLAN,
		InnerVLAN: sess.InnerVLAN,
		OuterTPID: c.getSessionOuterTPID(sess),
		SwIfIndex: parentSwIfIndex,
		RawData:   buf.Bytes(),
	}

	egressEvent := models.Event{
		Type:       models.EventTypeEgress,
		AccessType: models.AccessTypeIPoE,
		Protocol:   models.ProtocolDHCPv6,
		SessionID:  sess.SessionID,
	}
	egressEvent.SetPayload(egressPayload)

	c.logger.Debug("Sending DHCPv6 response", "session_id", sess.SessionID, "size", len(rawDHCPv6), "dst_ip", dstIP)

	return c.eventBus.Publish(events.TopicEgress, egressEvent)
}

func (c *Component) extractDHCPv6ClientDUID(dhcp *layers.DHCPv6) []byte {
	for _, opt := range dhcp.Options {
		if opt.Code == layers.DHCPv6OptClientID {
			return opt.Data
		}
	}
	return nil
}

func (c *Component) getLoopbackIPv6(svlan uint16) net.IP {
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil {
		return nil
	}

	if cfg.SubscriberGroups == nil {
		return nil
	}

	group, vlanCfg := cfg.SubscriberGroups.FindGroupBySVLAN(svlan)
	if group == nil || vlanCfg == nil {
		return nil
	}

	loopbackName := vlanCfg.Interface
	if loopbackName == "" {
		return nil
	}

	iface, ok := cfg.Interfaces[loopbackName]
	if !ok || iface.Address == nil {
		return nil
	}

	for _, cidr := range iface.Address.IPv6 {
		ip, _, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if ip.IsLinkLocalUnicast() {
			return ip
		}
	}

	for _, cidr := range iface.Address.IPv6 {
		ip, _, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		return ip
	}

	return nil
}

func (c *Component) consumeIPv6NDPackets() {
	if c.ipv6NDChan == nil {
		c.logger.Debug("IPv6 ND channel not configured, skipping IPv6 ND consumer")
		return
	}

	for {
		select {
		case <-c.Ctx.Done():
			return
		case pkt := <-c.ipv6NDChan:
			go func(pkt *dataplane.ParsedPacket) {
				if err := c.processRSPacket(pkt); err != nil {
					c.logger.Error("Error processing RS packet", "error", err)
				}
			}(pkt)
		}
	}
}

func (c *Component) processRSPacket(pkt *dataplane.ParsedPacket) error {
	if pkt.ICMPv6 == nil {
		return fmt.Errorf("no ICMPv6 layer")
	}

	if pkt.ICMPv6.TypeCode.Type() != layers.ICMPv6TypeRouterSolicitation {
		return nil
	}

	if pkt.IPv6 == nil {
		return fmt.Errorf("no IPv6 layer")
	}

	if pkt.OuterVLAN == 0 {
		return fmt.Errorf("packet rejected: S-VLAN required")
	}

	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil {
		return fmt.Errorf("no running config available")
	}

	group, _ := cfg.SubscriberGroups.FindGroupBySVLAN(pkt.OuterVLAN)

	raConfig := southbound.IPv6RAConfig{
		Managed:        true,
		Other:          true,
		RouterLifetime: 1800,
		MaxInterval:    600,
		MinInterval:    200,
	}

	if cfg.DHCPv6.RA != nil {
		raConfig.Managed = cfg.DHCPv6.RA.GetManaged()
		raConfig.Other = cfg.DHCPv6.RA.GetOther()
		raConfig.RouterLifetime = cfg.DHCPv6.RA.GetRouterLifetime()
		raConfig.MaxInterval = cfg.DHCPv6.RA.GetMaxInterval()
		raConfig.MinInterval = cfg.DHCPv6.RA.GetMinInterval()
	}

	if group != nil && group.IPv6 != nil && group.IPv6.RA != nil {
		groupRA := group.IPv6.RA
		if groupRA.Managed != nil {
			raConfig.Managed = *groupRA.Managed
		}
		if groupRA.Other != nil {
			raConfig.Other = *groupRA.Other
		}
		if groupRA.RouterLifetime != 0 {
			raConfig.RouterLifetime = groupRA.RouterLifetime
		}
		if groupRA.MaxInterval != 0 {
			raConfig.MaxInterval = groupRA.MaxInterval
		}
		if groupRA.MinInterval != 0 {
			raConfig.MinInterval = groupRA.MinInterval
		}
	}

	var ianaPoolName string
	if group != nil {
		ianaPoolName = group.IANAPool
	}

	var prefixes []raPrefixInfo

	for _, pool := range cfg.DHCPv6.IANAPools {
		if ianaPoolName != "" && pool.Name != ianaPoolName {
			continue
		}

		prefixes = append(prefixes, raPrefixInfo{
			network:       pool.Network,
			validTime:     pool.ValidTime,
			preferredTime: pool.PreferredTime,
		})

		if ianaPoolName != "" {
			break
		}
	}

	c.logger.Debug("Processing RS packet",
		"mac", pkt.MAC.String(),
		"svlan", pkt.OuterVLAN,
		"cvlan", pkt.InnerVLAN,
		"src_ip", pkt.IPv6.SrcIP,
		"managed", raConfig.Managed,
		"other", raConfig.Other,
		"prefixes", len(prefixes),
	)

	return c.sendRAResponse(pkt, raConfig, prefixes)
}

func (c *Component) sendRAResponse(pkt *dataplane.ParsedPacket, raConfig southbound.IPv6RAConfig, prefixes []raPrefixInfo) error {
	var srcMACBytes net.HardwareAddr
	var parentSwIfIndex uint32

	if c.srgMgr != nil {
		srcMACBytes = c.srgMgr.GetVirtualMAC(pkt.OuterVLAN)
	}
	if c.ifMgr != nil {
		if iface := c.ifMgr.Get(pkt.SwIfIndex); iface != nil {
			parentSwIfIndex = iface.SupSwIfIndex
		}
		if srcMACBytes == nil {
			if parent := c.ifMgr.Get(parentSwIfIndex); parent != nil && len(parent.MAC) >= 6 {
				srcMACBytes = net.HardwareAddr(parent.MAC[:6])
			}
		}
	}
	if srcMACBytes == nil {
		return fmt.Errorf("no source MAC available")
	}

	srcIP := c.getLoopbackIPv6(pkt.OuterVLAN)
	if srcIP == nil {
		return fmt.Errorf("no IPv6 source address available for S-VLAN %d", pkt.OuterVLAN)
	}

	dstIP := pkt.IPv6.SrcIP
	if dstIP.IsUnspecified() {
		dstIP = net.ParseIP("ff02::1")
	}

	var raFlags uint8
	if raConfig.Managed {
		raFlags |= 0x80
	}
	if raConfig.Other {
		raFlags |= 0x40
	}

	var raOptions layers.ICMPv6Options
	raOptions = append(raOptions, layers.ICMPv6Option{
		Type: layers.ICMPv6OptSourceAddress,
		Data: srcMACBytes,
	})

	for _, prefix := range prefixes {
		_, ipNet, err := net.ParseCIDR(prefix.network)
		if err != nil {
			c.logger.Warn("Invalid prefix in RA config", "prefix", prefix.network, "error", err)
			continue
		}

		prefixLen, _ := ipNet.Mask.Size()

		validLifetime := prefix.validTime
		if validLifetime == 0 {
			validLifetime = 2592000
		}
		preferredLifetime := prefix.preferredTime
		if preferredLifetime == 0 {
			preferredLifetime = 604800
		}

		prefixData := make([]byte, 30)
		prefixData[0] = byte(prefixLen)
		prefixData[1] = 0x80 // L (on-link) flag
		binary.BigEndian.PutUint32(prefixData[2:6], validLifetime)
		binary.BigEndian.PutUint32(prefixData[6:10], preferredLifetime)
		// 4 bytes reserved (10:14)
		copy(prefixData[14:30], ipNet.IP.To16())

		raOptions = append(raOptions, layers.ICMPv6Option{
			Type: layers.ICMPv6OptPrefixInfo,
			Data: prefixData,
		})
	}

	raLayer := &layers.ICMPv6RouterAdvertisement{
		HopLimit:       64,
		Flags:          raFlags,
		RouterLifetime: uint16(raConfig.RouterLifetime),
		ReachableTime:  0,
		RetransTimer:   0,
		Options:        raOptions,
	}

	icmpv6Layer := &layers.ICMPv6{
		TypeCode: layers.CreateICMPv6TypeCode(layers.ICMPv6TypeRouterAdvertisement, 0),
	}

	ipv6Layer := &layers.IPv6{
		Version:    6,
		HopLimit:   255,
		NextHeader: layers.IPProtocolICMPv6,
		SrcIP:      srcIP,
		DstIP:      dstIP,
	}

	icmpv6Layer.SetNetworkLayerForChecksum(ipv6Layer)

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}

	if err := gopacket.SerializeLayers(buf, opts, ipv6Layer, icmpv6Layer, raLayer); err != nil {
		return fmt.Errorf("serialize RA: %w", err)
	}

	egressPayload := &models.EgressPacketPayload{
		DstMAC:    pkt.MAC.String(),
		SrcMAC:    srcMACBytes.String(),
		OuterVLAN: pkt.OuterVLAN,
		InnerVLAN: pkt.InnerVLAN,
		OuterTPID: c.resolveOuterTPID(pkt.OuterVLAN),
		SwIfIndex: parentSwIfIndex,
		RawData:   buf.Bytes(),
	}

	egressEvent := models.Event{
		Type:       models.EventTypeEgress,
		AccessType: models.AccessTypeIPoE,
		Protocol:   models.ProtocolIPv6ND,
	}
	egressEvent.SetPayload(egressPayload)

	c.logger.Debug("Sending RA response",
		"dst_mac", pkt.MAC.String(),
		"src_mac", srcMACBytes.String(),
		"svlan", pkt.OuterVLAN,
		"cvlan", pkt.InnerVLAN,
		"managed", raConfig.Managed,
		"other", raConfig.Other,
	)

	return c.eventBus.Publish(events.TopicEgress, egressEvent)
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

	if err := c.opdb.Put(c.Ctx, opdb.NamespaceIPoESessions, sess.SessionID, data); err != nil {
		c.logger.Warn("Failed to checkpoint session", "session_id", sess.SessionID, "error", err)
	}
}

func (c *Component) deleteSessionCheckpoint(sessionID string) {
	if c.opdb == nil {
		return
	}

	if err := c.opdb.Delete(c.Ctx, opdb.NamespaceIPoESessions, sessionID); err != nil {
		c.logger.Warn("Failed to delete session checkpoint", "session_id", sessionID, "error", err)
	}
}

func (c *Component) restoreSessions(ctx context.Context) error {
	if c.opdb == nil {
		return nil
	}

	var count, expired, stale int
	sessionCounts := make(map[string]int)
	now := time.Now()

	validIfIndexes := make(map[uint32]bool)
	if c.vpp != nil {
		if ifaces, err := c.vpp.DumpInterfaces(); err == nil {
			for _, iface := range ifaces {
				validIfIndexes[iface.SwIfIndex] = true
			}
		}
	}

	err := c.opdb.Load(ctx, opdb.NamespaceIPoESessions, func(key string, value []byte) error {
		var sess SessionState
		if err := json.Unmarshal(value, &sess); err != nil {
			c.logger.Warn("Failed to unmarshal session from opdb", "key", key, "error", err)
			return nil
		}

		if c.isSessionExpired(&sess, now) {
			c.opdb.Delete(ctx, opdb.NamespaceIPoESessions, key)
			expired++
			return nil
		}

		if sess.IPoESwIfIndex != 0 && !validIfIndexes[sess.IPoESwIfIndex] {
			c.logger.Info("VPP interface not found, resetting session state",
				"session_id", sess.SessionID,
				"stale_sw_if_index", sess.IPoESwIfIndex)
			sess.IPoESwIfIndex = 0
			sess.IPoESessionCreated = false
			sess.AAAApproved = false
			sess.PendingDHCPDiscover = nil
			sess.PendingDHCPRequest = nil
			sess.PendingDHCPv6Solicit = nil
			sess.PendingDHCPv6Request = nil
			stale++

			data, err := json.Marshal(&sess)
			if err == nil {
				c.opdb.Put(ctx, opdb.NamespaceIPoESessions, sess.SessionID, data)
			}
		} else if sess.AAAApproved && !sess.IPoESessionCreated {
			c.logger.Info("Session approved but IPoE never created, resetting AAA state",
				"session_id", sess.SessionID)
			sess.AAAApproved = false
			stale++

			data, err := json.Marshal(&sess)
			if err == nil {
				c.opdb.Put(ctx, opdb.NamespaceIPoESessions, sess.SessionID, data)
			}
		}

		lookupKey := c.makeSessionKeyV4(sess.MAC, sess.OuterVLAN, sess.InnerVLAN)

		c.sessionMu.Lock()
		c.sessions[lookupKey] = &sess
		c.sessionIndex[sess.SessionID] = &sess

		if sess.State == "bound" && sess.MAC != nil {
			counterKey := fmt.Sprintf("osvbng:session_count:%s:%d:%d", sess.MAC.String(), sess.OuterVLAN, sess.InnerVLAN)
			sessionCounts[counterKey]++
		}
		c.sessionMu.Unlock()

		if sess.State == "bound" {
			c.restoreSessionToCache(ctx, &sess, now)
		}

		count++
		return nil
	})
	if err != nil {
		return fmt.Errorf("restore ipoe sessions: %w", err)
	}

	for counterKey, cnt := range sessionCounts {
		for i := 0; i < cnt; i++ {
			c.cache.Incr(ctx, counterKey)
		}
	}

	c.logger.Info("Restored sessions from OpDB", "count", count, "expired", expired, "stale_vpp", stale, "counters", len(sessionCounts))
	return nil
}

func (c *Component) isSessionExpired(sess *SessionState, now time.Time) bool {
	if sess.State != "bound" {
		return false
	}

	if sess.IPv4 != nil && sess.LeaseTime > 0 && !sess.BoundAt.IsZero() {
		expiresAt := sess.BoundAt.Add(time.Duration(sess.LeaseTime) * time.Second)
		if now.After(expiresAt) {
			return true
		}
	}

	if sess.IPv6Bound && sess.IPv6LeaseTime > 0 && !sess.IPv6BoundAt.IsZero() {
		expiresAt := sess.IPv6BoundAt.Add(time.Duration(sess.IPv6LeaseTime) * time.Second)
		if now.After(expiresAt) {
			return true
		}
	}

	return false
}

func (c *Component) restoreSessionToCache(ctx context.Context, sess *SessionState, now time.Time) {
	cacheKey := fmt.Sprintf("osvbng:sessions:%s", sess.SessionID)

	protocol := string(models.ProtocolDHCPv4)
	if sess.IPv4 == nil && sess.IPv6Bound {
		protocol = string(models.ProtocolDHCPv6)
	}

	ipoeSess := &models.IPoESession{
		SessionID:       sess.SessionID,
		State:           models.SessionStateActive,
		AccessType:      string(models.AccessTypeIPoE),
		Protocol:        protocol,
		MAC:             sess.MAC,
		OuterVLAN:       sess.OuterVLAN,
		InnerVLAN:       sess.InnerVLAN,
		VLANCount:       c.getVLANCount(sess.OuterVLAN, sess.InnerVLAN),
		IfIndex:         sess.IPoESwIfIndex,
		VRF:             sess.VRF,
		IPv4Address:     sess.IPv4,
		LeaseTime:       sess.LeaseTime,
		IPv6Address:     sess.IPv6Address,
		IPv6LeaseTime:   sess.IPv6LeaseTime,
		DUID:            sess.DHCPv6DUID,
		RADIUSSessionID: sess.AcctSessionID,
	}
	if sess.IPv6Prefix != nil {
		ipoeSess.IPv6Prefix = sess.IPv6Prefix.String()
	}

	data, err := json.Marshal(ipoeSess)
	if err != nil {
		c.logger.Warn("Failed to marshal session for cache restore", "session_id", sess.SessionID, "error", err)
		return
	}

	var ttl time.Duration
	if sess.LeaseTime > 0 && !sess.BoundAt.IsZero() {
		expiresAt := sess.BoundAt.Add(time.Duration(sess.LeaseTime) * time.Second)
		ttl = expiresAt.Sub(now)
		if ttl < 0 {
			ttl = 0
		}
	}
	if sess.IPv6LeaseTime > 0 && !sess.IPv6BoundAt.IsZero() {
		expiresAt := sess.IPv6BoundAt.Add(time.Duration(sess.IPv6LeaseTime) * time.Second)
		v6ttl := expiresAt.Sub(now)
		if v6ttl > ttl {
			ttl = v6ttl
		}
	}

	if err := c.cache.Set(ctx, cacheKey, data, ttl); err != nil {
		c.logger.Warn("Failed to restore session to cache", "session_id", sess.SessionID, "error", err)
	}
}
