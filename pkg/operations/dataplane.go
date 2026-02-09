package operations

import (
	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
)

type Dataplane interface {
	CreateInterface(cfg *interfaces.InterfaceConfig) error
	DeleteInterface(name string) error
	SetInterfaceDescription(name, description string) error
	SetInterfaceMTU(name string, mtu int) error
	SetInterfaceEnabled(name string, enabled bool) error
	AddIPv4Address(ifName, address string) error
	DelIPv4Address(ifName, address string) error
	AddIPv6Address(ifName, address string) error
	DelIPv6Address(ifName, address string) error
}

const (
	PuntProtoDHCPv4    uint8 = 0
	PuntProtoDHCPv6    uint8 = 1
	PuntProtoARP       uint8 = 2
	PuntProtoPPPoEDisc uint8 = 3
	PuntProtoPPPoESess uint8 = 4
	PuntProtoIPv6ND    uint8 = 5
	PuntProtoL2TP      uint8 = 6
)

type InterfaceState struct {
	SwIfIndex     uint32
	Name          string
	AdminUp       bool
	LinkUp        bool
	MTU           uint32
	IPv4Addresses []string
	IPv6Addresses []string
	OuterVlanID   uint16
	InnerVlanID   uint16
	SupSwIfIndex  uint32
}

type IPv6RAState struct {
	SwIfIndex          uint32
	Managed            bool
	Other              bool
	RouterLifetimeSecs uint16
	MaxIntervalSecs    float64
	MinIntervalSecs    float64
	SendRadv           bool
}

type DataplaneStateReader interface {
	IsInterfaceConfigured(name string) bool
	GetInterfaceByName(name string) *InterfaceState
	IsInterfaceEnabled(name string) bool
	GetInterfaceMTU(name string) uint32
	HasIPv4Address(name string, addr string) bool
	HasIPv6Address(name string, addr string) bool
	IsUnnumberedConfigured(swIfIndex uint32) bool
	GetUnnumberedTarget(swIfIndex uint32) (uint32, bool)
	IsIPv6Enabled(swIfIndex uint32) bool
	IsPuntEnabled(swIfIndex uint32, protocol uint8) bool
	GetIPv6RAConfig(swIfIndex uint32) *IPv6RAState
	IsDHCPv6MulticastEnabled(swIfIndex uint32) bool
}

type PuntConfig struct {
	Enabled    bool
	SocketPath string
}

type SVLANConfig struct {
	Enabled bool
	IPv4    []string
	IPv6    []string
}

type UnnumberedConfig struct {
	Enabled   bool
	Interface string
}

type DisableARPReplyConfig struct {
	Enabled bool
}

type IPv6EnabledConfig struct {
	Enabled bool
}

type InternalIPv6RAConfig struct {
	Managed        bool
	Other          bool
	RouterLifetime uint32
	MaxInterval    uint32
	MinInterval    uint32
}

type IPv6MulticastConfig struct {
	Enabled bool
}
