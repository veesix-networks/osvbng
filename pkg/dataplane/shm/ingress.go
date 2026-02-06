package shm

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
)

type Ingress struct {
	client *Client
	reader *PuntReader
	logger *slog.Logger
}

func NewIngress() *Ingress {
	return &Ingress{
		client: NewClient(),
		logger: logger.Get(logger.Dataplane),
	}
}

func (i *Ingress) Init(_ string) error {
	i.logger.Info("Connecting to VPP shared memory", "path", ShmPath)

	// Retry connection - VPP plugin may not have initialized shm yet
	maxRetries := 30
	retryInterval := time.Second

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := i.client.Connect(); err != nil {
			lastErr = err
			i.logger.Warn("Failed to connect to shm, retrying...",
				"attempt", attempt,
				"max_retries", maxRetries,
				"error", err,
			)
			time.Sleep(retryInterval)
			continue
		}

		i.reader = NewPuntReader(i.client)

		i.logger.Info("Connected to VPP shared memory",
			"punt_ring_size", i.client.header.PuntRingSize,
			"egress_ring_size", i.client.header.EgressRingSize,
			"slot_size", i.client.header.SlotSize,
		)

		return nil
	}

	return fmt.Errorf("connect shm after %d retries: %w", maxRetries, lastErr)
}

func (i *Ingress) ReadPacket(ctx context.Context) (*dataplane.ParsedPacket, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		pkt, ok := i.reader.Read()
		if !ok {
			i.reader.Commit()
			if err := i.reader.Wait(); err != nil {
				return nil, nil
			}
			continue
		}

		i.reader.Commit()

		parsed, err := i.parsePacket(pkt)
		if err != nil {
			i.logger.Debug("Failed to parse packet", "error", err)
			continue
		}

		return parsed, nil
	}
}

func (i *Ingress) parsePacket(pkt *PuntPacket) (*dataplane.ParsedPacket, error) {
	packet := gopacket.NewPacket(pkt.Data, layers.LayerTypeEthernet, gopacket.Default)

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

	outerVLAN := pkt.OuterVLAN
	innerVLAN := pkt.InnerVLAN
	if outerVLAN == 0 && len(vlanLayers) > 0 {
		outerVLAN = vlanLayers[0].VLANIdentifier
		if len(vlanLayers) > 1 {
			innerVLAN = vlanLayers[1].VLANIdentifier
		}
	}

	parsed := &dataplane.ParsedPacket{
		SwIfIndex: pkt.SwIfIndex,
		Ethernet:  eth,
		MAC:       eth.SrcMAC,
		OuterVLAN: outerVLAN,
		InnerVLAN: innerVLAN,
		RawPacket: pkt.Data,
		Dot1Q:     vlanLayers,
	}

	switch pkt.Protocol {
	case ProtoDHCPv4:
		parsed.Protocol = models.ProtocolDHCPv4
		if ipv4Layer := packet.Layer(layers.LayerTypeIPv4); ipv4Layer != nil {
			parsed.IPv4 = ipv4Layer.(*layers.IPv4)
		}
		if udpLayer := packet.Layer(layers.LayerTypeUDP); udpLayer != nil {
			parsed.UDP = udpLayer.(*layers.UDP)
		}
		if dhcpv4Layer := packet.Layer(layers.LayerTypeDHCPv4); dhcpv4Layer != nil {
			parsed.DHCPv4 = dhcpv4Layer.(*layers.DHCPv4)
		}
	case ProtoDHCPv6:
		parsed.Protocol = models.ProtocolDHCPv6
		if ipv6Layer := packet.Layer(layers.LayerTypeIPv6); ipv6Layer != nil {
			parsed.IPv6 = ipv6Layer.(*layers.IPv6)
		}
		if udpLayer := packet.Layer(layers.LayerTypeUDP); udpLayer != nil {
			parsed.UDP = udpLayer.(*layers.UDP)
		}
		if dhcpv6Layer := packet.Layer(layers.LayerTypeDHCPv6); dhcpv6Layer != nil {
			parsed.DHCPv6 = dhcpv6Layer.(*layers.DHCPv6)
		}
	case ProtoARP:
		parsed.Protocol = models.ProtocolARP
		if arpLayer := packet.Layer(layers.LayerTypeARP); arpLayer != nil {
			parsed.ARP = arpLayer.(*layers.ARP)
		}
	case ProtoPPPoEDisc:
		parsed.Protocol = models.ProtocolPPPoEDiscovery
		if pppoeLayer := packet.Layer(layers.LayerTypePPPoE); pppoeLayer != nil {
			parsed.PPPoE = pppoeLayer.(*layers.PPPoE)
		}
	case ProtoPPPoESess:
		parsed.Protocol = models.ProtocolPPPoESession
		if pppoeLayer := packet.Layer(layers.LayerTypePPPoE); pppoeLayer != nil {
			parsed.PPPoE = pppoeLayer.(*layers.PPPoE)
		}
		if pppLayer := packet.Layer(layers.LayerTypePPP); pppLayer != nil {
			parsed.PPP = pppLayer.(*layers.PPP)
		}
	case ProtoIPv6ND:
		parsed.Protocol = models.ProtocolIPv6ND
	case ProtoL2TP:
		parsed.Protocol = models.ProtocolL2TP
	default:
		return nil, fmt.Errorf("unsupported protocol: %d", pkt.Protocol)
	}

	i.logger.Debug("Received packet via shm",
		"protocol", parsed.Protocol,
		"mac", eth.SrcMAC.String(),
		"svlan", parsed.OuterVLAN,
		"cvlan", parsed.InnerVLAN,
	)

	return parsed, nil
}

func (i *Ingress) Client() *Client {
	return i.client
}

func (i *Ingress) Close() error {
	i.logger.Info("Closing shm ingress")
	return i.client.Close()
}
