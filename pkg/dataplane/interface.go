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
	ProtocolDHCP = "dhcp"
	ProtocolARP  = "arp"
	ProtocolPPP  = "ppp"
	ProtocolL2TP = "l2tp"

	DirectionRX = "rx"
	DirectionTX = "tx"
)
