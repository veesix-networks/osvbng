package vpp

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

type PuntSocketIngress struct {
	socketPath string
	conn       net.Conn
	ctx        context.Context
	cancel     context.CancelFunc
	logger     *slog.Logger
	readBuf    []byte
}

func NewPuntSocketIngress(socketPath string) *PuntSocketIngress {
	return &PuntSocketIngress{
		socketPath: socketPath,
		logger:     logger.Component(logger.ComponentDataplaneIO),
	}
}

func (p *PuntSocketIngress) Init(_ string) error {
	p.logger.Info("Creating punt socket", "path", p.socketPath)

	os.Remove(p.socketPath)

	addr, err := net.ResolveUnixAddr("unixgram", p.socketPath)
	if err != nil {
		return fmt.Errorf("resolve unix addr: %w", err)
	}

	conn, err := net.ListenUnixgram("unixgram", addr)
	if err != nil {
		return fmt.Errorf("create punt socket: %w", err)
	}

	if err := conn.SetReadBuffer(65536); err != nil {
		p.logger.Warn("Failed to set socket read buffer", "error", err)
	}

	p.conn = conn

	p.ctx, p.cancel = context.WithCancel(context.Background())

	p.readBuf = make([]byte, 65535)

	p.logger.Info("Punt socket created, waiting for packets from VPP", "path", p.socketPath)

	return nil
}

func (p *PuntSocketIngress) ReadPacket(ctx context.Context) (*dataplane.ParsedPacket, error) {
	if p.conn == nil {
		return nil, fmt.Errorf("punt socket not connected")
	}

	if unixConn, ok := p.conn.(*net.UnixConn); ok {
		unixConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	}

	n, err := p.conn.Read(p.readBuf)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return nil, nil
		}
		return nil, fmt.Errorf("read packet: %w", err)
	}

	buf := make([]byte, n)
	copy(buf, p.readBuf[:n])

	if n < 8 {
		return nil, fmt.Errorf("packet too short: %d bytes", n)
	}

	swIfIndex := uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24
	action := uint32(buf[4]) | uint32(buf[5])<<8 | uint32(buf[6])<<16 | uint32(buf[7])<<24

	p.logger.Info("Received punted packet from VPP", "sw_if_index", swIfIndex, "action", action, "size", n-8)

	puntData := buf[8:]

	packet := gopacket.NewPacket(puntData, layers.LayerTypeEthernet, gopacket.Default)

	ethLayer := packet.Layer(layers.LayerTypeEthernet)
	if ethLayer == nil {
		return nil, fmt.Errorf("no ethernet layer")
	}
	eth := ethLayer.(*layers.Ethernet)

	parsedPkt := &dataplane.ParsedPacket{
		SwIfIndex: swIfIndex,
		Ethernet:  eth,
		MAC:       eth.SrcMAC,
		RawPacket: puntData,
		Metadata:  make(map[string]interface{}),
	}

	var vlanLayers []*layers.Dot1Q
	for _, layer := range packet.Layers() {
		if dot1q, ok := layer.(*layers.Dot1Q); ok {
			vlanLayers = append(vlanLayers, dot1q)
		}
	}

	if len(vlanLayers) > 0 {
		parsedPkt.OuterVLAN = vlanLayers[0].VLANIdentifier
		parsedPkt.VLANCount = 1
		if len(vlanLayers) > 1 {
			parsedPkt.InnerVLAN = vlanLayers[1].VLANIdentifier
			parsedPkt.VLANCount = 2
		}
	}
	parsedPkt.Dot1Q = vlanLayers

	if ipv4Layer := packet.Layer(layers.LayerTypeIPv4); ipv4Layer != nil {
		parsedPkt.IPv4 = ipv4Layer.(*layers.IPv4)
	}

	if ipv6Layer := packet.Layer(layers.LayerTypeIPv6); ipv6Layer != nil {
		parsedPkt.IPv6 = ipv6Layer.(*layers.IPv6)
	}

	if udpLayer := packet.Layer(layers.LayerTypeUDP); udpLayer != nil {
		parsedPkt.UDP = udpLayer.(*layers.UDP)

		if parsedPkt.UDP.DstPort == 67 || parsedPkt.UDP.DstPort == 68 {
			parsedPkt.Protocol = dataplane.ProtocolDHCP
			if dhcpv4Layer := packet.Layer(layers.LayerTypeDHCPv4); dhcpv4Layer != nil {
				parsedPkt.DHCPv4 = dhcpv4Layer.(*layers.DHCPv4)
				if parsedPkt.DHCPv4.Operation == layers.DHCPOpReply {
					parsedPkt.Direction = dataplane.DirectionTX
				} else {
					parsedPkt.Direction = dataplane.DirectionRX
				}
			}
		}
	}

	if arpLayer := packet.Layer(layers.LayerTypeARP); arpLayer != nil {
		parsedPkt.ARP = arpLayer.(*layers.ARP)
		parsedPkt.Protocol = dataplane.ProtocolARP
		parsedPkt.Direction = dataplane.DirectionRX
	}

	if parsedPkt.Protocol == "" {
		return nil, fmt.Errorf("unknown protocol")
	}

	p.logger.Info("Received packet", "protocol", parsedPkt.Protocol, "mac", eth.SrcMAC.String(), "svlan", parsedPkt.OuterVLAN, "cvlan", parsedPkt.InnerVLAN)

	return parsedPkt, nil
}

func (p *PuntSocketIngress) Close() error {
	p.logger.Info("Closing punt socket")

	if p.cancel != nil {
		p.cancel()
	}

	if p.conn != nil {
		p.conn.Close()
	}

	return nil
}
