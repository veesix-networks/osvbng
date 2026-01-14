package dataplane

import (
	"net"

	"github.com/google/gopacket/layers"
)

type ParsedPacket struct {
	Direction string
	Protocol  string

	MAC       net.HardwareAddr
	OuterVLAN uint16
	InnerVLAN uint16
	VLANCount int
	SwIfIndex uint32

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
	Metadata  map[string]interface{}
}
