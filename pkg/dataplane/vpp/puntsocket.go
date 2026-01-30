package vpp

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
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
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

	if err := conn.SetReadBuffer(32 * 1024 * 1024); err != nil {
		p.logger.Warn("Failed to set socket read buffer", "error", err)
	}
	if err := conn.SetWriteBuffer(32 * 1024 * 1024); err != nil {
		p.logger.Warn("Failed to set socket write buffer", "error", err)
	}

	p.conn = conn

	p.ctx, p.cancel = context.WithCancel(context.Background())

	p.readBuf = make([]byte, 65535)

	p.logger.Info("Punt socket created, waiting for packets from VPP", "path", p.socketPath)

	return nil
}

func (p *PuntSocketIngress) ReadPacket(ctx context.Context) (*dataplane.ParsedPacket, error) {
	if p.conn == nil {
		p.logger.Error("ReadPacket called but connection is nil!")
		return nil, fmt.Errorf("punt socket not connected")
	}

	if unixConn, ok := p.conn.(*net.UnixConn); ok {
		unixConn.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
	}

	n, err := p.conn.Read(p.readBuf)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return nil, nil
		}
		p.logger.Error("Failed to read from punt socket", "error", err)
		return nil, fmt.Errorf("read packet: %w", err)
	}

	p.logger.Debug("Read data from punt socket", "bytes", n)

	buf := make([]byte, n)
	copy(buf, p.readBuf[:n])

	if n < 16 {
		return nil, fmt.Errorf("packet too short: %d bytes", n)
	}

	swIfIndex := binary.BigEndian.Uint32(buf[0:4])
	protocol := buf[4]
	direction := buf[5]
	dataLen := binary.BigEndian.Uint16(buf[6:8])
	timestampNs := binary.BigEndian.Uint64(buf[8:16])

	puntData := buf[16:]

	p.logger.Debug("Received punted packet from VPP", "sw_if_index", swIfIndex, "protocol", protocol, "direction", direction, "data_len", dataLen, "timestamp_ns", timestampNs, "size", n-16)

	packet := gopacket.NewPacket(puntData, layers.LayerTypeEthernet, gopacket.Default)

	ethLayer := packet.Layer(layers.LayerTypeEthernet)
	if ethLayer == nil {
		return nil, fmt.Errorf("no ethernet layer")
	}
	eth := ethLayer.(*layers.Ethernet)

	var vlanLayers []*layers.Dot1Q
	for _, layer := range packet.Layers() {
		if dot1q, ok := layer.(*layers.Dot1Q); ok {
			vlanLayers = append(vlanLayers, dot1q)
		}
	}

	outerVLAN := uint16(0)
	innerVLAN := uint16(0)
	if len(vlanLayers) > 0 {
		outerVLAN = vlanLayers[0].VLANIdentifier
		if len(vlanLayers) > 1 {
			innerVLAN = vlanLayers[1].VLANIdentifier
		}
	}

	parsedPkt := &dataplane.ParsedPacket{
		SwIfIndex: swIfIndex,
		Ethernet:  eth,
		MAC:       eth.SrcMAC,
		OuterVLAN: outerVLAN,
		InnerVLAN: innerVLAN,
		RawPacket: puntData,
		Dot1Q:     vlanLayers,
	}

	switch protocol {
	case 0:
		parsedPkt.Protocol = models.ProtocolDHCPv4
		if ipv4Layer := packet.Layer(layers.LayerTypeIPv4); ipv4Layer != nil {
			parsedPkt.IPv4 = ipv4Layer.(*layers.IPv4)
		}
		if udpLayer := packet.Layer(layers.LayerTypeUDP); udpLayer != nil {
			parsedPkt.UDP = udpLayer.(*layers.UDP)
		}
		if dhcpv4Layer := packet.Layer(layers.LayerTypeDHCPv4); dhcpv4Layer != nil {
			parsedPkt.DHCPv4 = dhcpv4Layer.(*layers.DHCPv4)
		}
	case 1:
		parsedPkt.Protocol = models.ProtocolDHCPv6
		if ipv6Layer := packet.Layer(layers.LayerTypeIPv6); ipv6Layer != nil {
			parsedPkt.IPv6 = ipv6Layer.(*layers.IPv6)
		}
		if udpLayer := packet.Layer(layers.LayerTypeUDP); udpLayer != nil {
			parsedPkt.UDP = udpLayer.(*layers.UDP)
		}
		if dhcpv6Layer := packet.Layer(layers.LayerTypeDHCPv6); dhcpv6Layer != nil {
			parsedPkt.DHCPv6 = dhcpv6Layer.(*layers.DHCPv6)
		}
	case 2:
		parsedPkt.Protocol = models.ProtocolARP
		if arpLayer := packet.Layer(layers.LayerTypeARP); arpLayer != nil {
			parsedPkt.ARP = arpLayer.(*layers.ARP)
		}
	case 3:
		parsedPkt.Protocol = models.ProtocolPPPoEDiscovery
		if pppoeLayer := packet.Layer(layers.LayerTypePPPoE); pppoeLayer != nil {
			parsedPkt.PPPoE = pppoeLayer.(*layers.PPPoE)
		}
	case 4:
		parsedPkt.Protocol = models.ProtocolPPPoESession
		if pppoeLayer := packet.Layer(layers.LayerTypePPPoE); pppoeLayer != nil {
			parsedPkt.PPPoE = pppoeLayer.(*layers.PPPoE)
		}
		if pppLayer := packet.Layer(layers.LayerTypePPP); pppLayer != nil {
			parsedPkt.PPP = pppLayer.(*layers.PPP)
		}
	case 5:
		parsedPkt.Protocol = models.ProtocolIPv6ND
	case 6:
		parsedPkt.Protocol = models.ProtocolL2TP
	default:
		return nil, fmt.Errorf("unsupported protocol: %d", protocol)
	}

	p.logger.Debug("Received packet", "protocol", parsedPkt.Protocol, "mac", eth.SrcMAC.String(), "svlan", parsedPkt.OuterVLAN, "cvlan", parsedPkt.InnerVLAN)

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
