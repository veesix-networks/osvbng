package dataplane

import (
	"net"

	"github.com/google/gopacket/layers"
	"github.com/veesix-networks/osvbng/pkg/models"
)

type ParsedPacket struct {
	Protocol models.Protocol

	MAC           net.HardwareAddr
	OuterVLAN     uint16
	InnerVLAN     uint16
	SwIfIndex     uint32
	InterfaceName string

	Ethernet *layers.Ethernet
	Dot1Q    []*layers.Dot1Q
	IPv4     *layers.IPv4
	IPv6     *layers.IPv6
	UDP      *layers.UDP
	TCP      *layers.TCP
	DHCPv4   *layers.DHCPv4
	DHCPv6   *layers.DHCPv6
	ARP      *layers.ARP
	PPPoE    *layers.PPPoE
	PPP      *layers.PPP

	RawPacket []byte
}
