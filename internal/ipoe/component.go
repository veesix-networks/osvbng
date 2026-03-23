// Copyright 2025 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

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
	hapb "github.com/veesix-networks/osvbng/api/proto/ha"
	"github.com/veesix-networks/osvbng/pkg/aaa"
	"github.com/veesix-networks/osvbng/pkg/allocator"
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/component"
	aaacfg "github.com/veesix-networks/osvbng/pkg/config/aaa"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/config/ip"
	"github.com/veesix-networks/osvbng/pkg/dhcp"
	"github.com/veesix-networks/osvbng/pkg/dhcp4"
	"github.com/veesix-networks/osvbng/pkg/dhcp6"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/ha"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/opdb"
	"github.com/veesix-networks/osvbng/pkg/session"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/svcgroup"
	"github.com/veesix-networks/osvbng/pkg/vrfmgr"
	"google.golang.org/protobuf/proto"
)

type Component struct {
	*component.Base

	logger           *slog.Logger
	eventBus         events.Bus
	srgMgr           ha.SRGProvider
	ifMgr            *ifmgr.Manager
	cfgMgr           component.ConfigManager
	vpp              southbound.Southbound
	vrfMgr           *vrfmgr.Manager
	svcGroupResolver *svcgroup.Resolver
	cache            cache.Cache
	opdb             opdb.Store
	dhcp4Providers   map[string]dhcp4.DHCPProvider
	dhcp6Providers   map[string]dhcp6.DHCPProvider
	sessions     sync.Map
	xidIndex     sync.Map
	xid6Index    sync.Map
	sessionIndex sync.Map

	dhcpChan   <-chan *dataplane.ParsedPacket
	dhcp6Chan  <-chan *dataplane.ParsedPacket
	ipv6NDChan <-chan *dataplane.ParsedPacket

	aaaRespSub  events.Subscription
	haStateSub  events.Subscription
}

type SessionState struct {
	mu                  sync.Mutex
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

	Username     string
	Attributes   map[string]string
	OuterTPID    uint16
	VRF          string
	ServiceGroup svcgroup.ServiceGroup
	SRGName      string
	AllocCtx     *allocator.Context
}

func (c *Component) resolveServiceGroup(svlan uint16, aaaAttrs map[string]interface{}) svcgroup.ServiceGroup {
	var sgName string
	if v, ok := aaaAttrs[aaa.AttrServiceGroup]; ok {
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

func (c *Component) buildAllocContext(sess *SessionState, aaaAttrs map[string]interface{}) *allocator.Context {
	var profileName, ipv6ProfileName string
	cfg, err := c.cfgMgr.GetRunning()
	if err == nil && cfg != nil && cfg.SubscriberGroups != nil {
		if group, _ := cfg.SubscriberGroups.FindGroupBySVLAN(sess.OuterVLAN); group != nil {
			profileName = group.IPv4Profile
			ipv6ProfileName = group.IPv6Profile
		}
	}

	ctx := allocator.NewContext(sess.SessionID, sess.MAC, sess.OuterVLAN, sess.InnerVLAN, sess.VRF, sess.ServiceGroup.Name, profileName, ipv6ProfileName, aaaAttrs)

	if ctx.PoolOverride == "" && sess.ServiceGroup.Pool != "" {
		ctx.PoolOverride = sess.ServiceGroup.Pool
	}
	if ctx.IANAPoolOverride == "" && sess.ServiceGroup.IANAPool != "" {
		ctx.IANAPoolOverride = sess.ServiceGroup.IANAPool
	}
	if ctx.PDPoolOverride == "" && sess.ServiceGroup.PDPool != "" {
		ctx.PDPoolOverride = sess.ServiceGroup.PDPool
	}

	if sess.ServiceGroup.IPv4Profile != "" {
		ctx.ProfileName = sess.ServiceGroup.IPv4Profile
	}
	if sess.ServiceGroup.IPv6Profile != "" {
		ctx.IPv6ProfileName = sess.ServiceGroup.IPv6Profile
	}

	return ctx
}

func (c *Component) getDHCP4Provider(profile *ip.IPv4Profile) dhcp4.DHCPProvider {
	mode := "local"
	if profile != nil {
		m := profile.GetMode()
		if m == "relay" || m == "proxy" {
			mode = m
		}
	}
	return c.dhcp4Providers[mode]
}

func (c *Component) getDHCP6Provider(profile *ip.IPv6Profile) dhcp6.DHCPProvider {
	mode := "local"
	if profile != nil {
		m := profile.GetMode()
		if m == "relay" || m == "proxy" {
			mode = m
		}
	}
	return c.dhcp6Providers[mode]
}

func (c *Component) resolveIPv4Profile(ctx *allocator.Context) *ip.IPv4Profile {
	if ctx == nil || ctx.ProfileName == "" {
		return nil
	}
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil {
		return nil
	}
	return cfg.IPv4Profiles[ctx.ProfileName]
}

func (c *Component) resolveIPv6Profile(ctx *allocator.Context) *ip.IPv6Profile {
	if ctx == nil || ctx.IPv6ProfileName == "" {
		return nil
	}
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil {
		return nil
	}
	return cfg.IPv6Profiles[ctx.IPv6ProfileName]
}

func (c *Component) resolveAccessInterfaceName(encapIfIndex uint32) string {
	iface := c.ifMgr.Get(encapIfIndex)
	if iface == nil {
		return ""
	}
	parent := c.ifMgr.Get(iface.SupSwIfIndex)
	if parent == nil {
		return iface.Name
	}
	return parent.Name
}

func (c *Component) resolveDHCPv4(ctx *allocator.Context) *dhcp.ResolvedDHCPv4 {
	if ctx == nil || ctx.ProfileName == "" {
		return nil
	}
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil {
		return nil
	}
	profile := cfg.IPv4Profiles[ctx.ProfileName]
	if profile == nil {
		c.logger.Error("IPv4 profile not found", "profile", ctx.ProfileName, "session_id", ctx.SessionID)
		return nil
	}
	resolved := dhcp.ResolveV4(ctx, profile)
	if resolved == nil {
		c.logger.Warn("IPv4 address resolution failed",
			"session_id", ctx.SessionID,
			"profile", ctx.ProfileName,
			"vrf", ctx.VRF,
			"pool_override", ctx.PoolOverride)
	}
	return resolved
}

func (c *Component) resolveDHCPv6(ctx *allocator.Context) *dhcp.ResolvedDHCPv6 {
	if ctx == nil || ctx.IPv6ProfileName == "" {
		return nil
	}
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil {
		return nil
	}
	profile := cfg.IPv6Profiles[ctx.IPv6ProfileName]
	if profile == nil {
		c.logger.Error("IPv6 profile not found", "profile", ctx.IPv6ProfileName, "session_id", ctx.SessionID)
		return nil
	}
	return dhcp.ResolveV6(ctx, profile)
}

func (c *Component) resolveSRGName(svlan uint16) string {
	if c.srgMgr == nil {
		return ""
	}
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil || cfg.SubscriberGroups == nil {
		return ""
	}
	groupName := cfg.SubscriberGroups.FindGroupNameBySVLAN(svlan)
	if groupName == "" {
		return ""
	}
	return c.srgMgr.GetSRGForGroup(groupName)
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

func New(deps component.Dependencies, srgMgr ha.SRGProvider, ifMgr *ifmgr.Manager, dhcp4Providers map[string]dhcp4.DHCPProvider, dhcp6Providers map[string]dhcp6.DHCPProvider) (*Component, error) {
	log := logger.Get(logger.IPoE)

	c := &Component{
		Base:             component.NewBase("ipoe"),
		logger:           log,
		eventBus:         deps.EventBus,
		srgMgr:           srgMgr,
		ifMgr:            ifMgr,
		vrfMgr:           deps.VRFManager,
		svcGroupResolver: deps.SvcGroupResolver,
		cfgMgr:           deps.ConfigManager,
		vpp:              deps.Southbound,
		cache:            deps.Cache,
		opdb:             deps.OpDB,
		dhcp4Providers:   dhcp4Providers,
		dhcp6Providers:   dhcp6Providers,
		dhcpChan:         deps.DHCPChan,
		dhcp6Chan:        deps.DHCPv6Chan,
		ipv6NDChan:       deps.IPv6NDChan,
	}

	return c, nil
}

func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)
	c.logger.Info("Starting IPoE component")

	if err := c.restoreSessions(ctx); err != nil {
		c.logger.Warn("Failed to restore sessions from OpDB", "error", err)
	}

	c.aaaRespSub = c.eventBus.Subscribe(events.TopicAAAResponseIPoE, c.handleAAAResponse)
	c.haStateSub = c.eventBus.Subscribe(events.TopicHAStateChange, c.handleHAStateChange)

	c.Go(c.cleanupSessions)
	c.Go(c.consumeDHCPPackets)
	c.Go(c.consumeDHCPv6Packets)
	c.Go(c.consumeIPv6NDPackets)

	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.logger.Info("Stopping IPoE component")

	c.aaaRespSub.Unsubscribe()
	c.haStateSub.Unsubscribe()

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

	if c.srgMgr != nil {
		srgName := c.resolveSRGName(pkt.OuterVLAN)
		if !c.srgMgr.IsActive(srgName) {
			return nil
		}
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

func (c *Component) unwrapDHCPv6Relay(rawDHCPv6 []byte) (*dhcp6.Message, *dhcp6.RelayInfo) {
	msg, info := dhcp6.UnwrapRelay(rawDHCPv6)
	if info != nil {
		c.logger.Info("DHCPv6 relay message",
			"hop_count", info.HopCount,
			"link_addr", info.LinkAddr,
			"peer_addr", info.PeerAddr,
			"interface_id", string(info.InterfaceID),
			"remote_id", string(info.RemoteID))
	}
	if msg != nil {
		c.logger.Info("Unwrapped inner DHCPv6 message",
			"inner_type", msg.MsgType,
			"xid", fmt.Sprintf("0x%x", msg.TransactionID))
	}
	return msg, info
}

func (c *Component) handleDiscover(pkt *dataplane.ParsedPacket) error {
	lookupKey := c.makeSessionKeyV4(pkt.MAC, pkt.OuterVLAN, pkt.InnerVLAN)

	var sess *SessionState
	if val, ok := c.sessions.Load(lookupKey); ok {
		sess = val.(*SessionState)
	}

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

		if actual, loaded := c.sessions.LoadOrStore(lookupKey, newSess); loaded {
			sess = actual.(*SessionState)
		} else {
			sess = newSess
			c.sessionIndex.Store(sessID, sess)
		}
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

	sess.mu.Lock()
	sess.XID = pkt.DHCPv4.Xid
	sess.Hostname = hostname
	sess.ClientID = clientID
	sess.CircuitID = circuitID
	sess.RemoteID = remoteID
	sess.LastSeen = time.Now()
	sess.EncapIfIndex = pkt.SwIfIndex
	sess.PendingDHCPDiscover = buf.Bytes()
	alreadyApproved := sess.AAAApproved
	ipoeCreated := sess.IPoESessionCreated
	v6AaaPending := sess.PendingDHCPv6Solicit != nil || sess.PendingDHCPv6Request != nil
	sess.mu.Unlock()
	c.xidIndex.Store(pkt.DHCPv4.Xid, sess)

	c.logger.WithGroup(logger.IPoEDHCP4).Info("Session discovering", "session_id", sess.SessionID, "circuit_id", string(circuitID), "remote_id", string(remoteID))

	if alreadyApproved && ipoeCreated {
		c.logger.WithGroup(logger.IPoEDHCP4).Info("Session already approved, forwarding DISCOVER to provider", "session_id", sess.SessionID)
		v4Profile := c.resolveIPv4Profile(sess.AllocCtx)
		var resolved *dhcp.ResolvedDHCPv4
		if v4Profile == nil || v4Profile.GetMode() == "server" {
			resolved = c.resolveDHCPv4(sess.AllocCtx)
		}
		pkt := &dhcp4.Packet{
			SessionID: sess.SessionID,
			MAC:       sess.MAC.String(),
			SVLAN:     sess.OuterVLAN,
			CVLAN:     sess.InnerVLAN,
			Raw:       buf.Bytes(),
			Resolved:  resolved,
			SwIfIndex: sess.EncapIfIndex,
			Interface: c.resolveAccessInterfaceName(sess.EncapIfIndex),
			Profile:   v4Profile,
			LocalMAC:  c.getLocalMAC(sess.SRGName, sess.EncapIfIndex),
		}
		provider := c.getDHCP4Provider(v4Profile)
		if provider == nil {
			mode := "local"
			if v4Profile != nil {
				mode = v4Profile.GetMode()
			}
			return fmt.Errorf("no DHCPv4 provider for mode %s", mode)
		}
		response, err := provider.HandlePacket(c.Ctx, pkt)
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
		if policy := cfg.AAA.GetPolicyByType(policyName, aaacfg.PolicyTypeDHCP); policy != nil {
			ctx := &aaacfg.PolicyContext{
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

	aaaAttrs := make(map[string]string)
	if len(circuitID) > 0 {
		aaaAttrs[aaa.AttrCircuitID] = string(circuitID)
	}
	if len(remoteID) > 0 {
		aaaAttrs[aaa.AttrRemoteID] = string(remoteID)
	}
	if hostname != "" {
		aaaAttrs[aaa.AttrHostname] = hostname
	}

	aaaPayload := &models.AAARequest{
		RequestID:     requestID,
		Username:      username,
		MAC:           pkt.MAC.String(),
		AcctSessionID: sess.AcctSessionID,
		SVLAN:         pkt.OuterVLAN,
		CVLAN:         pkt.InnerVLAN,
		Interface:     accessInterface,
		PolicyName:    policyName,
		Attributes:    aaaAttrs,
	}

	c.eventBus.Publish(events.TopicAAARequest, events.Event{
		Source: c.Name(),
		Data: &events.AAARequestEvent{
			AccessType: models.AccessTypeIPoE,
			Protocol:   models.ProtocolDHCPv4,
			SessionID:  sess.SessionID,
			Request:    *aaaPayload,
		},
	})
	return nil
}

func (c *Component) handleRequest(pkt *dataplane.ParsedPacket) error {
	lookupKey := c.makeSessionKeyV4(pkt.MAC, pkt.OuterVLAN, pkt.InnerVLAN)

	var sess *SessionState
	if val, ok := c.sessions.Load(lookupKey); ok {
		sess = val.(*SessionState)
	}
	if sess == nil {
		sessID := session.GenerateID()
		newSess := &SessionState{
			SessionID:     sessID,
			AcctSessionID: session.ToAcctSessionID(sessID),
			MAC:           pkt.MAC,
			OuterVLAN:     pkt.OuterVLAN,
			InnerVLAN:     pkt.InnerVLAN,
			EncapIfIndex:  pkt.SwIfIndex,
			State:         "requesting",
		}
		if actual, loaded := c.sessions.LoadOrStore(lookupKey, newSess); loaded {
			sess = actual.(*SessionState)
		} else {
			sess = newSess
			c.sessionIndex.Store(sessID, sess)
		}
	} else {
		sess.mu.Lock()
		sess.State = "requesting"
		sess.mu.Unlock()
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}
	if err := pkt.DHCPv4.SerializeTo(buf, opts); err != nil {
		return fmt.Errorf("serialize DHCP: %w", err)
	}

	sess.mu.Lock()
	sess.XID = pkt.DHCPv4.Xid
	sess.LastSeen = time.Now()
	sess.PendingDHCPRequest = buf.Bytes()
	alreadyApproved := sess.AAAApproved
	sess.mu.Unlock()
	c.xidIndex.Store(pkt.DHCPv4.Xid, sess)

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

		v4Profile := c.resolveIPv4Profile(sess.AllocCtx)
		var resolved *dhcp.ResolvedDHCPv4
		if v4Profile == nil || v4Profile.GetMode() == "server" {
			resolved = c.resolveDHCPv4(sess.AllocCtx)
		}
		dhcpPkt := &dhcp4.Packet{
			SessionID: sess.SessionID,
			MAC:       pkt.MAC.String(),
			SVLAN:     pkt.OuterVLAN,
			CVLAN:     pkt.InnerVLAN,
			Raw:       buf.Bytes(),
			Resolved:  resolved,
			SwIfIndex: sess.EncapIfIndex,
			Interface: c.resolveAccessInterfaceName(sess.EncapIfIndex),
			Profile:   v4Profile,
			LocalMAC:  c.getLocalMAC(sess.SRGName, sess.EncapIfIndex),
		}

		provider := c.getDHCP4Provider(v4Profile)
		if provider == nil {
			mode := "local"
			if v4Profile != nil {
				mode = v4Profile.GetMode()
			}
			return fmt.Errorf("no DHCPv4 provider for mode %s", mode)
		}
		response, err := provider.HandlePacket(c.Ctx, dhcpPkt)
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
		if policy := cfg.AAA.GetPolicyByType(policyName, aaacfg.PolicyTypeDHCP); policy != nil {
			ctx := &aaacfg.PolicyContext{
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

	aaaAttrs := make(map[string]string)
	if len(circuitID) > 0 {
		aaaAttrs[aaa.AttrCircuitID] = string(circuitID)
	}
	if len(remoteID) > 0 {
		aaaAttrs[aaa.AttrRemoteID] = string(remoteID)
	}
	if hostname != "" {
		aaaAttrs[aaa.AttrHostname] = hostname
	}

	aaaPayload := &models.AAARequest{
		RequestID:     requestID,
		Username:      username,
		MAC:           pkt.MAC.String(),
		AcctSessionID: sess.AcctSessionID,
		SVLAN:         pkt.OuterVLAN,
		CVLAN:         pkt.InnerVLAN,
		Interface:     accessInterface,
		PolicyName:    policyName,
		Attributes:    aaaAttrs,
	}

	c.eventBus.Publish(events.TopicAAARequest, events.Event{
		Source: c.Name(),
		Data: &events.AAARequestEvent{
			AccessType: models.AccessTypeIPoE,
			Protocol:   models.ProtocolDHCPv4,
			SessionID:  sess.SessionID,
			Request:    *aaaPayload,
		},
	})
	return nil
}

func (c *Component) handleRelease(pkt *dataplane.ParsedPacket) error {
	lookupKey := c.makeSessionKeyV4(pkt.MAC, pkt.OuterVLAN, pkt.InnerVLAN)

	val, ok := c.sessions.Load(lookupKey)
	if !ok {
		c.logger.Info("Received DHCPRELEASE for unknown session", "mac", pkt.MAC.String(), "svlan", pkt.OuterVLAN, "cvlan", pkt.InnerVLAN)
		return nil
	}
	sess := val.(*SessionState)

	sess.mu.Lock()
	ciaddr := pkt.DHCPv4.ClientIP
	if sess.IPv4 != nil && !sess.IPv4.Equal(ciaddr) {
		sess.mu.Unlock()
		c.logger.Warn("DHCPRELEASE anti-spoof: ciaddr mismatch",
			"session_id", sess.SessionID,
			"expected", sess.IPv4,
			"received", ciaddr,
			"mac", pkt.MAC.String())
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
	ipv6Address := sess.IPv6Address
	ipv6Prefix := sess.IPv6Prefix
	v6duid := sess.DHCPv6DUID

	sess.IPv4 = nil
	sess.State = "released"
	sess.mu.Unlock()
	c.xidIndex.Delete(xid)

	sessionMode := c.getSessionMode(pkt.OuterVLAN)
	deleteSession := true
	if sessionMode == subscriber.SessionModeUnified && ipv6Bound {
		deleteSession = false
	}

	if deleteSession {
		sess.mu.Lock()
		sess.IPv6Bound = false
		sess.IPv6Address = nil
		sess.IPv6Prefix = nil
		dhcpv6XID := sess.DHCPv6XID
		sess.mu.Unlock()
		c.xid6Index.Delete(dhcpv6XID)
		c.sessionIndex.Delete(sessID)
		c.sessions.Delete(lookupKey)
	}

	c.logger.Info("IPv4 released by client", "session_id", sessID, "delete_session", deleteSession)

	if ipv4 != nil {
		allocator.GetGlobalRegistry().ReleaseIP(ipv4)
	}
	for _, p := range c.dhcp4Providers {
		p.ReleaseLease(mac.String())
	}
	if deleteSession {
		if ipv6Address != nil {
			allocator.GetGlobalRegistry().ReleaseIANAByIP(ipv6Address)
		}
		if ipv6Prefix != nil {
			allocator.GetGlobalRegistry().ReleasePDByPrefix(ipv6Prefix)
		}
		if v6duid != nil {
			for _, p := range c.dhcp6Providers {
				p.ReleaseLease(v6duid)
			}
		}
	}

	if c.vpp != nil && ipoeSwIfIndex != 0 {
		if ipv4 != nil {
			c.vpp.IPoESetSessionIPv4Async(ipoeSwIfIndex, ipv4, false, func(err error) {
				if err != nil {
					c.logger.Debug("Failed to unbind IPv4 from IPoE session", "session_id", sessID, "error", err)
				}
			})
		}
		if deleteSession {
			if ipv6Address != nil {
				c.vpp.IPoESetSessionIPv6Async(ipoeSwIfIndex, ipv6Address, false, func(err error) {
					if err != nil {
						c.logger.Debug("Failed to unbind IPv6 from IPoE session", "session_id", sessID, "error", err)
					}
				})
			}
			if ipv6Prefix != nil {
				c.vpp.IPoESetDelegatedPrefixAsync(ipoeSwIfIndex, *ipv6Prefix, net.ParseIP("::"), false, func(err error) {
					if err != nil {
						c.logger.Debug("Failed to unbind delegated prefix from IPoE session", "session_id", sessID, "error", err)
					}
				})
			}
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
		SessionID:    sessID,
		State:        models.SessionStateReleased,
		AccessType:   string(models.AccessTypeIPoE),
		Protocol:     string(models.ProtocolDHCPv4),
		AAASessionID: acctSessionID,
		MAC:          mac,
		OuterVLAN:    pkt.OuterVLAN,
		InnerVLAN:    pkt.InnerVLAN,
		VRF:          sess.VRF,
		SRGName:      sess.SRGName,
		Username:     sess.Username,
		IPv4Address:  ipv4,
		IfIndex:      ipoeSwIfIndex,
	})
}

func (c *Component) handleServerResponse(pkt *dataplane.ParsedPacket) error {
	val, ok := c.xidIndex.Load(pkt.DHCPv4.Xid)
	if !ok {
		msgType := getDHCPMessageType(pkt.DHCPv4.Options)
		c.logger.Info("Received DHCP response but no session found", "message_type", msgType.String(), "xid", fmt.Sprintf("0x%x", pkt.DHCPv4.Xid))
		return nil
	}
	sess := val.(*SessionState)

	msgType := getDHCPMessageType(pkt.DHCPv4.Options)
	c.logger.Debug("Forwarding DHCP to client", "message_type", msgType.String(), "mac", sess.MAC.String(), "session_id", sess.SessionID, "xid", fmt.Sprintf("0x%x", pkt.DHCPv4.Xid))

	var vmac net.HardwareAddr
	var parentSwIfIndex uint32
	if c.srgMgr != nil {
		vmac = c.srgMgr.GetVirtualMAC(sess.SRGName)
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

	c.logger.Debug("Sending DHCP via egress", "message_type", msgType.String(), "dst_mac", dstMAC, "svlan", sess.OuterVLAN, "cvlan", sess.InnerVLAN)

	c.eventBus.Publish(events.TopicEgress, events.Event{
		Source: c.Name(),
		Data: &events.EgressEvent{
			Protocol: models.ProtocolDHCPv4,
			Packet:   *egressPayload,
		},
	})

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

	sess.mu.Lock()
	alreadyBound := sess.State == "bound"
	sess.State = "bound"
	sess.IPv4 = pkt.DHCPv4.YourClientIP
	sess.LeaseTime = leaseTime
	sess.BoundAt = time.Now()
	mac := sess.MAC
	svlan := sess.OuterVLAN
	cvlan := sess.InnerVLAN
	ipoeSwIfIndex := sess.IPoESwIfIndex
	snapshotIPv6 := sess.IPv6Address
	snapshotIPv6LeaseTime := sess.IPv6LeaseTime
	snapshotIPv6Prefix := sess.IPv6Prefix
	sess.mu.Unlock()

	c.logger.WithGroup(logger.IPoEDHCP4).Info("Session bound", "session_id", sess.SessionID, "ipv4", sess.IPv4.String())

	if c.vpp != nil {
		sessID := sess.SessionID
		ipv4 := sess.IPv4
		if ipoeSwIfIndex != 0 {
			c.vpp.IPoESetSessionIPv4Async(ipoeSwIfIndex, ipv4, true, func(err error) {
				if err != nil {
					if errors.Is(err, southbound.ErrUnavailable) {
						c.logger.WithGroup(logger.IPoEDHCP4).Debug("VPP unavailable, cannot bind IPv4", "session_id", sessID)
					} else {
						c.logger.WithGroup(logger.IPoEDHCP4).Error("Failed to bind IPv4 to IPoE session", "session_id", sessID, "error", err)
					}
					return
				}
				c.logger.WithGroup(logger.IPoEDHCP4).Info("Bound IPv4 to IPoE session", "session_id", sessID, "sw_if_index", ipoeSwIfIndex, "ipv4", ipv4.String())
				c.publishSessionProgrammed(sess, ipoeSwIfIndex)
			})
		} else {
			sess.mu.Lock()
			sess.PendingIPv4Binding = ipv4
			sess.mu.Unlock()
			c.logger.WithGroup(logger.IPoEDHCP4).Debug("IPoE session not ready, queued IPv4 binding", "session_id", sessID, "ipv4", ipv4.String())
		}
	}

	counterKey := fmt.Sprintf("osvbng:session_count:%s:%d:%d", mac.String(), svlan, cvlan)
	if !alreadyBound {
		if _, err := c.cache.Incr(c.Ctx, counterKey); err != nil {
			c.logger.Warn("Failed to increment session counter", "error", err, "key", counterKey)
		}
	}
	expiry := time.Duration(sess.LeaseTime*2) * time.Second
	if expiry == 0 || expiry > 24*time.Hour {
		expiry = 24 * time.Hour
	}
	c.cache.Expire(c.Ctx, counterKey, expiry)

	c.checkpointSession(sess)

	c.logger.Info("Publishing session lifecycle event", "session_id", sess.SessionID, "sw_if_index", ipoeSwIfIndex, "ipv4", sess.IPv4.String())

	ipoeSess := &models.IPoESession{
		SessionID:     sess.SessionID,
		State:         models.SessionStateActive,
		AccessType:    string(models.AccessTypeIPoE),
		Protocol:      string(models.ProtocolDHCPv4),
		MAC:           sess.MAC,
		OuterVLAN:     sess.OuterVLAN,
		InnerVLAN:     sess.InnerVLAN,
		VLANCount:     c.getVLANCount(sess.OuterVLAN, sess.InnerVLAN),
		IfIndex:       ipoeSwIfIndex,
		VRF:           sess.VRF,
		ServiceGroup:  sess.ServiceGroup.Name,
		SRGName:       sess.SRGName,
		IPv4Address:   sess.IPv4,
		LeaseTime:     sess.LeaseTime,
		IPv6Address:   snapshotIPv6,
		IPv6LeaseTime: snapshotIPv6LeaseTime,
		DUID:          sess.DHCPv6DUID,
		Username:      sess.Username,
		AAASessionID:  sess.AcctSessionID,
		ActivatedAt:   time.Now(),
		OuterTPID:     sess.OuterTPID,
	}
	if sess.AllocCtx != nil {
		ipoeSess.IPv4Pool = sess.AllocCtx.AllocatedPool
		ipoeSess.IANAPool = sess.AllocCtx.AllocatedIANAPool
		ipoeSess.PDPool = sess.AllocCtx.AllocatedPDPool
	}
	if snapshotIPv6Prefix != nil {
		ipoeSess.IPv6Prefix = snapshotIPv6Prefix.String()
	}

	return c.publishSessionLifecycle(ipoeSess)
}

func (c *Component) handleAAAResponse(event events.Event) {
	data, ok := event.Data.(*events.AAAResponseEvent)
	if !ok {
		c.logger.Error("Invalid event data for AAA response")
		return
	}

	sessID := data.SessionID
	allowed := data.Response.Allowed

	val, ok := c.sessionIndex.Load(sessID)
	if !ok {
		c.logger.Error("Session not found for AAA response", "session_id", sessID)
		return
	}
	sess := val.(*SessionState)

	sess.mu.Lock()
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
	sess.mu.Unlock()

	if !allowed {
		c.logger.Info("Session AAA rejected, cleaning up session", "session_id", sessID)
		c.sessionIndex.Delete(sessID)
		c.xidIndex.Delete(sess.XID)
		lookupV4 := c.makeSessionKeyV4(mac, svlan, cvlan)
		lookupV6 := c.makeSessionKeyV6(mac, svlan, cvlan)
		c.sessions.Delete(lookupV4)
		c.sessions.Delete(lookupV6)
		return
	}

	var subscriberGroup, ipv4Profile, ipv6Profile string
	cfg, _ := c.cfgMgr.GetRunning()
	if cfg != nil && cfg.SubscriberGroups != nil {
		for name, group := range cfg.SubscriberGroups.Groups {
			for i := range group.VLANs {
				if group.VLANs[i].MatchesSVLAN(svlan) {
					subscriberGroup = name
					ipv4Profile = group.IPv4Profile
					ipv6Profile = group.IPv6Profile
					break
				}
			}
		}
	}
	c.logger.Info("Session AAA approved",
		"session_id", sessID,
		"subscriber_group", subscriberGroup,
		"ipv4_profile", ipv4Profile,
		"ipv6_profile", ipv6Profile,
	)

	resolved := c.resolveServiceGroup(svlan, data.Response.Attributes)

	logArgs := []any{"session_id", sessID}
	for _, attr := range resolved.LogAttrs() {
		logArgs = append(logArgs, attr.Key, attr.Value.Any())
	}
	c.logger.Info("Resolved service group", logArgs...)

	var srgName string
	if c.srgMgr != nil && subscriberGroup != "" {
		srgName = c.srgMgr.GetSRGForGroup(subscriberGroup)
	}

	storedAttrs := make(map[string]string, len(data.Response.Attributes))
	for k, v := range data.Response.Attributes {
		storedAttrs[k] = fmt.Sprintf("%v", v)
	}

	sess.mu.Lock()
	sess.Attributes = storedAttrs
	sess.ServiceGroup = resolved
	sess.SRGName = srgName
	sess.mu.Unlock()

	vrfName := resolved.VRF
	var decapVrfID uint32
	if vrfName != "" {
		if c.vrfMgr != nil {
			tableID, _, _, err := c.vrfMgr.ResolveVRF(vrfName)
			if err != nil {
				c.logger.Error("Failed to resolve VRF for session", "session_id", sessID, "vrf", vrfName, "error", err)
				return
			}
			decapVrfID = tableID
		}
		sess.mu.Lock()
		sess.VRF = vrfName
		sess.mu.Unlock()
	}

	allocCtx := c.buildAllocContext(sess, data.Response.Attributes)
	c.logger.Info("Built allocator context",
		"session_id", sessID,
		"profile", allocCtx.ProfileName,
		"pool_override", allocCtx.PoolOverride,
		"iana_pool_override", allocCtx.IANAPoolOverride,
		"pd_pool_override", allocCtx.PDPoolOverride,
	)
	sess.mu.Lock()
	sess.AllocCtx = allocCtx
	sess.mu.Unlock()

	if !ipoeCreated && c.vpp != nil {
		sess.mu.Lock()
		if sess.IPoESessionCreated {
			sess.mu.Unlock()
			c.logger.Debug("IPoE session already created by another handler", "session_id", sessID)
		} else {
			sess.mu.Unlock()

			localMAC := c.getLocalMAC(srgName, encapIfIndex)
			if localMAC == nil {
				c.logger.Error("No local MAC available for IPoE session", "session_id", sessID, "svlan", svlan)
				return
			}

			c.vpp.AddIPoESessionAsync(mac, localMAC, encapIfIndex, svlan, cvlan, decapVrfID, func(swIfIndex uint32, err error) {
				sess.mu.Lock()
				if sess.IPoESessionCreated {
					sess.mu.Unlock()
					c.logger.Debug("IPoE session already created by concurrent handler", "session_id", sessID)
					return
				}
				if err != nil {
					sess.mu.Unlock()
					if errors.Is(err, southbound.ErrUnavailable) {
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
				latePendingV6Solicit := sess.PendingDHCPv6Solicit
				latePendingV6Request := sess.PendingDHCPv6Request
				latePendingV4Discover := sess.PendingDHCPDiscover
				latePendingV4Request := sess.PendingDHCPRequest
				lateV6DUID := sess.DHCPv6DUID
				lateAllocCtx := sess.AllocCtx
				sess.PendingDHCPv6Solicit = nil
				sess.PendingDHCPv6Request = nil
				sess.PendingDHCPDiscover = nil
				sess.PendingDHCPRequest = nil
				unnumberedLoopback := c.resolveUnnumberedLoopback(sess)
				sess.mu.Unlock()
				c.logger.Info("Created IPoE session in VPP", "session_id", sessID, "sw_if_index", swIfIndex)

				c.setupSessionUnnumbered(sessID, swIfIndex, unnumberedLoopback)

				if pendingIPv4 != nil {
					c.vpp.IPoESetSessionIPv4Async(swIfIndex, pendingIPv4, true, func(err error) {
						if err != nil {
							if errors.Is(err, southbound.ErrUnavailable) {
								c.logger.Debug("VPP unavailable, cannot bind pending IPv4", "session_id", sessID)
							} else {
								c.logger.Error("Failed to bind pending IPv4", "session_id", sessID, "error", err)
							}
							return
						}
						c.logger.Info("Bound pending IPv4 to IPoE session", "session_id", sessID, "ipv4", pendingIPv4.String())
						c.publishSessionProgrammed(sess, swIfIndex)
					})
				}
				if pendingIPv6 != nil {
					c.vpp.IPoESetSessionIPv6Async(swIfIndex, pendingIPv6, true, func(err error) {
						if err != nil {
							if errors.Is(err, southbound.ErrUnavailable) {
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
							if errors.Is(err, southbound.ErrUnavailable) {
								c.logger.Debug("VPP unavailable, cannot bind pending PD", "session_id", sessID)
							} else {
								c.logger.Error("Failed to bind pending delegated prefix", "session_id", sessID, "error", err)
							}
							return
						}
						c.logger.Info("Bound pending delegated prefix", "session_id", sessID, "prefix", pendingPD.String())
					})
				}

				c.forwardLatePendingPackets(sess, sessID, mac, svlan, cvlan, encapIfIndex, srgName, lateAllocCtx, lateV6DUID, latePendingV4Discover, latePendingV4Request, latePendingV6Solicit, latePendingV6Request)
			})
		}
	}

	sess.mu.Lock()
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
	sess.mu.Unlock()

	v4Profile := c.resolveIPv4Profile(allocCtx)
	v6Profile := c.resolveIPv6Profile(allocCtx)
	accessIfName := c.resolveAccessInterfaceName(encapIfIndex)
	localMAC := c.getLocalMAC(srgName, encapIfIndex)

	hasV4 := pendingDiscover != nil || pendingRequest != nil
	v6Provider := c.getDHCP6Provider(v6Profile)
	hasV6 := (pendingDHCPv6Solicit != nil || pendingDHCPv6Request != nil) && v6Provider != nil

	var wg sync.WaitGroup

	if hasV4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.forwardPendingDHCPv4(sessID, mac, svlan, cvlan, encapIfIndex, accessIfName, v4Profile, localMAC, allocCtx, pendingDiscover, pendingRequest)
		}()
	}

	if hasV6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.forwardPendingDHCPv6(sess, sessID, mac, svlan, cvlan, encapIfIndex, accessIfName, v6Profile, v6Provider, localMAC, allocCtx, dhcpv6DUID, pendingDHCPv6Solicit, pendingDHCPv6Request)
		}()
	}

	wg.Wait()
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
	var srgName string

	if val, ok := c.sessionIndex.Load(sessID); ok {
		sess := val.(*SessionState)
		sess.mu.Lock()
		outerTPID = c.getSessionOuterTPID(sess)
		srgName = sess.SRGName
		sess.mu.Unlock()
	}
	if outerTPID == 0 {
		outerTPID = c.resolveOuterTPID(svlan)
	}

	if c.srgMgr != nil {
		if vmac := c.srgMgr.GetVirtualMAC(srgName); vmac != nil {
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

	c.logger.Debug("Sending DHCP "+msgType+" to client", "session_id", sessID, "size", len(rawData))

	c.eventBus.Publish(events.TopicEgress, events.Event{
		Source: c.Name(),
		Data: &events.EgressEvent{
			Protocol: models.ProtocolDHCPv4,
			Packet:   *egressPayload,
		},
	})

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
			now := time.Now()
			var toDelete []struct {
				key  string
				sess *SessionState
			}
			c.sessions.Range(func(k, v any) bool {
				key := k.(string)
				session := v.(*SessionState)
				session.mu.Lock()
				lastSeen := session.LastSeen
				session.mu.Unlock()
				if now.Sub(lastSeen) > 30*time.Minute {
					toDelete = append(toDelete, struct {
						key  string
						sess *SessionState
					}{key, session})
				}
				return true
			})
			for _, item := range toDelete {
				c.logger.Info("Cleaning up stale session", "session_id", item.sess.SessionID)
				c.xidIndex.Delete(item.sess.XID)
				c.sessionIndex.Delete(item.sess.SessionID)
				c.sessions.Delete(item.key)
			}

			for _, item := range toDelete {
				sess := item.sess
				sessID := sess.SessionID

				if sess.IPv4 != nil {
					allocator.GetGlobalRegistry().ReleaseIP(sess.IPv4)
				}
				if sess.IPv6Address != nil {
					allocator.GetGlobalRegistry().ReleaseIANAByIP(sess.IPv6Address)
				}
				if sess.IPv6Prefix != nil {
					allocator.GetGlobalRegistry().ReleasePDByPrefix(sess.IPv6Prefix)
				}
				for _, p := range c.dhcp4Providers {
					p.ReleaseLease(sess.MAC.String())
				}

				if c.vpp != nil && sess.IPoESwIfIndex != 0 {
					if sess.IPv6Address != nil {
						c.vpp.IPoESetSessionIPv6Async(sess.IPoESwIfIndex, sess.IPv6Address, false, func(err error) {
							if err != nil {
								c.logger.Debug("Failed to unbind IPv6 from stale IPoE session", "session_id", sessID, "error", err)
							}
						})
					}
					if sess.IPv6Prefix != nil {
						c.vpp.IPoESetDelegatedPrefixAsync(sess.IPoESwIfIndex, *sess.IPv6Prefix, net.ParseIP("::"), false, func(err error) {
							if err != nil {
								c.logger.Debug("Failed to unbind PD from stale IPoE session", "session_id", sessID, "error", err)
							}
						})
					}
					if sess.IPv4 != nil {
						c.vpp.IPoESetSessionIPv4Async(sess.IPoESwIfIndex, sess.IPv4, false, func(err error) {
							if err != nil {
								c.logger.Warn("Failed to unbind IPv4 from stale IPoE session", "session_id", sessID, "error", err)
							}
						})
					}
					c.vpp.DeleteIPoESessionAsync(sess.MAC, sess.EncapIfIndex, sess.InnerVLAN, func(err error) {
						if err != nil {
							c.logger.Warn("Failed to delete stale IPoE session", "session_id", sessID, "error", err)
						}
					})
				}

				c.deleteSessionCheckpoint(sessID)

				c.publishSessionLifecycle(&models.IPoESession{
					SessionID:   sessID,
					State:       models.SessionStateReleased,
					AccessType:  string(models.AccessTypeIPoE),
					MAC:         sess.MAC,
					OuterVLAN:   sess.OuterVLAN,
					InnerVLAN:   sess.InnerVLAN,
					VRF:         sess.VRF,
					SRGName:     sess.SRGName,
					Username:    sess.Username,
					IPv4Address: sess.IPv4,
					IPv6Address: sess.IPv6Address,
				})
			}
		}
	}
}

func (c *Component) getLocalMAC(srgName string, encapIfIndex uint32) net.HardwareAddr {
	if c.srgMgr != nil {
		if vmac := c.srgMgr.GetVirtualMAC(srgName); vmac != nil {
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

	if len(c.dhcp6Providers) == 0 {
		return fmt.Errorf("no DHCPv6 provider configured")
	}

	if c.srgMgr != nil {
		srgName := c.resolveSRGName(pkt.OuterVLAN)
		if !c.srgMgr.IsActive(srgName) {
			return nil
		}
	}

	rawDHCPv6 := append(pkt.DHCPv6.LayerContents(), pkt.DHCPv6.LayerPayload()...)

	c.logger.Info("Received DHCPv6 packet",
		"message_type", pkt.DHCPv6.MsgType.String(),
		"mac", pkt.MAC.String(),
		"xid", fmt.Sprintf("0x%x", pkt.DHCPv6.TransactionID))

	if pkt.DHCPv6.MsgType == layers.DHCPv6MsgTypeRelayForward {
		inner, info := c.unwrapDHCPv6Relay(rawDHCPv6)
		if inner == nil {
			return fmt.Errorf("failed to unwrap relay message")
		}
		var interfaceID, remoteID []byte
		if info != nil {
			interfaceID = info.InterfaceID
			remoteID = info.RemoteID
		}
		return c.processDHCPv6Message(pkt, inner, interfaceID, remoteID)
	}

	msg, err := dhcp6.ParseMessage(rawDHCPv6)
	if err != nil {
		return fmt.Errorf("parse DHCPv6: %w", err)
	}

	return c.processDHCPv6Message(pkt, msg, nil, nil)
}

func (c *Component) processDHCPv6Message(pkt *dataplane.ParsedPacket, msg *dhcp6.Message, relayInterfaceID, relayRemoteID []byte) error {
	switch msg.MsgType {
	case dhcp6.MsgTypeSolicit:
		return c.handleDHCPv6Solicit(pkt, msg, relayInterfaceID, relayRemoteID)
	case dhcp6.MsgTypeRequest, dhcp6.MsgTypeRenew, dhcp6.MsgTypeRebind:
		return c.handleDHCPv6Request(pkt, msg)
	case dhcp6.MsgTypeRelease, dhcp6.MsgTypeDecline:
		return c.handleDHCPv6Release(pkt, msg)
	}

	return nil
}

func (c *Component) handleDHCPv6Solicit(pkt *dataplane.ParsedPacket, msg *dhcp6.Message, relayInterfaceID, relayRemoteID []byte) error {
	lookupKey := c.makeSessionKeyV6(pkt.MAC, pkt.OuterVLAN, pkt.InnerVLAN)

	var sess *SessionState
	if val, ok := c.sessions.Load(lookupKey); ok {
		sess = val.(*SessionState)
	}

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

		if actual, loaded := c.sessions.LoadOrStore(lookupKey, newSess); loaded {
			sess = actual.(*SessionState)
		} else {
			sess = newSess
			c.sessionIndex.Store(sessID, sess)
		}
	}

	sess.mu.Lock()
	sess.DHCPv6XID = msg.TransactionID
	sess.DHCPv6DUID = msg.Options.ClientID
	sess.LastSeen = time.Now()
	sess.PendingDHCPv6Solicit = msg.Raw
	if pkt.IPv6 != nil {
		sess.ClientLinkLocal = pkt.IPv6.SrcIP
	}
	alreadyApproved := sess.AAAApproved
	ipoeCreated := sess.IPoESessionCreated
	circuitID := sess.CircuitID
	remoteID := sess.RemoteID
	v4AaaPending := sess.PendingDHCPDiscover != nil || sess.PendingDHCPRequest != nil
	sess.mu.Unlock()
	c.xid6Index.Store(msg.TransactionID, sess)

	if len(circuitID) == 0 && len(relayInterfaceID) > 0 {
		circuitID = relayInterfaceID
		c.logger.Info("Using DHCPv6 relay interface-id as circuit-id", "interface_id", string(relayInterfaceID))
	}
	if len(remoteID) == 0 && len(relayRemoteID) > 0 {
		remoteID = relayRemoteID
		c.logger.Info("Using DHCPv6 relay remote-id as remote-id", "remote_id", string(relayRemoteID))
	}

	if alreadyApproved && ipoeCreated {
		return c.forwardDHCPv6ToProvider(sess, pkt, msg.Raw)
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
		if policy := cfg.AAA.GetPolicyByType(policyName, aaacfg.PolicyTypeDHCP); policy != nil {
			ctx := &aaacfg.PolicyContext{
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

	aaaAttrs := make(map[string]string)
	if len(circuitID) > 0 {
		aaaAttrs[aaa.AttrCircuitID] = string(circuitID)
	}
	if len(remoteID) > 0 {
		aaaAttrs[aaa.AttrRemoteID] = string(remoteID)
	}

	aaaPayload := &models.AAARequest{
		RequestID:     requestID,
		Username:      username,
		MAC:           pkt.MAC.String(),
		AcctSessionID: sess.AcctSessionID,
		SVLAN:         pkt.OuterVLAN,
		CVLAN:         pkt.InnerVLAN,
		Interface:     accessInterface,
		PolicyName:    policyName,
		Attributes:    aaaAttrs,
	}

	c.logger.Info("Publishing AAA request for DHCPv6 SOLICIT", "session_id", sess.SessionID, "username", username)

	c.eventBus.Publish(events.TopicAAARequest, events.Event{
		Source: c.Name(),
		Data: &events.AAARequestEvent{
			AccessType: models.AccessTypeIPoE,
			Protocol:   models.ProtocolDHCPv6,
			SessionID:  sess.SessionID,
			Request:    *aaaPayload,
		},
	})
	return nil
}

func (c *Component) handleDHCPv6Request(pkt *dataplane.ParsedPacket, msg *dhcp6.Message) error {
	lookupKey := c.makeSessionKeyV6(pkt.MAC, pkt.OuterVLAN, pkt.InnerVLAN)

	val, ok := c.sessions.Load(lookupKey)
	if !ok {
		return fmt.Errorf("no session for DHCPv6 REQUEST")
	}
	sess := val.(*SessionState)

	sess.mu.Lock()
	sess.DHCPv6XID = msg.TransactionID
	sess.LastSeen = time.Now()
	sess.PendingDHCPv6Request = msg.Raw
	if pkt.IPv6 != nil && sess.ClientLinkLocal == nil {
		sess.ClientLinkLocal = pkt.IPv6.SrcIP
	}
	alreadyApproved := sess.AAAApproved
	ipoeCreated := sess.IPoESessionCreated
	sess.mu.Unlock()
	c.xid6Index.Store(msg.TransactionID, sess)

	if alreadyApproved && ipoeCreated {
		return c.forwardDHCPv6ToProvider(sess, pkt, msg.Raw)
	}

	c.logger.Info("DHCPv6 REQUEST received, session awaiting AAA", "session_id", sess.SessionID)

	return nil
}

func (c *Component) handleDHCPv6Release(pkt *dataplane.ParsedPacket, msg *dhcp6.Message) error {
	lookupKey := c.makeSessionKeyV6(pkt.MAC, pkt.OuterVLAN, pkt.InnerVLAN)

	val, ok := c.sessions.Load(lookupKey)
	if !ok {
		c.logger.Info("Received DHCPv6 Release for unknown session", "mac", pkt.MAC.String(), "svlan", pkt.OuterVLAN, "cvlan", pkt.InnerVLAN)
		return nil
	}
	sess := val.(*SessionState)

	sess.mu.Lock()
	sessID := sess.SessionID
	ipv6Address := sess.IPv6Address
	ipv6Prefix := sess.IPv6Prefix
	ipoeSwIfIndex := sess.IPoESwIfIndex
	mac := sess.MAC
	encapIfIndex := sess.EncapIfIndex
	innerVLAN := sess.InnerVLAN
	ipv4 := sess.IPv4
	ipv4Bound := ipv4 != nil
	xid6 := sess.DHCPv6XID
	duid := sess.DHCPv6DUID

	sess.IPv6Bound = false

	sessionMode := c.getSessionMode(pkt.OuterVLAN)
	deleteSession := true
	if sessionMode == subscriber.SessionModeUnified && ipv4Bound {
		deleteSession = false
	}

	sess.IPv6Address = nil
	sess.IPv6Prefix = nil
	if deleteSession {
		sess.IPv4 = nil
	}
	sess.mu.Unlock()
	c.xid6Index.Delete(xid6)
	if deleteSession {
		c.sessionIndex.Delete(sessID)
		c.sessions.Delete(lookupKey)
	}

	c.logger.Info("IPv6 released by client", "session_id", sessID, "delete_session", deleteSession)

	if len(c.dhcp6Providers) > 0 {
		v6Prof := c.resolveIPv6Profile(sess.AllocCtx)
		v6Prov := c.getDHCP6Provider(v6Prof)
		if v6Prov != nil {
			dhcpPkt := &dhcp6.Packet{
				SessionID: sessID,
				MAC:       mac.String(),
				SVLAN:     pkt.OuterVLAN,
				CVLAN:     pkt.InnerVLAN,
				DUID:      duid,
				Raw:       msg.Raw,
				SwIfIndex: sess.EncapIfIndex,
				Interface: c.resolveAccessInterfaceName(sess.EncapIfIndex),
				PeerAddr:  sess.ClientLinkLocal,
				Profile:   v6Prof,
				LocalMAC:  c.getLocalMAC(sess.SRGName, sess.EncapIfIndex),
			}
			response, err := v6Prov.HandlePacket(c.Ctx, dhcpPkt)
			if err != nil {
				c.logger.Warn("DHCPv6 provider failed on Release", "session_id", sessID, "error", err)
			} else if response != nil && len(response.Raw) > 0 {
				if err := c.sendDHCPv6Response(sess, response.Raw); err != nil {
					c.logger.Debug("Failed to send DHCPv6 Release Reply", "session_id", sessID, "error", err)
				}
			}
		}
	}

	if ipv6Address != nil {
		allocator.GetGlobalRegistry().ReleaseIANAByIP(ipv6Address)
	}
	if ipv6Prefix != nil {
		allocator.GetGlobalRegistry().ReleasePDByPrefix(ipv6Prefix)
	}
	if deleteSession {
		if ipv4 != nil {
			allocator.GetGlobalRegistry().ReleaseIP(ipv4)
		}
		for _, p := range c.dhcp4Providers {
			p.ReleaseLease(mac.String())
		}
	}

	if c.vpp != nil && ipoeSwIfIndex != 0 {
		if ipv6Address != nil {
			c.vpp.IPoESetSessionIPv6Async(ipoeSwIfIndex, ipv6Address, false, func(err error) {
				if err != nil {
					c.logger.Debug("Failed to unbind IPv6 from IPoE session", "session_id", sessID, "error", err)
				}
			})
		}
		if ipv6Prefix != nil {
			c.vpp.IPoESetDelegatedPrefixAsync(ipoeSwIfIndex, *ipv6Prefix, net.ParseIP("::"), false, func(err error) {
				if err != nil {
					c.logger.Debug("Failed to unbind delegated prefix from IPoE session", "session_id", sessID, "error", err)
				}
			})
		}
		if deleteSession {
			if ipv4 != nil {
				c.vpp.IPoESetSessionIPv4Async(ipoeSwIfIndex, ipv4, false, func(err error) {
					if err != nil {
						c.logger.Debug("Failed to unbind IPv4 from IPoE session", "session_id", sessID, "error", err)
					}
				})
			}
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

	if sessionMode != subscriber.SessionModeUnified {
		var prefixStr string
		if ipv6Prefix != nil {
			prefixStr = ipv6Prefix.String()
		}

		return c.publishSessionLifecycle(&models.IPoESession{
			SessionID:    sessID,
			State:        models.SessionStateReleased,
			AccessType:   string(models.AccessTypeIPoE),
			Protocol:     string(models.ProtocolDHCPv6),
			MAC:          mac,
			OuterVLAN:    pkt.OuterVLAN,
			InnerVLAN:    pkt.InnerVLAN,
			IfIndex:      ipoeSwIfIndex,
			VRF:          sess.VRF,
			SRGName:      sess.SRGName,
			IPv6Address:  ipv6Address,
			IPv6Prefix:   prefixStr,
			Username:     sess.Username,
			AAASessionID: "",
		})
	}

	return nil
}

func (c *Component) forwardPendingDHCPv4(sessID string, mac net.HardwareAddr, svlan, cvlan uint16, encapIfIndex uint32, accessIfName string, v4Profile *ip.IPv4Profile, localMAC net.HardwareAddr, allocCtx *allocator.Context, pendingDiscover, pendingRequest []byte) {
	provider := c.getDHCP4Provider(v4Profile)
	if provider == nil {
		c.logger.Error("No DHCPv4 provider available", "session_id", sessID)
		return
	}

	if pendingDiscover != nil {
		c.logger.Info("Forwarding pending DHCP DISCOVER", "session_id", sessID)
		var resolved *dhcp.ResolvedDHCPv4
		if v4Profile == nil || v4Profile.GetMode() == "server" {
			resolved = c.resolveDHCPv4(allocCtx)
		}
		pkt := &dhcp4.Packet{
			SessionID: sessID,
			MAC:       mac.String(),
			SVLAN:     svlan,
			CVLAN:     cvlan,
			Raw:       pendingDiscover,
			Resolved:  resolved,
			SwIfIndex: encapIfIndex,
			Interface: accessIfName,
			Profile:   v4Profile,
			LocalMAC:  localMAC,
		}
		response, err := provider.HandlePacket(c.Ctx, pkt)
		if err != nil {
			c.logger.Error("DHCP provider failed for DISCOVER", "session_id", sessID, "error", err)
			return
		}
		if response != nil && len(response.Raw) > 0 {
			if err := c.sendDHCPResponse(sessID, svlan, cvlan, encapIfIndex, mac, response.Raw, "OFFER"); err != nil {
				c.logger.Error("Failed to send DHCP OFFER", "session_id", sessID, "error", err)
				return
			}
		}
	}

	if pendingRequest != nil {
		c.logger.Info("Forwarding pending DHCP REQUEST", "session_id", sessID)
		var resolved *dhcp.ResolvedDHCPv4
		if v4Profile == nil || v4Profile.GetMode() == "server" {
			resolved = c.resolveDHCPv4(allocCtx)
		}
		pkt := &dhcp4.Packet{
			SessionID: sessID,
			MAC:       mac.String(),
			SVLAN:     svlan,
			CVLAN:     cvlan,
			Raw:       pendingRequest,
			Resolved:  resolved,
			SwIfIndex: encapIfIndex,
			Interface: accessIfName,
			Profile:   v4Profile,
			LocalMAC:  localMAC,
		}
		response, err := provider.HandlePacket(c.Ctx, pkt)
		if err != nil {
			c.logger.Error("DHCP provider failed for REQUEST", "session_id", sessID, "error", err)
			return
		}
		if response != nil && len(response.Raw) > 0 {
			if err := c.sendDHCPResponse(sessID, svlan, cvlan, encapIfIndex, mac, response.Raw, "ACK"); err != nil {
				c.logger.Error("Failed to send DHCP ACK", "session_id", sessID, "error", err)
				return
			}
		}
	}
}

func (c *Component) forwardPendingDHCPv6(sess *SessionState, sessID string, mac net.HardwareAddr, svlan, cvlan uint16, encapIfIndex uint32, accessIfName string, v6Profile *ip.IPv6Profile, v6Provider dhcp6.DHCPProvider, localMAC net.HardwareAddr, allocCtx *allocator.Context, dhcpv6DUID []byte, pendingSolicit, pendingRequest []byte) {
	if pendingSolicit != nil {
		c.logger.Info("Forwarding pending DHCPv6 SOLICIT", "session_id", sessID)
		var resolved *dhcp.ResolvedDHCPv6
		if v6Profile == nil || v6Profile.GetMode() == "server" {
			resolved = c.resolveDHCPv6(allocCtx)
		}
		pkt := &dhcp6.Packet{
			SessionID: sessID,
			MAC:       mac.String(),
			SVLAN:     svlan,
			CVLAN:     cvlan,
			DUID:      dhcpv6DUID,
			Raw:       pendingSolicit,
			Resolved:  resolved,
			SwIfIndex: encapIfIndex,
			Interface: accessIfName,
			PeerAddr:  sess.ClientLinkLocal,
			Profile:   v6Profile,
			LocalMAC:  localMAC,
		}
		response, err := v6Provider.HandlePacket(c.Ctx, pkt)
		if err != nil {
			c.logger.Error("DHCPv6 provider failed for SOLICIT", "session_id", sessID, "error", err)
		} else if response != nil && len(response.Raw) > 0 {
			if err := c.sendDHCPv6Response(sess, response.Raw); err != nil {
				c.logger.Error("Failed to send DHCPv6 ADVERTISE", "session_id", sessID, "error", err)
			}
		}
	}

	if pendingRequest != nil {
		c.logger.Info("Forwarding pending DHCPv6 REQUEST", "session_id", sessID)
		var resolved *dhcp.ResolvedDHCPv6
		if v6Profile == nil || v6Profile.GetMode() == "server" {
			resolved = c.resolveDHCPv6(allocCtx)
		}
		pkt := &dhcp6.Packet{
			SessionID: sessID,
			MAC:       mac.String(),
			SVLAN:     svlan,
			CVLAN:     cvlan,
			DUID:      dhcpv6DUID,
			Raw:       pendingRequest,
			Resolved:  resolved,
			SwIfIndex: encapIfIndex,
			Interface: accessIfName,
			PeerAddr:  sess.ClientLinkLocal,
			Profile:   v6Profile,
			LocalMAC:  localMAC,
		}
		response, err := v6Provider.HandlePacket(c.Ctx, pkt)
		if err != nil {
			c.logger.Error("DHCPv6 provider failed for REQUEST", "session_id", sessID, "error", err)
		} else if response != nil && len(response.Raw) > 0 {
			if err := c.sendDHCPv6Response(sess, response.Raw); err != nil {
				c.logger.Error("Failed to send DHCPv6 REPLY", "session_id", sessID, "error", err)
			}
			respMsg, parseErr := dhcp6.ParseMessage(response.Raw)
			if parseErr == nil && respMsg.MsgType == dhcp6.MsgTypeReply {
				c.handleDHCPv6Reply(sess, respMsg)
			}
		}
	}
}

func (c *Component) forwardLatePendingPackets(sess *SessionState, sessID string, mac net.HardwareAddr, svlan, cvlan uint16, encapIfIndex uint32, srgName string, allocCtx *allocator.Context, dhcpv6DUID []byte, pendingV4Discover, pendingV4Request, pendingV6Solicit, pendingV6Request []byte) {
	if pendingV4Discover == nil && pendingV4Request == nil && pendingV6Solicit == nil && pendingV6Request == nil {
		return
	}

	v4Profile := c.resolveIPv4Profile(allocCtx)
	v6Profile := c.resolveIPv6Profile(allocCtx)
	accessIfName := c.resolveAccessInterfaceName(encapIfIndex)
	localMAC := c.getLocalMAC(srgName, encapIfIndex)

	if pendingV4Discover != nil {
		c.logger.Info("Forwarding late-pending DHCP DISCOVER", "session_id", sessID)
		var resolved *dhcp.ResolvedDHCPv4
		if v4Profile == nil || v4Profile.GetMode() == "server" {
			resolved = c.resolveDHCPv4(allocCtx)
		}
		pkt := &dhcp4.Packet{
			SessionID: sessID,
			MAC:       mac.String(),
			SVLAN:     svlan,
			CVLAN:     cvlan,
			Raw:       pendingV4Discover,
			Resolved:  resolved,
			SwIfIndex: encapIfIndex,
			Interface: accessIfName,
			Profile:   v4Profile,
			LocalMAC:  localMAC,
		}
		provider := c.getDHCP4Provider(v4Profile)
		if provider != nil {
			response, err := provider.HandlePacket(c.Ctx, pkt)
			if err != nil {
				c.logger.Error("DHCP provider failed for late-pending DISCOVER", "session_id", sessID, "error", err)
			} else if response != nil && len(response.Raw) > 0 {
				if err := c.sendDHCPResponse(sessID, svlan, cvlan, encapIfIndex, mac, response.Raw, "OFFER"); err != nil {
					c.logger.Error("Failed to send DHCP OFFER", "session_id", sessID, "error", err)
				}
			}
		}
	}

	if pendingV4Request != nil {
		c.logger.Info("Forwarding late-pending DHCP REQUEST", "session_id", sessID)
		var resolved *dhcp.ResolvedDHCPv4
		if v4Profile == nil || v4Profile.GetMode() == "server" {
			resolved = c.resolveDHCPv4(allocCtx)
		}
		pkt := &dhcp4.Packet{
			SessionID: sessID,
			MAC:       mac.String(),
			SVLAN:     svlan,
			CVLAN:     cvlan,
			Raw:       pendingV4Request,
			Resolved:  resolved,
			SwIfIndex: encapIfIndex,
			Interface: accessIfName,
			Profile:   v4Profile,
			LocalMAC:  localMAC,
		}
		provider := c.getDHCP4Provider(v4Profile)
		if provider != nil {
			response, err := provider.HandlePacket(c.Ctx, pkt)
			if err != nil {
				c.logger.Error("DHCP provider failed for late-pending REQUEST", "session_id", sessID, "error", err)
			} else if response != nil && len(response.Raw) > 0 {
				if err := c.sendDHCPResponse(sessID, svlan, cvlan, encapIfIndex, mac, response.Raw, "ACK"); err != nil {
					c.logger.Error("Failed to send DHCP ACK", "session_id", sessID, "error", err)
				}
			}
		}
	}

	v6Provider := c.getDHCP6Provider(v6Profile)

	if pendingV6Solicit != nil && v6Provider != nil {
		c.logger.Info("Forwarding late-pending DHCPv6 SOLICIT", "session_id", sessID)
		var resolved *dhcp.ResolvedDHCPv6
		if v6Profile == nil || v6Profile.GetMode() == "server" {
			resolved = c.resolveDHCPv6(allocCtx)
		}
		pkt := &dhcp6.Packet{
			SessionID: sessID,
			MAC:       mac.String(),
			SVLAN:     svlan,
			CVLAN:     cvlan,
			DUID:      dhcpv6DUID,
			Raw:       pendingV6Solicit,
			Resolved:  resolved,
			SwIfIndex: encapIfIndex,
			Interface: accessIfName,
			PeerAddr:  sess.ClientLinkLocal,
			Profile:   v6Profile,
			LocalMAC:  localMAC,
		}
		response, err := v6Provider.HandlePacket(c.Ctx, pkt)
		if err != nil {
			c.logger.Error("DHCPv6 provider failed for late-pending SOLICIT", "session_id", sessID, "error", err)
		} else if response != nil && len(response.Raw) > 0 {
			if err := c.sendDHCPv6Response(sess, response.Raw); err != nil {
				c.logger.Error("Failed to send DHCPv6 ADVERTISE", "session_id", sessID, "error", err)
			}
		}
	}

	if pendingV6Request != nil && v6Provider != nil {
		c.logger.Info("Forwarding late-pending DHCPv6 REQUEST", "session_id", sessID)
		var resolved *dhcp.ResolvedDHCPv6
		if v6Profile == nil || v6Profile.GetMode() == "server" {
			resolved = c.resolveDHCPv6(allocCtx)
		}
		pkt := &dhcp6.Packet{
			SessionID: sessID,
			MAC:       mac.String(),
			SVLAN:     svlan,
			CVLAN:     cvlan,
			DUID:      dhcpv6DUID,
			Raw:       pendingV6Request,
			Resolved:  resolved,
			SwIfIndex: encapIfIndex,
			Interface: accessIfName,
			PeerAddr:  sess.ClientLinkLocal,
			Profile:   v6Profile,
			LocalMAC:  localMAC,
		}
		response, err := v6Provider.HandlePacket(c.Ctx, pkt)
		if err != nil {
			c.logger.Error("DHCPv6 provider failed for late-pending REQUEST", "session_id", sessID, "error", err)
		} else if response != nil && len(response.Raw) > 0 {
			if err := c.sendDHCPv6Response(sess, response.Raw); err != nil {
				c.logger.Error("Failed to send DHCPv6 REPLY", "session_id", sessID, "error", err)
			}
			respMsg, parseErr := dhcp6.ParseMessage(response.Raw)
			if parseErr == nil && respMsg.MsgType == dhcp6.MsgTypeReply {
				c.handleDHCPv6Reply(sess, respMsg)
			}
		}
	}
}

func (c *Component) forwardDHCPv6ToProvider(sess *SessionState, pkt *dataplane.ParsedPacket, raw []byte) error {
	v6Profile := c.resolveIPv6Profile(sess.AllocCtx)
	var resolved *dhcp.ResolvedDHCPv6
	if v6Profile == nil || v6Profile.GetMode() == "server" {
		resolved = c.resolveDHCPv6(sess.AllocCtx)
	}
	dhcpPkt := &dhcp6.Packet{
		SessionID: sess.SessionID,
		MAC:       sess.MAC.String(),
		SVLAN:     sess.OuterVLAN,
		CVLAN:     sess.InnerVLAN,
		DUID:      sess.DHCPv6DUID,
		Raw:       raw,
		Resolved:  resolved,
		SwIfIndex: sess.EncapIfIndex,
		Interface: c.resolveAccessInterfaceName(sess.EncapIfIndex),
		PeerAddr:  sess.ClientLinkLocal,
		Profile:   v6Profile,
		LocalMAC:  c.getLocalMAC(sess.SRGName, sess.EncapIfIndex),
	}

	provider := c.getDHCP6Provider(v6Profile)
	if provider == nil {
		return fmt.Errorf("no DHCPv6 provider available")
	}
	response, err := provider.HandlePacket(c.Ctx, dhcpPkt)
	if err != nil {
		return fmt.Errorf("dhcp6 provider failed: %w", err)
	}

	if response == nil || len(response.Raw) == 0 {
		return nil
	}

	if err := c.sendDHCPv6Response(sess, response.Raw); err != nil {
		return err
	}

	respMsg, err := dhcp6.ParseMessage(response.Raw)
	if err == nil && respMsg.MsgType == dhcp6.MsgTypeReply {
		return c.handleDHCPv6Reply(sess, respMsg)
	}

	return nil
}

func (c *Component) handleDHCPv6Reply(sess *SessionState, msg *dhcp6.Message) error {
	var ianaAddr net.IP
	var pdPrefix *net.IPNet
	var validTime uint32

	if msg.Options.IANA != nil && msg.Options.IANA.Address != nil {
		ianaAddr = msg.Options.IANA.Address
		validTime = msg.Options.IANA.ValidTime
	}

	if msg.Options.IAPD != nil && msg.Options.IAPD.Prefix != nil {
		pdPrefix = &net.IPNet{
			IP:   msg.Options.IAPD.Prefix,
			Mask: net.CIDRMask(int(msg.Options.IAPD.PrefixLen), 128),
		}
	}

	sess.mu.Lock()
	sess.IPv6Address = ianaAddr
	sess.IPv6Prefix = pdPrefix
	sess.IPv6LeaseTime = validTime
	sess.IPv6BoundAt = time.Now()
	sess.IPv6Bound = true
	ipoeSwIfIndex := sess.IPoESwIfIndex
	v4AlreadyBound := sess.State == "bound" && sess.IPv4 != nil
	snapshotIPv4 := sess.IPv4
	snapshotLeaseTime := sess.LeaseTime
	sess.mu.Unlock()

	c.logger.Info("DHCPv6 session bound", "session_id", sess.SessionID, "ipv6", ianaAddr, "prefix", pdPrefix)

	if c.vpp != nil {
		sessID := sess.SessionID
		if ipoeSwIfIndex != 0 {
			if ianaAddr != nil {
				c.vpp.IPoESetSessionIPv6Async(ipoeSwIfIndex, ianaAddr, true, func(err error) {
					if err != nil {
						if errors.Is(err, southbound.ErrUnavailable) {
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
						if errors.Is(err, southbound.ErrUnavailable) {
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
			sess.mu.Lock()
			sess.PendingIPv6Binding = ianaAddr
			sess.PendingPDBinding = pdPrefix
			sess.mu.Unlock()
			c.logger.Debug("IPoE session not ready, queued IPv6 bindings", "session_id", sessID)
		}
	}

	c.checkpointSession(sess)

	var prefixStr string
	if pdPrefix != nil {
		prefixStr = pdPrefix.String()
	}

	sessionMode := c.getSessionMode(sess.OuterVLAN)
	if sessionMode == subscriber.SessionModeUnified && !v4AlreadyBound {
		return nil
	}

	ipoeSess := &models.IPoESession{
		SessionID:     sess.SessionID,
		State:         models.SessionStateActive,
		AccessType:    string(models.AccessTypeIPoE),
		Protocol:      string(models.ProtocolDHCPv6),
		MAC:           sess.MAC,
		OuterVLAN:     sess.OuterVLAN,
		InnerVLAN:     sess.InnerVLAN,
		VLANCount:     c.getVLANCount(sess.OuterVLAN, sess.InnerVLAN),
		IfIndex:       ipoeSwIfIndex,
		VRF:           sess.VRF,
		ServiceGroup:  sess.ServiceGroup.Name,
		SRGName:       sess.SRGName,
		IPv4Address:   snapshotIPv4,
		LeaseTime:     snapshotLeaseTime,
		IPv6Address:   ianaAddr,
		IPv6Prefix:    prefixStr,
		IPv6LeaseTime: sess.IPv6LeaseTime,
		DUID:          sess.DHCPv6DUID,
		Username:      sess.Username,
		AAASessionID:  sess.AcctSessionID,
		ActivatedAt:   time.Now(),
		OuterTPID:     sess.OuterTPID,
	}
	if sess.AllocCtx != nil {
		ipoeSess.IPv4Pool = sess.AllocCtx.AllocatedPool
		ipoeSess.IANAPool = sess.AllocCtx.AllocatedIANAPool
		ipoeSess.PDPool = sess.AllocCtx.AllocatedPDPool
	}

	return c.publishSessionLifecycle(ipoeSess)
}

func (c *Component) sendDHCPv6Response(sess *SessionState, rawDHCPv6 []byte) error {
	var srcMACBytes net.HardwareAddr
	var parentSwIfIndex uint32

	if c.srgMgr != nil {
		srcMACBytes = c.srgMgr.GetVirtualMAC(sess.SRGName)
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

	frame := dhcp.BuildIPv6UDPFrame(srcIP, dstIP, 547, 546, rawDHCPv6)
	if frame == nil {
		return fmt.Errorf("failed to build IPv6/UDP frame")
	}

	egressPayload := &models.EgressPacketPayload{
		DstMAC:    sess.MAC.String(),
		SrcMAC:    srcMAC,
		OuterVLAN: sess.OuterVLAN,
		InnerVLAN: sess.InnerVLAN,
		OuterTPID: c.getSessionOuterTPID(sess),
		SwIfIndex: parentSwIfIndex,
		RawData:   frame,
	}

	c.logger.Debug("Sending DHCPv6 response", "session_id", sess.SessionID, "size", len(rawDHCPv6), "dst_ip", dstIP)

	c.eventBus.Publish(events.TopicEgress, events.Event{
		Source: c.Name(),
		Data: &events.EgressEvent{
			Protocol: models.ProtocolDHCPv6,
			Packet:   *egressPayload,
		},
	})
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

	var prefixes []raPrefixInfo

	if group != nil && group.IPv6Profile != "" {
		if profile := cfg.IPv6Profiles[group.IPv6Profile]; profile != nil {
			for _, pool := range profile.IANAPools {
				prefixes = append(prefixes, raPrefixInfo{
					network:       pool.Network,
					validTime:     pool.ValidTime,
					preferredTime: pool.PreferredTime,
				})
			}
		}
	}

	if len(prefixes) == 0 {
		for _, pool := range cfg.DHCPv6.IANAPools {
			prefixes = append(prefixes, raPrefixInfo{
				network:       pool.Network,
				validTime:     pool.ValidTime,
				preferredTime: pool.PreferredTime,
			})
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
		srgName := c.resolveSRGName(pkt.OuterVLAN)
		srcMACBytes = c.srgMgr.GetVirtualMAC(srgName)
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

	c.logger.Debug("Sending RA response",
		"dst_mac", pkt.MAC.String(),
		"src_mac", srcMACBytes.String(),
		"svlan", pkt.OuterVLAN,
		"cvlan", pkt.InnerVLAN,
		"managed", raConfig.Managed,
		"other", raConfig.Other,
	)

	c.eventBus.Publish(events.TopicEgress, events.Event{
		Source: c.Name(),
		Data: &events.EgressEvent{
			Protocol: models.ProtocolIPv6ND,
			Packet:   *egressPayload,
		},
	})
	return nil
}

func (c *Component) resolveUnnumberedLoopback(sess *SessionState) string {
	if sess.ServiceGroup.Unnumbered != "" {
		return sess.ServiceGroup.Unnumbered
	}

	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg.SubscriberGroups == nil {
		return ""
	}

	if group, _ := cfg.SubscriberGroups.FindGroupBySVLAN(sess.OuterVLAN); group != nil {
		return group.FindGatewayForSVLAN(sess.OuterVLAN)
	}

	return ""
}

func (c *Component) setupSessionUnnumbered(sessID string, swIfIndex uint32, loopback string) {
	if loopback == "" {
		return
	}

	c.vpp.SetUnnumberedAsync(swIfIndex, loopback, func(err error) {
		if err != nil {
			c.logger.Error("Failed to set unnumbered on IPoE session", "session_id", sessID, "sw_if_index", swIfIndex, "loopback", loopback, "error", err)
		}
	})
}

func (c *Component) publishSessionProgrammed(sess *SessionState, swIfIndex uint32) {
	sess.mu.Lock()
	ipoeSess := &models.IPoESession{
		SessionID:    sess.SessionID,
		State:        models.SessionStateActive,
		AccessType:   string(models.AccessTypeIPoE),
		Protocol:     string(models.ProtocolDHCPv4),
		MAC:          sess.MAC,
		OuterVLAN:    sess.OuterVLAN,
		InnerVLAN:    sess.InnerVLAN,
		VLANCount:    c.getVLANCount(sess.OuterVLAN, sess.InnerVLAN),
		IfIndex:      swIfIndex,
		VRF:          sess.VRF,
		ServiceGroup: sess.ServiceGroup.Name,
		SRGName:      sess.SRGName,
		IPv4Address:  sess.IPv4,
		Username:     sess.Username,
		AAASessionID: sess.AcctSessionID,
	}
	sess.mu.Unlock()

	c.eventBus.Publish(events.TopicSessionProgrammed, events.Event{
		Source: c.Name(),
		Data: &events.SessionLifecycleEvent{
			AccessType: ipoeSess.GetAccessType(),
			Protocol:   ipoeSess.GetProtocol(),
			SessionID:  ipoeSess.GetSessionID(),
			State:      ipoeSess.GetState(),
			Session:    ipoeSess,
		},
	})
}

func (c *Component) publishSessionLifecycle(payload models.SubscriberSession) error {
	c.eventBus.Publish(events.TopicSessionLifecycle, events.Event{
		Source: c.Name(),
		Data: &events.SessionLifecycleEvent{
			AccessType: payload.GetAccessType(),
			Protocol:   payload.GetProtocol(),
			SessionID:  payload.GetSessionID(),
			State:      payload.GetState(),
			Session:    payload,
		},
	})
	return nil
}

func (c *Component) checkpointSession(sess *SessionState) {
	if c.opdb == nil {
		return
	}

	sess.mu.Lock()
	sessID := sess.SessionID
	data, err := json.Marshal(sess)
	sess.mu.Unlock()
	if err != nil {
		c.logger.Warn("Failed to marshal session for checkpoint", "session_id", sessID, "error", err)
		return
	}

	go func() {
		if err := c.opdb.Put(c.Ctx, opdb.NamespaceIPoESessions, sessID, data); err != nil {
			c.logger.Warn("Failed to checkpoint session", "session_id", sessID, "error", err)
		}
	}()
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
			c.logger.Info("VPP interface not found, recreating IPoE session",
				"session_id", sess.SessionID,
				"stale_sw_if_index", sess.IPoESwIfIndex)

			var decapVrfID uint32
			if sess.VRF != "" && c.vrfMgr != nil {
				if tableID, _, _, err := c.vrfMgr.ResolveVRF(sess.VRF); err == nil {
					decapVrfID = tableID
				}
			}

			localMAC := c.getLocalMAC(sess.SRGName, sess.EncapIfIndex)
			if localMAC != nil && c.vpp != nil {
				swIfIndex, err := c.vpp.AddIPoESession(sess.MAC, localMAC, sess.EncapIfIndex, sess.OuterVLAN, sess.InnerVLAN, decapVrfID)
				if err != nil {
					c.logger.Error("Failed to recreate IPoE session", "session_id", sess.SessionID, "error", err)
					sess.IPoESwIfIndex = 0
					sess.IPoESessionCreated = false
				} else {
					c.logger.Info("Recreated IPoE session in VPP", "session_id", sess.SessionID, "sw_if_index", swIfIndex)
					sess.IPoESwIfIndex = swIfIndex
					sess.IPoESessionCreated = true
					c.setupSessionUnnumbered(sess.SessionID, swIfIndex, c.resolveUnnumberedLoopback(&sess))
				}
			} else {
				sess.IPoESwIfIndex = 0
				sess.IPoESessionCreated = false
			}

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

		sessPtr := &sess
		c.sessions.Store(lookupKey, sessPtr)
		c.sessionIndex.Store(sess.SessionID, sessPtr)

		if sess.State == "bound" && sess.MAC != nil {
			counterKey := fmt.Sprintf("osvbng:session_count:%s:%d:%d", sess.MAC.String(), sess.OuterVLAN, sess.InnerVLAN)
			sessionCounts[counterKey]++
		}

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
		SessionID:     sess.SessionID,
		State:         models.SessionStateActive,
		AccessType:    string(models.AccessTypeIPoE),
		Protocol:      protocol,
		MAC:           sess.MAC,
		OuterVLAN:     sess.OuterVLAN,
		InnerVLAN:     sess.InnerVLAN,
		VLANCount:     c.getVLANCount(sess.OuterVLAN, sess.InnerVLAN),
		IfIndex:       sess.IPoESwIfIndex,
		VRF:           sess.VRF,
		ServiceGroup:  sess.ServiceGroup.Name,
		SRGName:       sess.SRGName,
		IPv4Address:   sess.IPv4,
		LeaseTime:     sess.LeaseTime,
		IPv6Address:   sess.IPv6Address,
		IPv6LeaseTime: sess.IPv6LeaseTime,
		DUID:          sess.DHCPv6DUID,
		AAASessionID:  sess.AcctSessionID,
		ActivatedAt:   sess.BoundAt,
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

func (c *Component) sessionCount() int {
	n := 0
	c.sessions.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}

func (c *Component) RecoverSessions(ctx context.Context) error {
	total := c.sessionCount()

	if total == 0 {
		c.logger.Info("No IPoE sessions to recover")
		return nil
	}

	c.logger.Info("Recovering IPoE sessions from OpDB", "total_in_memory", total)

	if err := c.restoreSessions(ctx); err != nil {
		return fmt.Errorf("recover ipoe sessions: %w", err)
	}

	recovered := c.sessionCount()

	c.logger.Info("IPoE session recovery complete", "recovered", recovered)
	return nil
}

func (c *Component) handleHAStateChange(event events.Event) {
	data, ok := event.Data.(events.HAStateChangeEvent)
	if !ok {
		return
	}

	wasActive := data.OldState == string(ha.SRGStateActive) || data.OldState == string(ha.SRGStateActiveSolo)
	isActive := data.NewState == string(ha.SRGStateActive) || data.NewState == string(ha.SRGStateActiveSolo)
	wasStandbyAlone := data.OldState == string(ha.SRGStateStandbyAlone)

	if isActive && !wasActive && wasStandbyAlone {
		c.logger.Info("SRG promoted from standby alone, restoring synced IPoE sessions", "srg", data.SRGName)
		go c.restoreFromHASync(data.SRGName)
	}
}

func (c *Component) restoreFromHASync(srgName string) {
	if c.opdb == nil || c.vpp == nil {
		return
	}

	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil {
		c.logger.Error("Failed to get running config for HA restore", "error", err)
		return
	}

	srgCfg, ok := cfg.HA.SRGs[srgName]
	if !ok || len(srgCfg.Interfaces) == 0 {
		c.logger.Error("SRG config not found or no interfaces", "srg", srgName)
		return
	}

	// At some point when we want to support multi-interfaces on the same
	// srg, we can't hardcode the ifIndex to 0, I'm not sure the best action here
	// so its a problem in the future, SRGs right now are mostly tied to S-VLAN
	// therefore its probably a very specific scenario someone wants this feature...
	encapIfIndex, ok := c.ifMgr.GetSwIfIndex(srgCfg.Interfaces[0])
	if !ok {
		c.logger.Error("Failed to resolve SRG access interface",
			"srg", srgName,
			"interface", srgCfg.Interfaces[0])
		return
	}

	type checkpoint struct {
		key  string
		data []byte
	}
	var checkpoints []checkpoint

	c.opdb.Load(c.Ctx, opdb.NamespaceHASyncedIPoE, func(key string, value []byte) error {
		cp := make([]byte, len(value))
		copy(cp, value)
		checkpoints = append(checkpoints, checkpoint{key: key, data: cp})
		return nil
	})

	if len(checkpoints) == 0 {
		c.logger.Info("No synced IPoE sessions to restore", "srg", srgName)
		return
	}

	c.logger.Info("Restoring synced IPoE sessions", "srg", srgName, "count", len(checkpoints))

	var restored, failed int
	now := time.Now()

	for _, entry := range checkpoints {
		var cp hapb.SessionCheckpoint
		if err := proto.Unmarshal(entry.data, &cp); err != nil {
			c.logger.Warn("Failed to unmarshal synced IPoE checkpoint", "key", entry.key, "error", err)
			failed++
			continue
		}

		if cp.SrgName != srgName {
			continue
		}

		mac := net.HardwareAddr(cp.Mac)
		outerVLAN := uint16(cp.OuterVlan)
		innerVLAN := uint16(cp.InnerVlan)

		var decapVrfID uint32
		if cp.Vrf != "" && c.vrfMgr != nil {
			tableID, _, _, err := c.vrfMgr.ResolveVRF(cp.Vrf)
			if err != nil {
				c.logger.Error("Failed to resolve VRF for HA restore",
					"session_id", cp.SessionId, "vrf", cp.Vrf, "error", err)
				failed++
				continue
			}
			decapVrfID = tableID
		}

		localMAC := c.getLocalMAC(srgName, encapIfIndex)
		if localMAC == nil {
			c.logger.Error("No local MAC available for HA restore", "session_id", cp.SessionId)
			failed++
			continue
		}

		swIfIndex, err := c.vpp.AddIPoESession(mac, localMAC, encapIfIndex, outerVLAN, innerVLAN, decapVrfID)
		if err != nil {
			c.logger.Error("Failed to create IPoE session from HA sync",
				"session_id", cp.SessionId, "error", err)
			failed++
			continue
		}

		var ipv4 net.IP
		if len(cp.Ipv4Address) > 0 {
			ipv4 = net.IP(cp.Ipv4Address)
		}

		var ipv6 net.IP
		if len(cp.Ipv6Address) > 0 {
			ipv6 = net.IP(cp.Ipv6Address)
		}

		var ipv6Prefix *net.IPNet
		if len(cp.Ipv6Prefix) > 0 && cp.Ipv6PrefixLen > 0 {
			ipv6Prefix = &net.IPNet{
				IP:   net.IP(cp.Ipv6Prefix),
				Mask: net.CIDRMask(int(cp.Ipv6PrefixLen), 128),
			}
		}

		var boundAt time.Time
		if cp.BoundAtNs > 0 {
			boundAt = time.Unix(0, cp.BoundAtNs)
		} else {
			boundAt = now
		}

		sess := &SessionState{
			SessionID:          cp.SessionId,
			AcctSessionID:      cp.AaaSessionId,
			MAC:                mac,
			OuterVLAN:          outerVLAN,
			InnerVLAN:          innerVLAN,
			OuterTPID:          uint16(cp.OuterTpid),
			EncapIfIndex:       encapIfIndex,
			IPoESwIfIndex:      swIfIndex,
			IPoESessionCreated: true,
			State:              "bound",
			IPv4:               ipv4,
			IPv6Address:        ipv6,
			IPv6Prefix:         ipv6Prefix,
			IPv6Bound:          ipv6 != nil || ipv6Prefix != nil,
			LeaseTime:          cp.Ipv4LeaseTime,
			IPv6LeaseTime:      cp.Ipv6LeaseTime,
			BoundAt:            boundAt,
			IPv6BoundAt:        boundAt,
			AAAApproved:        true,
			Username:           cp.Username,
			VRF:                cp.Vrf,
			SRGName:            srgName,
			CircuitID:          cp.CircuitId,
			RemoteID:           cp.RemoteId,
			ClientID:           cp.ClientId,
			Hostname:           cp.Hostname,
			DHCPv6DUID:         cp.Dhcpv6Duid,
		}

		if cp.ServiceGroup != "" {
			var aaaAttrs map[string]interface{}
			if len(cp.AaaAttributes) > 0 {
				aaaAttrs = make(map[string]interface{}, len(cp.AaaAttributes))
				for k, v := range cp.AaaAttributes {
					aaaAttrs[k] = v
				}
				sess.Attributes = cp.AaaAttributes
			}
			sess.ServiceGroup = c.svcGroupResolver.Resolve(cp.ServiceGroup, cp.ServiceGroup, aaaAttrs)
		}

		unnumberedLoopback := c.resolveUnnumberedLoopback(sess)
		c.setupSessionUnnumbered(cp.SessionId, swIfIndex, unnumberedLoopback)

		if ipv4 != nil {
			if err := c.vpp.IPoESetSessionIPv4(swIfIndex, ipv4, true); err != nil {
				c.logger.Error("Failed to bind IPv4 during HA restore",
					"session_id", cp.SessionId, "error", err)
			}
		}

		if ipv6 != nil {
			if err := c.vpp.IPoESetSessionIPv6(swIfIndex, ipv6, true); err != nil {
				c.logger.Error("Failed to bind IPv6 during HA restore",
					"session_id", cp.SessionId, "error", err)
			}
		}

		if ipv6Prefix != nil {
			nextHop := ipv6
			if nextHop == nil {
				nextHop = net.ParseIP("::")
			}
			if err := c.vpp.IPoESetDelegatedPrefix(swIfIndex, *ipv6Prefix, nextHop, true); err != nil {
				c.logger.Error("Failed to bind delegated prefix during HA restore",
					"session_id", cp.SessionId, "error", err)
			}
		}

		lookupKey := c.makeSessionKeyV4(mac, outerVLAN, innerVLAN)

		c.sessions.Store(lookupKey, sess)
		c.sessionIndex.Store(cp.SessionId, sess)

		c.restoreSessionToCache(c.Ctx, sess, now)
		c.checkpointSession(sess)
		c.publishSessionProgrammed(sess, swIfIndex)

		c.opdb.Delete(c.Ctx, opdb.NamespaceHASyncedIPoE, cp.SessionId)

		restored++
		c.logger.Info("Restored IPoE session from HA sync",
			"session_id", cp.SessionId,
			"mac", mac,
			"ipv4", ipv4,
			"sw_if_index", swIfIndex)
	}

	c.logger.Info("HA IPoE session restore complete",
		"srg", srgName,
		"restored", restored,
		"failed", failed)
}

func (c *Component) ForEachSession(fn func(models.SubscriberSession) bool) {
	c.sessions.Range(func(_, v any) bool {
		sess := v.(*SessionState)
		sess.mu.Lock()
		if sess.State != "bound" {
			sess.mu.Unlock()
			return true
		}

		snapshot := &models.IPoESession{
			SessionID:     sess.SessionID,
			State:         models.SessionStateActive,
			AccessType:    string(models.AccessTypeIPoE),
			MAC:           sess.MAC,
			OuterVLAN:     sess.OuterVLAN,
			InnerVLAN:     sess.InnerVLAN,
			VLANCount:     c.getVLANCount(sess.OuterVLAN, sess.InnerVLAN),
			IfIndex:       sess.IPoESwIfIndex,
			VRF:           sess.VRF,
			ServiceGroup:  sess.ServiceGroup.Name,
			SRGName:       sess.SRGName,
			IPv4Address:   sess.IPv4,
			LeaseTime:     sess.LeaseTime,
			IPv6Address:   sess.IPv6Address,
			IPv6LeaseTime: sess.IPv6LeaseTime,
			DUID:          sess.DHCPv6DUID,
			ClientID:      sess.ClientID,
			Hostname:      sess.Hostname,
			Username:      sess.Username,
			AAASessionID:  sess.AcctSessionID,
			ActivatedAt:   sess.BoundAt,
			OuterTPID:     sess.OuterTPID,
			Attributes:    sess.Attributes,
			RelayInfo:     map[uint8][]byte{},
		}
		if sess.AllocCtx != nil {
			snapshot.IPv4Pool = sess.AllocCtx.AllocatedPool
			snapshot.IANAPool = sess.AllocCtx.AllocatedIANAPool
			snapshot.PDPool = sess.AllocCtx.AllocatedPDPool
		}
		if sess.IPv6Prefix != nil {
			snapshot.IPv6Prefix = sess.IPv6Prefix.String()
		}
		if len(sess.CircuitID) > 0 {
			snapshot.RelayInfo[1] = sess.CircuitID
		}
		if len(sess.RemoteID) > 0 {
			snapshot.RelayInfo[2] = sess.RemoteID
		}
		sess.mu.Unlock()

		return fn(snapshot)
	})
}
