package ipoe

import (
	"context"
	"encoding/binary"
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
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/dhcp4"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/session"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/srg"
)

type Component struct {
	*component.Base

	logger        *slog.Logger
	eventBus      events.Bus
	srgMgr        *srg.Manager
	cfgMgr        component.ConfigManager
	vpp           *southbound.VPP
	cache         cache.Cache
	dhcp4Provider dhcp4.DHCPProvider
	sessions      map[string]*SessionState
	xidIndex      map[uint32]*SessionState
	sessionIndex  map[string]*SessionState
	sessionMu     sync.RWMutex

	dhcpChan <-chan *dataplane.ParsedPacket
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
}

func New(deps component.Dependencies, srgMgr *srg.Manager, dhcp4Provider dhcp4.DHCPProvider) (component.Component, error) {
	log := logger.Component(logger.ComponentIPoE)

	c := &Component{
		Base:          component.NewBase("ipoe"),
		logger:        log,
		eventBus:      deps.EventBus,
		srgMgr:        srgMgr,
		cfgMgr:        deps.ConfigManager,
		vpp:           deps.VPP,
		cache:         deps.Cache,
		dhcp4Provider: dhcp4Provider,
		sessions:      make(map[string]*SessionState),
		xidIndex:      make(map[uint32]*SessionState),
		sessionIndex:  make(map[string]*SessionState),
		dhcpChan:      deps.DHCPChan,
	}

	return c, nil
}

func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)
	c.logger.Info("Starting IPoE component")

	if err := c.eventBus.Subscribe(events.TopicAAAResponseIPoE, c.handleAAAResponse); err != nil {
		return fmt.Errorf("subscribe to aaa responses: %w", err)
	}

	c.Go(c.cleanupSessions)
	c.Go(c.consumeDHCPPackets)

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

	c.logger.Debug("[DF] Received DHCP packet",
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

func (c *Component) handleDiscover(pkt *dataplane.ParsedPacket) error {
	lookupKey := fmt.Sprintf("ipoe-v4:%s:%d:%d", pkt.MAC.String(), pkt.OuterVLAN, pkt.InnerVLAN)

	c.sessionMu.RLock()
	sess := c.sessions[lookupKey]
	c.sessionMu.RUnlock()

	if sess == nil {
		if err := c.checkSessionLimit(pkt.MAC, pkt.OuterVLAN, pkt.InnerVLAN); err != nil {
			c.logger.Info("DHCPDISCOVER rejected", "error", err)
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
	c.sessionMu.Unlock()

	c.logger.Info("Session discovering", "session_id", sess.SessionID, "circuit_id", string(circuitID), "remote_id", string(remoteID))

	cfg, _ := c.cfgMgr.GetRunning()
	username := pkt.MAC.String()
	var policyName string
	if cfg != nil && cfg.SubscriberGroups != nil {
		if group, _ := cfg.SubscriberGroups.FindGroupBySVLAN(pkt.OuterVLAN); group != nil {
			policyName = group.AAAPolicy
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
			c.logger.Info("Built username from policy", "policy", policyName, "format", policy.Format, "username", username)
		}
	}

	c.logger.Info("Publishing AAA request for DISCOVER", "session_id", sess.SessionID, "username", username)
	requestID := uuid.New().String()

	aaaPayload := &models.AAARequest{
		RequestID:     requestID,
		Username:      username,
		MAC:           pkt.MAC.String(),
		AcctSessionID: sess.AcctSessionID,
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
	lookupKey := fmt.Sprintf("ipoe-v4:%s:%d:%d", pkt.MAC.String(), pkt.OuterVLAN, pkt.InnerVLAN)

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
		c.logger.Info("Session already AAA approved, processing REQUEST with DHCP provider", "session_id", sess.SessionID)

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
			c.logger.Error("DHCP provider failed for REQUEST", "session_id", sess.SessionID, "error", err)
			return fmt.Errorf("dhcp provider failed: %w", err)
		}

		if response != nil && len(response.Raw) > 0 {
			if err := c.sendDHCPResponse(sess.SessionID, pkt.OuterVLAN, pkt.InnerVLAN, pkt.MAC, response.Raw, "ACK"); err != nil {
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

	c.logger.Info("Session requesting, waiting for AAA approval", "session_id", sess.SessionID)

	hostname := string(getDHCPOption(pkt.DHCPv4.Options, layers.DHCPOptHostname))
	circuitID, remoteID := parseOption82(getDHCPOption(pkt.DHCPv4.Options, 82))

	cfg, _ := c.cfgMgr.GetRunning()
	username := pkt.MAC.String()
	var policyName string
	if cfg != nil && cfg.SubscriberGroups != nil {
		if group, _ := cfg.SubscriberGroups.FindGroupBySVLAN(pkt.OuterVLAN); group != nil {
			policyName = group.AAAPolicy
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
			c.logger.Info("Built username from policy", "policy", policyName, "format", policy.Format, "username", username)
		}
	}

	c.logger.Info("Publishing AAA request", "session_id", sess.SessionID, "username", username)
	requestID := uuid.New().String()

	aaaPayload := &models.AAARequest{
		RequestID:     requestID,
		Username:      username,
		MAC:           pkt.MAC.String(),
		AcctSessionID: sess.AcctSessionID,
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
	lookupKey := fmt.Sprintf("ipoe-v4:%s:%d:%d", pkt.MAC.String(), pkt.OuterVLAN, pkt.InnerVLAN)

	c.sessionMu.Lock()
	sess := c.sessions[lookupKey]
	if sess == nil {
		c.sessionMu.Unlock()
		c.logger.Info("Received DHCPRELEASE for unknown session", "mac", pkt.MAC.String(), "svlan", pkt.OuterVLAN, "cvlan", pkt.InnerVLAN)
		return nil
	}
	sess.State = "released"
	sessID := sess.SessionID
	acctSessionID := sess.AcctSessionID
	xid := sess.XID
	ipoeSwIfIndex := sess.IPoESwIfIndex
	ipv4 := sess.IPv4
	mac := sess.MAC
	encapIfIndex := sess.EncapIfIndex
	innerVLAN := sess.InnerVLAN
	delete(c.xidIndex, xid)
	delete(c.sessionIndex, sessID)
	delete(c.sessions, lookupKey)
	c.sessionMu.Unlock()

	c.logger.Info("Session released by client", "session_id", sessID)

	if c.vpp != nil && ipoeSwIfIndex != 0 {
		if ipv4 != nil {
			if err := c.vpp.IPoESetSessionIPv4(ipoeSwIfIndex, ipv4, false); err != nil {
				c.logger.Warn("Failed to unbind IPv4 from IPoE session", "session_id", sessID, "error", err)
			}
		}
		if err := c.vpp.DeleteIPoESession(mac, encapIfIndex, innerVLAN); err != nil {
			c.logger.Warn("Failed to delete IPoE session", "session_id", sessID, "error", err)
		} else {
			c.logger.Info("Deleted IPoE session from VPP", "session_id", sessID, "sw_if_index", ipoeSwIfIndex)
		}
	}

	counterKey := fmt.Sprintf("osvbng:session_count:%s:%d:%d", pkt.MAC.String(), pkt.OuterVLAN, pkt.InnerVLAN)
	newCount, err := c.cache.Decr(c.Ctx, counterKey)
	if err != nil {
		c.logger.Warn("Failed to decrement session counter", "error", err, "key", counterKey)
	} else if newCount <= 0 {
		c.cache.Delete(c.Ctx, counterKey)
	}

	lifecyclePayload := &models.DHCPv4Session{
		SessionID:        sessID,
		RADIUSSessionID:  acctSessionID,
		MAC:              mac,
		OuterVLAN:        pkt.OuterVLAN,
		InnerVLAN:        pkt.InnerVLAN,
		State:            models.SessionStateReleased,
		RADIUSAttributes: make(map[string]string),
	}

	lifecycleEvent := models.Event{
		Type:       models.EventTypeSessionLifecycle,
		AccessType: models.AccessTypeIPoE,
		Protocol:   models.ProtocolDHCPv4,
		SessionID:  sessID,
	}
	lifecycleEvent.SetPayload(lifecyclePayload)

	return c.eventBus.Publish(events.TopicSessionLifecycle, lifecycleEvent)
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
	if c.srgMgr != nil {
		vmac = c.srgMgr.GetVirtualMAC(sess.OuterVLAN)
	} else {
		vmac = c.vpp.GetParentInterfaceMAC()
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
	mac := sess.MAC
	svlan := sess.OuterVLAN
	cvlan := sess.InnerVLAN
	ipoeSwIfIndex := sess.IPoESwIfIndex
	c.sessionMu.Unlock()

	c.logger.Info("Session bound", "session_id", sess.SessionID, "ipv4", sess.IPv4.String())

	if c.vpp != nil && ipoeSwIfIndex != 0 {
		if err := c.vpp.IPoESetSessionIPv4(ipoeSwIfIndex, sess.IPv4, true); err != nil {
			c.logger.Error("Failed to bind IPv4 to IPoE session", "session_id", sess.SessionID, "error", err)
			return fmt.Errorf("bind ipv4: %w", err)
		}
		c.logger.Info("Bound IPv4 to IPoE session", "session_id", sess.SessionID, "sw_if_index", ipoeSwIfIndex, "ipv4", sess.IPv4.String())
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

	c.logger.Info("Publishing session lifecycle event", "session_id", sess.SessionID, "sw_if_index", ipoeSwIfIndex, "ipv4", sess.IPv4.String())

	lifecyclePayload := &models.DHCPv4Session{
		SessionID:        sess.SessionID,
		State:            models.SessionStateActive,
		MAC:              sess.MAC,
		OuterVLAN:        sess.OuterVLAN,
		InnerVLAN:        sess.InnerVLAN,
		VLANCount:        c.getVLANCount(sess.OuterVLAN, sess.InnerVLAN),
		IfIndex:          int(ipoeSwIfIndex),
		IPv4Address:      sess.IPv4,
		LeaseTime:        sess.LeaseTime,
		RADIUSSessionID:  sess.AcctSessionID,
		RADIUSAttributes: make(map[string]string),
	}

	lifecycleEvent := models.Event{
		Type:       models.EventTypeSessionLifecycle,
		AccessType: models.AccessTypeIPoE,
		Protocol:   models.ProtocolDHCPv4,
		SessionID:  sess.SessionID,
	}
	lifecycleEvent.SetPayload(lifecyclePayload)

	return c.eventBus.Publish(events.TopicSessionLifecycle, lifecycleEvent)
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

	if !ipoeCreated && c.vpp != nil {
		localMAC := c.getLocalMAC(svlan)
		if localMAC == nil {
			c.logger.Error("No local MAC available for IPoE session", "session_id", sessID, "svlan", svlan)
			return fmt.Errorf("no local MAC for svlan %d", svlan)
		}

		swIfIndex, err := c.vpp.AddIPoESession(mac, localMAC, encapIfIndex, svlan, cvlan, 0)
		if err != nil {
			c.logger.Error("Failed to create IPoE session in VPP", "session_id", sessID, "error", err)
			return fmt.Errorf("create ipoe session: %w", err)
		}

		c.sessionMu.Lock()
		sess.IPoESwIfIndex = swIfIndex
		sess.IPoESessionCreated = true
		c.sessionMu.Unlock()

		c.logger.Info("Created IPoE session in VPP", "session_id", sessID, "sw_if_index", swIfIndex)
	}

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
			if err := c.sendDHCPResponse(sessID, svlan, cvlan, mac, response.Raw, "OFFER"); err != nil {
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
			if err := c.sendDHCPResponse(sessID, svlan, cvlan, mac, response.Raw, "ACK"); err != nil {
				return err
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

func (c *Component) sendDHCPResponse(sessID string, svlan, cvlan uint16, mac net.HardwareAddr, rawData []byte, msgType string) error {
	var srcMAC string

	if c.srgMgr != nil {
		if vmac := c.srgMgr.GetVirtualMAC(svlan); vmac != nil {
			srcMAC = vmac.String()
		}
	}
	if srcMAC == "" && c.vpp != nil {
		if ifMac := c.vpp.GetParentInterfaceMAC(); ifMac != nil {
			srcMAC = ifMac.String()
		}
	}

	egressPayload := &models.EgressPacketPayload{
		DstMAC:    mac.String(),
		SrcMAC:    srcMAC,
		OuterVLAN: svlan,
		InnerVLAN: cvlan,
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
					if item.sess.IPv4 != nil {
						c.vpp.IPoESetSessionIPv4(item.sess.IPoESwIfIndex, item.sess.IPv4, false)
					}
					if err := c.vpp.DeleteIPoESession(item.sess.MAC, item.sess.EncapIfIndex, item.sess.InnerVLAN); err != nil {
						c.logger.Warn("Failed to delete stale IPoE session", "session_id", item.sess.SessionID, "error", err)
					}
				}
			}
		}
	}
}

func (c *Component) getLocalMAC(svlan uint16) net.HardwareAddr {
	if c.srgMgr != nil {
		if vmac := c.srgMgr.GetVirtualMAC(svlan); vmac != nil {
			return vmac
		}
	}
	if c.vpp != nil {
		return c.vpp.GetParentInterfaceMAC()
	}
	return nil
}
