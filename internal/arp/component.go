package arp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/srg"
	"github.com/veesix-networks/osvbng/pkg/vrfmgr"
)

type Component struct {
	*component.Base

	logger    *slog.Logger
	eventBus  events.Bus
	cache     cache.Cache
	srgMgr    *srg.Manager
	ifMgr     *ifmgr.Manager
	vrfMgr    *vrfmgr.Manager
	configMgr component.ConfigManager
	arpChan   <-chan *dataplane.ParsedPacket
}

func New(deps component.Dependencies, srgMgr *srg.Manager, ifMgr *ifmgr.Manager) (component.Component, error) {
	log := logger.Get(logger.ARP)

	return &Component{
		Base:      component.NewBase("arp"),
		logger:    log,
		eventBus:  deps.EventBus,
		cache:     deps.Cache,
		srgMgr:    srgMgr,
		ifMgr:     ifMgr,
		vrfMgr:    deps.VRFManager,
		configMgr: deps.ConfigManager,
		arpChan:   deps.ARPChan,
	}, nil
}

func (c *Component) resolveOuterTPID(svlan uint16) uint16 {
	cfg, err := c.configMgr.GetRunning()
	if err != nil || cfg == nil || cfg.SubscriberGroups == nil {
		return 0x88A8
	}
	group, _ := cfg.SubscriberGroups.FindGroupBySVLAN(svlan)
	if group == nil {
		return 0x88A8
	}
	return group.GetOuterTPID()
}

func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)
	c.logger.Info("Starting ARP component")

	c.Go(c.readLoop)

	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.logger.Info("Stopping ARP component")
	c.StopContext()
	return nil
}

func (c *Component) readLoop() {
	for {
		select {
		case <-c.Ctx.Done():
			return
		case pkt := <-c.arpChan:
			if err := c.handlePacket(pkt); err != nil {
				c.logger.Debug("Error handling packet", "error", err, "sw_if_index", pkt.SwIfIndex)
			}
		}
	}
}

func (c *Component) handlePacket(pkt *dataplane.ParsedPacket) error {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		c.logger.Debug("ARP packet processing time",
			"duration_us", duration.Microseconds(),
			"sw_if_index", pkt.SwIfIndex)
	}()

	if pkt.ARP == nil {
		return fmt.Errorf("no ARP layer")
	}

	arp := pkt.ARP

	if arp.Operation != 1 {
		return nil
	}

	srcMAC := net.HardwareAddr(arp.SourceHwAddress)
	dstIP := net.IP(arp.DstProtAddress)

	c.logger.Debug("ARP request",
		"sw_if_index", pkt.SwIfIndex,
		"src_mac", srcMAC.String(),
		"dst_ip", dstIP.String(),
	)

	if c.ifMgr == nil || !c.ifMgr.HasIPv4(dstIP) {
		c.logger.Debug("Ignoring ARP request for non-owned IP",
			"dst_ip", dstIP.String())
		return nil
	}

	sess := c.lookupSubscriberSession(pkt)
	if sess != nil && sess.VRF != "" {
		if !c.isOwnedIPInVRF(dstIP, sess.VRF) {
			c.logger.Debug("Ignoring ARP request for IP not in subscriber VRF",
				"dst_ip", dstIP.String(),
				"vrf", sess.VRF)
			return nil
		}
	}

	var gatewayMAC net.HardwareAddr
	var parentSwIfIndex uint32
	if c.srgMgr != nil {
		gatewayMAC = c.srgMgr.GetVirtualMAC(0)
	}
	if c.ifMgr != nil {
		if iface := c.ifMgr.Get(pkt.SwIfIndex); iface != nil {
			parentSwIfIndex = iface.SupSwIfIndex
		}
		if gatewayMAC == nil {
			if parent := c.ifMgr.Get(parentSwIfIndex); parent != nil && len(parent.MAC) >= 6 {
				gatewayMAC = net.HardwareAddr(parent.MAC[:6])
			}
		}
	}
	if gatewayMAC == nil {
		return fmt.Errorf("no gateway MAC available for ARP reply")
	}

	arpReply := c.buildARPReply(arp, dstIP, gatewayMAC)
	if arpReply == nil {
		return nil
	}

	egressPayload := &models.EgressPacketPayload{
		DstMAC:    srcMAC.String(),
		SrcMAC:    gatewayMAC.String(),
		OuterVLAN: pkt.OuterVLAN,
		InnerVLAN: pkt.InnerVLAN,
		OuterTPID: c.resolveOuterTPID(pkt.OuterVLAN),
		SwIfIndex: parentSwIfIndex,
		RawData:   arpReply,
	}

	egressEvent := models.Event{
		Type:       models.EventTypeEgress,
		AccessType: models.AccessTypeIPoE,
		Protocol:   models.ProtocolARP,
	}
	egressEvent.SetPayload(egressPayload)

	if err := c.eventBus.Publish(events.TopicEgress, egressEvent); err != nil {
		return fmt.Errorf("publish egress event: %w", err)
	}

	c.logger.Debug("Sent ARP reply", "target_ip", dstIP.String(), "gateway_mac", gatewayMAC.String())
	return nil
}

func (c *Component) lookupSubscriberSession(pkt *dataplane.ParsedPacket) *models.IPoESession {
	if c.cache == nil {
		return nil
	}

	srcMAC := net.HardwareAddr(pkt.ARP.SourceHwAddress)
	lookupKey := fmt.Sprintf("osvbng:lookup:ipoe:%s:%d:%d", srcMAC.String(), pkt.OuterVLAN, pkt.InnerVLAN)

	sessionIDBytes, err := c.cache.Get(c.Ctx, lookupKey)
	if err != nil || len(sessionIDBytes) == 0 {
		return nil
	}

	sessionID := string(sessionIDBytes)
	sessionKey := fmt.Sprintf("osvbng:sessions:%s", sessionID)

	sessionData, err := c.cache.Get(c.Ctx, sessionKey)
	if err != nil || len(sessionData) == 0 {
		return nil
	}

	var sess models.IPoESession
	if err := json.Unmarshal(sessionData, &sess); err != nil {
		c.logger.Debug("Failed to unmarshal session", "session_id", sessionID, "error", err)
		return nil
	}

	return &sess
}

func (c *Component) isOwnedIPInVRF(ip net.IP, vrfName string) bool {
	if c.ifMgr == nil || c.vrfMgr == nil {
		return false
	}

	tableID, _, _, err := c.vrfMgr.ResolveVRF(vrfName)
	if err != nil {
		c.logger.Debug("Failed to resolve VRF", "vrf", vrfName, "error", err)
		return false
	}

	return c.ifMgr.HasIPv4InFIB(ip, tableID)
}

func (c *Component) buildARPReply(req *layers.ARP, targetIP net.IP, gatewayMAC net.HardwareAddr) []byte {
	reply := &layers.ARP{
		AddrType:          layers.LinkTypeEthernet,
		Protocol:          layers.EthernetTypeIPv4,
		HwAddressSize:     6,
		ProtAddressSize:   4,
		Operation:         2,
		SourceHwAddress:   []byte(gatewayMAC),
		SourceProtAddress: []byte(targetIP),
		DstHwAddress:      req.SourceHwAddress,
		DstProtAddress:    req.SourceProtAddress,
	}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{}
	if err := reply.SerializeTo(buf, opts); err != nil {
		c.logger.Warn("Failed to serialize ARP reply", "error", err)
		return nil
	}

	return buf.Bytes()
}
