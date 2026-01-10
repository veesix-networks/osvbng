package dataplane

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/dataplane/vpp"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

type Component struct {
	*component.Base

	logger       *slog.Logger
	eventBus     events.Bus
	ingress      dataplane.Ingress
	memifHandler *vpp.MemifHandler
	vpp          *southbound.VPP
	virtualMAC   string

	DHCPChan  chan *dataplane.ParsedPacket
	ARPChan   chan *dataplane.ParsedPacket
	PPPoEChan chan *dataplane.ParsedPacket
}

func New(deps component.Dependencies) (component.Component, error) {
	puntSocketPath := "/run/osvbng/osvbng-punt.sock"
	if deps.Config != nil && deps.Config.Dataplane.PuntSocketPath != "" {
		puntSocketPath = deps.Config.Dataplane.PuntSocketPath
	}

	ingress := vpp.NewPuntSocketIngress(puntSocketPath)
	if err := ingress.Init(""); err != nil {
		return nil, fmt.Errorf("init punt socket: %w", err)
	}

	log := logger.Component(logger.ComponentDataplane)

	memifSocketPath := "/run/osvbng/memif.sock"
	if deps.Config != nil && deps.Config.Dataplane.MemifSocketPath != "" {
		memifSocketPath = deps.Config.Dataplane.MemifSocketPath
	}

	virtualMAC := ""
	if deps.Config != nil {
		virtualMAC = deps.Config.Redundancy.VirtualMAC
	}

	if virtualMAC == "" && deps.Config != nil {
		accessIface := deps.Config.Dataplane.AccessInterface
		swIfIndex, err := deps.VPP.GetInterfaceIndex(accessIface)
		if err == nil {
			mac, err := deps.VPP.GetInterfaceMAC(uint32(swIfIndex))
			if err == nil {
				virtualMAC = mac.String()
				log.Info("Using access interface MAC for ARP replies", "interface", accessIface, "mac", virtualMAC)
			}
		}
	}

	c := &Component{
		Base:       component.NewBase("dataplane"),
		logger:     log,
		eventBus:   deps.EventBus,
		ingress:    ingress,
		vpp:        deps.VPP,
		virtualMAC: virtualMAC,
		DHCPChan:   make(chan *dataplane.ParsedPacket, 1000),
		ARPChan:    make(chan *dataplane.ParsedPacket, 1000),
		PPPoEChan:  make(chan *dataplane.ParsedPacket, 1000),
	}

	memifHandler, err := vpp.NewMemifHandler(virtualMAC, c.handleARPRequest)
	if err != nil {
		return nil, fmt.Errorf("create memif handler: %w", err)
	}
	if err := memifHandler.Init(memifSocketPath); err != nil {
		return nil, fmt.Errorf("init memif handler: %w", err)
	}
	c.memifHandler = memifHandler

	return c, nil
}

func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)
	c.logger.Info("Starting dataplane component")

	if err := c.eventBus.Subscribe(events.TopicEgress, c.handleEgress); err != nil {
		return fmt.Errorf("subscribe to egress: %w", err)
	}

	if err := c.eventBus.Subscribe(events.TopicSessionLifecycle, c.handleSessionLifecycle); err != nil {
		return fmt.Errorf("subscribe to session lifecycle: %w", err)
	}

	c.Go(c.readLoop)

	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.logger.Info("Stopping dataplane component")

	if err := c.ingress.Close(); err != nil {
		c.logger.Error("Error closing ingress", "error", err)
	}

	if err := c.memifHandler.Close(); err != nil {
		c.logger.Error("Error closing memif handler", "error", err)
	}

	c.StopContext()

	return nil
}

func (c *Component) handleARPRequest(arp *vpp.ARPPacket) {
	if arp.Operation == 1 {
		if err := c.memifHandler.SendARPReply(arp); err != nil {
			c.logger.Error("Failed to send ARP reply", "error", err)
		}
	}
}

func (c *Component) readLoop() {
	c.logger.Info("Started packet read loop")

	for {
		select {
		case <-c.Ctx.Done():
			return
		default:
			pkt, err := c.ingress.ReadPacket(c.Ctx)
			if err != nil {
				select {
				case <-c.Ctx.Done():
					return
				default:
					c.logger.Error("Error reading packet", "error", err)
					continue
				}
			}

			if pkt == nil {
				continue
			}

			if err := c.handlePacket(pkt); err != nil {
				c.logger.Error("Error handling packet", "error", err)
			}
		}
	}
}

func (c *Component) handlePacket(pkt *dataplane.ParsedPacket) error {
	if pkt.Direction == dataplane.DirectionRX && pkt.OuterVLAN == 0 {
		return fmt.Errorf("packet rejected: S-VLAN required (untagged not supported)")
	}

	switch pkt.Protocol {
	case dataplane.ProtocolDHCP:
		if pkt.DHCPv4 != nil {
			c.logger.Info("Received DHCP packet",
				"operation", pkt.DHCPv4.Operation,
				"direction", pkt.Direction,
				"mac", pkt.MAC.String(),
				"svlan", pkt.OuterVLAN,
				"cvlan", pkt.InnerVLAN)
		}
		select {
		case c.DHCPChan <- pkt:
		default:
			c.logger.Warn("DHCP channel full, dropping packet")
		}

	case dataplane.ProtocolARP:
		if pkt.ARP != nil {
			c.logger.Info("Received ARP packet",
				"operation", pkt.ARP.Operation,
				"direction", pkt.Direction,
				"mac", pkt.MAC.String(),
				"svlan", pkt.OuterVLAN,
				"cvlan", pkt.InnerVLAN,
				"src_ip", net.IP(pkt.ARP.SourceProtAddress).String(),
				"dst_ip", net.IP(pkt.ARP.DstProtAddress).String())
		}
		select {
		case c.ARPChan <- pkt:
		default:
			c.logger.Warn("ARP channel full, dropping packet")
		}

	case dataplane.ProtocolPPP:
		select {
		case c.PPPoEChan <- pkt:
		default:
			c.logger.Warn("PPPoE channel full, dropping packet")
		}

	default:
		return fmt.Errorf("unknown protocol: %s", pkt.Protocol)
	}

	return nil
}

func (c *Component) handleEgress(event models.Event) error {
	var payload models.EgressPacketPayload
	if err := event.GetPayload(&payload); err != nil {
		return fmt.Errorf("failed to decode egress packet payload: %w", err)
	}

	dstMAC, err := net.ParseMAC(payload.DstMAC)
	if err != nil {
		return fmt.Errorf("invalid dst mac: %w", err)
	}

	srcMAC, err := net.ParseMAC(payload.SrcMAC)
	if err != nil {
		return fmt.Errorf("invalid src mac: %w", err)
	}

	etherType := uint16(0x0800)
	if event.Protocol == models.ProtocolARP {
		etherType = 0x0806
	}

	pkt := &dataplane.EgressPacket{
		DstMAC:    dstMAC,
		SrcMAC:    srcMAC,
		OuterVLAN: payload.OuterVLAN,
		InnerVLAN: payload.InnerVLAN,
		EtherType: etherType,
		Payload:   payload.RawData,
	}

	if err := c.memifHandler.SendPacket(pkt); err != nil {
		return fmt.Errorf("send packet: %w", err)
	}

	c.logger.Info("Sent egress packet", "dst_mac", dstMAC.String(), "svlan", payload.OuterVLAN, "cvlan", payload.InnerVLAN)

	return nil
}

func (c *Component) handleSessionLifecycle(event models.Event) error {
	var sess models.DHCPv4Session
	if err := event.GetPayload(&sess); err != nil {
		return fmt.Errorf("failed to decode session: %w", err)
	}

	if sess.State == models.SessionStateActive {
		return c.programFIB(&sess)
	} else if sess.State == models.SessionStateReleased {
		return c.removeFIB(&sess)
	}

	return nil
}

func (c *Component) programFIB(sess *models.DHCPv4Session) error {
	start := time.Now()

	ipStr := sess.IPv4Address.String()
	macStr := sess.MAC.String()
	swIfIndex := uint32(sess.IfIndex)
	svlan := sess.OuterVLAN
	cvlan := sess.InnerVLAN

	if ipStr == "" || macStr == "" || swIfIndex == 0 {
		return fmt.Errorf("missing required fields: ip=%s mac=%s sw_if_index=%d", ipStr, macStr, swIfIndex)
	}

	c.logger.Debug("Programming FIB for session",
		"ip", ipStr,
		"mac", macStr,
		"sw_if_index", swIfIndex,
		"svlan", svlan,
		"cvlan", cvlan)

	fibID, err := c.vpp.GetFIBIDForInterface(swIfIndex)
	if err != nil {
		c.logger.Warn("Failed to get FIB ID, using default", "error", err)
		fibID = 0
	}

	c.logger.Debug("Building L2 rewrite", "dst_mac", macStr, "src_mac", c.virtualMAC, "svlan", svlan, "cvlan", cvlan)

	rewrite := c.vpp.BuildL2Rewrite(macStr, c.virtualMAC, svlan, cvlan)
	if rewrite == nil {
		return fmt.Errorf("failed to build L2 rewrite")
	}
	c.logger.Debug("Built L2 rewrite", "len", len(rewrite))

	parentSwIfIndex, err := c.vpp.GetParentSwIfIndex()
	if err != nil {
		return fmt.Errorf("get parent sw_if_index: %w", err)
	}

	// Create adjacency with L2 rewrite on parent interface - we pre-program the FIB here to write the ethernet headers (this is slightly worrying if something goes wrong here, so we should probably have better error checking / validation on the fib_control plugin side to avoid crashing VPP itself)
	adjStart := time.Now()
	adjIndex, err := c.vpp.AddAdjacencyWithRewrite(ipStr, parentSwIfIndex, rewrite)
	adjDuration := time.Since(adjStart)
	if err != nil {
		return fmt.Errorf("add adjacency: %w", err)
	}

	c.logger.Warn("Added adjacency with rewrite",
		"ip", ipStr,
		"sw_if_index", parentSwIfIndex,
		"sub_if_index", swIfIndex,
		"adj_index", adjIndex,
		"svlan", svlan,
		"cvlan", cvlan,
		"duration_us", adjDuration.Microseconds())

	// Add /32 host route pointing to the adjacency, this is required so that FRR/routing daemon can sync routes into control plane (in the event that a customer wants to leak /32s)
	// We need to rework this whole function (and others tbh) to work properly with IPv6 /128s and also IPv6 delegated prefixes
	routeStart := time.Now()
	if err := c.vpp.AddHostRoute(ipStr, adjIndex, fibID, swIfIndex); err != nil {
		c.logger.Error("Failed to add host route, unlocking adjacency", "error", err)
		c.vpp.UnlockAdjacency(adjIndex)
		return fmt.Errorf("add host route: %w", err)
	}
	routeDuration := time.Since(routeStart)

	totalDuration := time.Since(start)
	c.logger.Warn("FIB programming complete",
		"ip", ipStr,
		"adj_index", adjIndex,
		"fib_id", fibID,
		"adj_duration_us", adjDuration.Microseconds(),
		"route_duration_us", routeDuration.Microseconds(),
		"total_duration_us", totalDuration.Microseconds())

	return nil
}

func (c *Component) removeFIB(sess *models.DHCPv4Session) error {
	ipStr := sess.IPv4Address.String()
	swIfIndex := uint32(sess.IfIndex)

	if ipStr == "" {
		return fmt.Errorf("missing ip address")
	}

	c.logger.Info("Removing FIB for session", "ip", ipStr, "sw_if_index", swIfIndex)
	fibID, err := c.vpp.GetFIBIDForInterface(swIfIndex)
	if err != nil {
		c.logger.Warn("Failed to get FIB ID, using default", "error", err)
		fibID = 0
	}

	// Idea: we should probably build some kind of watchdog based daemon to scan the subscriber database and compare against FIB to clean up adjacencies that have not been properly cleaned up
	if err := c.vpp.DeleteHostRoute(ipStr, fibID); err != nil {
		return fmt.Errorf("delete host route: %w", err)
	}

	c.logger.Info("Removed host route", "ip", ipStr, "fib_id", fibID)

	return nil
}
