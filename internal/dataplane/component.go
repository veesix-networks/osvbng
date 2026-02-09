package dataplane

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/cppm"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/dataplane/shm"
	"github.com/veesix-networks/osvbng/pkg/ethernet"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

type Component struct {
	*component.Base

	logger   *slog.Logger
	eventBus events.Bus
	ingress  *shm.Ingress
	egress   *shm.Egress
	vpp      *southbound.VPP

	DHCPChan    chan *dataplane.ParsedPacket
	DHCPv6Chan  chan *dataplane.ParsedPacket
	ARPChan     chan *dataplane.ParsedPacket
	PPPoEChan   chan *dataplane.ParsedPacket
	IPv6NDChan  chan *dataplane.ParsedPacket

	CPPM *cppm.Manager

	egressCount  atomic.Int64
	egressErrors atomic.Int64
	readLoopWg   sync.WaitGroup
}

func New(deps component.Dependencies) (component.Component, error) {
	log := logger.Get(logger.Dataplane)

	c := &Component{
		Base:       component.NewBase("dataplane"),
		logger:     log,
		eventBus:   deps.EventBus,
		vpp:        deps.VPP,
		DHCPChan:    make(chan *dataplane.ParsedPacket, 1000),
		DHCPv6Chan:  make(chan *dataplane.ParsedPacket, 1000),
		ARPChan:     make(chan *dataplane.ParsedPacket, 1000),
		PPPoEChan:   make(chan *dataplane.ParsedPacket, 1000),
		IPv6NDChan:  make(chan *dataplane.ParsedPacket, 1000),
		CPPM:       deps.CPPM,
	}

	ingress := shm.NewIngress()
	if err := ingress.Init(""); err != nil {
		return nil, fmt.Errorf("init shm ingress: %w", err)
	}
	c.ingress = ingress

	c.egress = shm.NewEgress(ingress.Client())

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

	c.readLoopWg.Add(2)
	go c.readLoop()
	go c.egressStatsLoop()

	return nil
}

func (c *Component) egressStatsLoop() {
	defer c.readLoopWg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var lastCount, lastErrors int64
	for {
		select {
		case <-c.Ctx.Done():
			return
		case <-ticker.C:
			count := c.egressCount.Load()
			errors := c.egressErrors.Load()
			if count != lastCount || errors != lastErrors {
				c.logger.Debug("Egress stats", "total_sent", count, "total_errors", errors, "sent_per_sec", count-lastCount, "errors_per_sec", errors-lastErrors)
				lastCount = count
				lastErrors = errors
			}
		}
	}
}

func (c *Component) readLoop() {
	defer c.readLoopWg.Done()
	c.logger.Info("Starting dataplane readLoop")
	ctx := c.Ctx
	pktCount := 0
	lastLogTime := time.Now()
	for {
		select {
		case <-ctx.Done():
			c.logger.Info("Stopping dataplane readLoop")
			return
		default:
			pkt, err := c.ingress.ReadPacket(ctx)
			if err != nil {
				c.logger.Error("Failed to read packet", "error", err)
				continue
			}

			if pkt == nil {
				continue
			}

			pktCount++
			now := time.Now()
			if now.Sub(lastLogTime) >= time.Second {
				c.logger.Debug("Ingress throughput", "packets_per_sec", pktCount, "dhcp_chan_len", len(c.DHCPChan), "ppp_chan_len", len(c.PPPoEChan))
				pktCount = 0
				lastLogTime = now
			}

			switch pkt.Protocol {
			case models.ProtocolDHCPv4:
				if !c.CPPM.Allow(cppm.ProtocolDHCPv4) {
					continue
				}
				select {
				case c.DHCPChan <- pkt:
				default:
					c.logger.Warn("DHCP channel full, dropping packet")
				}
			case models.ProtocolARP:
				if !c.CPPM.Allow(cppm.ProtocolARP) {
					continue
				}
				select {
				case c.ARPChan <- pkt:
				default:
					c.logger.Warn("ARP channel full, dropping packet")
				}
			case models.ProtocolDHCPv6:
				if !c.CPPM.Allow(cppm.ProtocolDHCPv6) {
					continue
				}
				select {
				case c.DHCPv6Chan <- pkt:
				default:
					c.logger.Warn("DHCPv6 channel full, dropping packet")
				}
			case models.ProtocolPPPoEDiscovery, models.ProtocolPPPoESession:
				if !c.CPPM.Allow(cppm.ProtocolPPPoE) {
					continue
				}
				select {
				case c.PPPoEChan <- pkt:
				default:
					c.logger.Warn("PPPoE channel full, dropping packet")
				}
			case models.ProtocolIPv6ND:
				if !c.CPPM.Allow(cppm.ProtocolIPv6ND) {
					continue
				}
				select {
				case c.IPv6NDChan <- pkt:
				default:
					c.logger.Warn("IPv6 ND channel full, dropping packet")
				}
			case models.ProtocolL2TP:
				if !c.CPPM.Allow(cppm.ProtocolL2TP) {
					continue
				}
				c.logger.Debug("Received L2TP packet", "sw_if_index", pkt.SwIfIndex, "mac", pkt.MAC.String(), "svlan", pkt.OuterVLAN, "cvlan", pkt.InnerVLAN)
			default:
				c.logger.Warn("Unknown protocol", "protocol", pkt.Protocol)
			}
		}
	}
}

func (c *Component) Stop(ctx context.Context) error {
	c.logger.Info("Stopping dataplane component")

	c.StopContext()
	c.readLoopWg.Wait()

	if err := c.egress.Close(); err != nil {
		c.logger.Error("Error closing egress", "error", err)
	}

	if err := c.ingress.Close(); err != nil {
		c.logger.Error("Error closing ingress", "error", err)
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

	etherType := ethernet.EtherTypeIPv4
	switch event.Protocol {
	case models.ProtocolARP:
		etherType = ethernet.EtherTypeARP
	case models.ProtocolPPPoEDiscovery:
		etherType = ethernet.EtherTypePPPoEDiscovery
	case models.ProtocolPPPoESession:
		etherType = ethernet.EtherTypePPPoESession
	case models.ProtocolDHCPv6, models.ProtocolIPv6ND:
		etherType = ethernet.EtherTypeIPv6
	}

	swIfIndex := payload.SwIfIndex
	if payload.OuterVLAN > 0 && c.vpp != nil {
		if iface := c.vpp.GetIfMgr().Get(payload.SwIfIndex); iface != nil && iface.HasParent() {
			swIfIndex = iface.SupSwIfIndex
		}
	}

	pkt := &dataplane.EgressPacket{
		SwIfIndex: swIfIndex,
		DstMAC:    dstMAC,
		SrcMAC:    srcMAC,
		OuterVLAN: payload.OuterVLAN,
		InnerVLAN: payload.InnerVLAN,
		EtherType: etherType,
		Payload:   payload.RawData,
	}

	if err := c.egress.SendPacket(pkt); err != nil {
		c.egressErrors.Add(1)
		return fmt.Errorf("send packet: %w", err)
	}
	c.egressCount.Add(1)

	c.logger.Debug("Sent egress packet", "sw_if_index", swIfIndex, "payload_sw_if_index", payload.SwIfIndex, "dst_mac", dstMAC.String(), "src_mac", srcMAC.String(), "svlan", payload.OuterVLAN, "cvlan", payload.InnerVLAN)

	return nil
}

func (c *Component) handleSessionLifecycle(event models.Event) error {
	// session lifecycle fib programming is now moved to align with respective
	// access technology profile, keeping this here for now until we've fully
	// unified the dhcpv4, dhcpv6 and pppoe processes
	return nil
}

