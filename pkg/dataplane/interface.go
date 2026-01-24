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

type EgressPacket struct {
	DstMAC    net.HardwareAddr
	SrcMAC    net.HardwareAddr
	OuterVLAN uint16
	InnerVLAN uint16
	EtherType uint16
	Payload   []byte
}
