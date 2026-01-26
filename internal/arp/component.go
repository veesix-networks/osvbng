package arp

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/srg"
)

type Component struct {
	*component.Base

	logger   *slog.Logger
	eventBus events.Bus
	srgMgr   *srg.Manager
	vpp      interface {
		GetParentInterfaceMAC() net.HardwareAddr
	}
	configMgr component.ConfigManager
	arpChan   <-chan *dataplane.ParsedPacket
}

func New(deps component.Dependencies, srgMgr *srg.Manager) (component.Component, error) {
	log := logger.Component(logger.ComponentARP)

	return &Component{
		Base:      component.NewBase("arp"),
		logger:    log,
		eventBus:  deps.EventBus,
		srgMgr:    srgMgr,
		vpp:       deps.VPP,
		configMgr: deps.ConfigManager,
		arpChan:   deps.ARPChan,
	}, nil
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
		c.logger.Warn("ARP packet processing time",
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
	srcIP := net.IP(arp.SourceProtAddress)
	dstIP := net.IP(arp.DstProtAddress)

	c.logger.Debug("ARP request",
		"sw_if_index", pkt.SwIfIndex,
		"src_mac", srcMAC.String(),
		"src_ip", srcIP.String(),
		"dst_ip", dstIP.String(),
	)

	if !c.isOwnedIP(dstIP) {
		c.logger.Debug("Ignoring ARP request for non-owned IP", "dst_ip", dstIP.String())
		return nil
	}

	var gatewayMAC net.HardwareAddr
	if c.srgMgr != nil {
		gatewayMAC = c.srgMgr.GetVirtualMAC(0)
	}
	if gatewayMAC == nil && c.vpp != nil {
		gatewayMAC = c.vpp.GetParentInterfaceMAC()
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

func (c *Component) isOwnedIP(ip net.IP) bool {
	if c.configMgr == nil {
		return false
	}

	cfg, err := c.configMgr.GetRunning()
	if err != nil || cfg == nil || cfg.Interfaces == nil {
		return false
	}

	ipStr := ip.String()
	for _, iface := range cfg.Interfaces {
		if iface.Address == nil {
			continue
		}
		for _, addr := range iface.Address.IPv4 {
			ip, _, err := net.ParseCIDR(addr)
			if err == nil && ip.String() == ipStr {
				return true
			}
		}
	}

	return false
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
