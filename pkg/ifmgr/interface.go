package ifmgr

import "net"

type IfType uint32

const (
	IfTypeHardware IfType = 0
	IfTypeSub      IfType = 1
	IfTypeP2P      IfType = 2
	IfTypePipe     IfType = 3
)

type Interface struct {
	SwIfIndex       uint32
	SupSwIfIndex    uint32
	Name            string
	DevType         string
	Type            IfType
	AdminUp         bool
	LinkUp          bool
	MTU             uint32
	LinkSpeed       uint32
	MAC             []byte
	SubID           uint32
	SubNumberOfTags uint8
	OuterVlanID     uint16
	InnerVlanID     uint16
	Tag             string
	IPv4Addresses   []net.IP
	IPv6Addresses   []net.IP
	FIBTableID      uint32
}

func (i *Interface) IsSubinterface() bool {
	return i.Type == IfTypeSub
}

func (i *Interface) HasParent() bool {
	return i.SupSwIfIndex != i.SwIfIndex
}
