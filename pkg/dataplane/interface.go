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
	SendPacketBatch(pkts []*EgressPacket) (int, error)
	Close() error
}

type EgressPacket struct {
	SwIfIndex uint32
	DstMAC    net.HardwareAddr
	SrcMAC    net.HardwareAddr
	OuterVLAN uint16
	InnerVLAN uint16
	EtherType uint16
	Payload   []byte
}
