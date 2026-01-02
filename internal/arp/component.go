package arp

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/srg"
)

type puntHeader struct {
	SwIfIndex uint32
	Protocol  uint16
	DataLen   uint16
}

type Component struct {
	*component.Base

	logger   *slog.Logger
	eventBus events.Bus
	srgMgr   *srg.Manager
	vpp      interface {
		GetInterfaceMAC(uint32) (net.HardwareAddr, error)
		GetParentSwIfIndex() (uint32, error)
	}
	socketPath string
	socketConn *net.UnixConn
}

func New(deps component.Dependencies, srgMgr *srg.Manager) (component.Component, error) {
	socketPath := "/run/vpp/arp-punt.sock"
	if deps.Config != nil && deps.Config.Dataplane.ARPPuntSocketPath != "" {
		socketPath = deps.Config.Dataplane.ARPPuntSocketPath
	}

	log := logger.Component(logger.ComponentARP)

	return &Component{
		Base:       component.NewBase("arp"),
		logger:     log,
		eventBus:   deps.EventBus,
		srgMgr:     srgMgr,
		vpp:        deps.VPP,
		socketPath: socketPath,
	}, nil
}

func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)
	c.logger.Info("Starting ARP component", "socket", c.socketPath)

	if err := os.Remove(c.socketPath); err != nil && !os.IsNotExist(err) {
		c.logger.Warn("Failed to remove existing socket", "error", err)
	}

	addr, err := net.ResolveUnixAddr("unixgram", c.socketPath)
	if err != nil {
		return fmt.Errorf("resolve unix addr: %w", err)
	}

	conn, err := net.ListenUnixgram("unixgram", addr)
	if err != nil {
		return fmt.Errorf("listen on unix socket: %w", err)
	}

	if err := os.Chmod(c.socketPath, 0666); err != nil {
		conn.Close()
		return fmt.Errorf("chmod socket: %w", err)
	}

	c.socketConn = conn

	c.Go(c.readLoop)

	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.logger.Info("Stopping ARP component")

	if c.socketConn != nil {
		c.socketConn.Close()
	}

	c.StopContext()

	os.Remove(c.socketPath)

	return nil
}

func (c *Component) readLoop() {
	buf := make([]byte, 8+1500)

	for {
		select {
		case <-c.Ctx.Done():
			return
		default:
		}

		c.socketConn.SetReadDeadline(time.Now().Add(1 * time.Second))

		n, err := c.socketConn.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			select {
			case <-c.Ctx.Done():
				return
			default:
				c.logger.Warn("Error reading from socket", "error", err)
				continue
			}
		}

		if n < 8 {
			c.logger.Debug("Packet too small", "size", n)
			continue
		}

		hdr := puntHeader{
			SwIfIndex: binary.BigEndian.Uint32(buf[0:4]),
			Protocol:  binary.BigEndian.Uint16(buf[4:6]),
			DataLen:   binary.BigEndian.Uint16(buf[6:8]),
		}

		if hdr.Protocol != 0x0806 {
			c.logger.Debug("Non-ARP packet", "protocol", hdr.Protocol)
			continue
		}

		if n < int(8+hdr.DataLen) {
			c.logger.Debug("Incomplete packet", "expected", 8+hdr.DataLen, "got", n)
			continue
		}

		pktData := make([]byte, hdr.DataLen)
		copy(pktData, buf[8:8+hdr.DataLen])

		if err := c.handlePacket(hdr.SwIfIndex, pktData); err != nil {
			c.logger.Debug("Error handling packet", "error", err, "sw_if_index", hdr.SwIfIndex)
		}
	}
}

func (c *Component) handlePacket(swIfIndex uint32, pkt []byte) error {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		c.logger.Warn("ARP packet processing time",
			"duration_us", duration.Microseconds(),
			"sw_if_index", swIfIndex)
	}()

	packet := gopacket.NewPacket(pkt, layers.LayerTypeARP, gopacket.Default)

	arpLayer := packet.Layer(layers.LayerTypeARP)
	if arpLayer == nil {
		return fmt.Errorf("no ARP layer")
	}

	arp := arpLayer.(*layers.ARP)

	if arp.Operation != 1 {
		return nil
	}

	srcMAC := net.HardwareAddr(arp.SourceHwAddress)
	srcIP := net.IP(arp.SourceProtAddress)
	dstIP := net.IP(arp.DstProtAddress)

	c.logger.Debug("ARP request",
		"sw_if_index", swIfIndex,
		"src_mac", srcMAC.String(),
		"src_ip", srcIP.String(),
		"dst_ip", dstIP.String(),
	)

	var gatewayMAC net.HardwareAddr
	if c.srgMgr != nil {
		gatewayMAC = c.srgMgr.GetVirtualMAC(0)
	}
	if gatewayMAC == nil && c.vpp != nil {
		parentIfIdx, err := c.vpp.GetParentSwIfIndex()
		if err == nil {
			if ifMac, err := c.vpp.GetInterfaceMAC(parentIfIdx); err == nil {
				gatewayMAC = ifMac
			} else {
				c.logger.Warn("Failed to get parent interface MAC for ARP", "error", err)
			}
		} else {
			c.logger.Warn("Failed to get parent interface index for ARP", "error", err)
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
		OuterVLAN: 0,
		InnerVLAN: 0,
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
