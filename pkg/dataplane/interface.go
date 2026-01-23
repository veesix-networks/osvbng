package dataplane

import (
	"context"
	"net"
)

type Ingress interface {
	Init(interfaceName string) error
	ReadPacket(ctx context.Context) (*ParsedPacket, error)
	Close() error
}

type Egress interface {
	Init(interfaceName string) error
	SendPacket(pkt *EgressPacket) error
	Close() error
}

type ControlPacket struct {
	Protocol  string
	Direction string
	MAC       net.HardwareAddr
	OuterVLAN uint16
	InnerVLAN uint16
	VLANCount int
	SwIfIndex uint32
	RawData   []byte
	Metadata  map[string]interface{}
}

type EgressPacket struct {
	DstMAC    net.HardwareAddr
	SrcMAC    net.HardwareAddr
	OuterVLAN uint16
	InnerVLAN uint16
	EtherType uint16
	Payload   []byte
}

const (
	ProtocolDHCPv4         = "dhcpv4"
	ProtocolDHCPv6         = "dhcpv6"
	ProtocolARP            = "arp"
	ProtocolPPPoEDiscovery = "pppoe_discovery"
	ProtocolPPPoESession   = "pppoe_session"
	ProtocolIPv6ND         = "ipv6_nd"
	ProtocolL2TP           = "l2tp"
	ProtocolDHCP           = "dhcp"
	ProtocolPPP            = "ppp"
)
