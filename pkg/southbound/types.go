package southbound

import "net"

type InterfaceInfo struct {
	SwIfIndex    uint32
	Name         string
	AdminUp      bool
	LinkUp       bool
	MTU          uint32
	OuterVlanID  uint16
	InnerVlanID  uint16
	SupSwIfIndex uint32
}

type IPAddressInfo struct {
	SwIfIndex uint32
	Address   string
	IsIPv6    bool
}

type UnnumberedInfo struct {
	SwIfIndex   uint32
	IPSwIfIndex uint32
}

type IPv6RAConfig struct {
	Managed        bool   // M flag
	Other          bool   // O flag
	RouterLifetime uint32
	MaxInterval    uint32
	MinInterval    uint32
}

type IPv6RAPrefixConfig struct {
	Prefix            string
	OnLink            bool // L flag
	Autonomous        bool // A flag
	ValidLifetime     uint32
	PreferredLifetime uint32
}

type IPv6RAInfo struct {
	SwIfIndex          uint32
	Managed            bool
	Other              bool
	RouterLifetimeSecs uint16
	MaxIntervalSecs    float64
	MinIntervalSecs    float64
	SendRadv           bool
}

type PuntRegistration struct {
	SwIfIndex uint32
	Protocol  uint8
}

type PuntStats struct {
	Protocol       uint8
	PacketsPunted  uint64
	PacketsDropped uint64
	PacketsPoliced uint64
	PolicerRate    float64
	PolicerBurst   uint32
}

type MrouteInfo struct {
	TableID    uint32
	GrpAddress net.IP
	SrcAddress net.IP
	IsIPv6     bool
}

type IPTableInfo struct {
	TableID uint32 `json:"table_id"`
	Name    string `json:"name"`
	IsIPv6  bool   `json:"is_ipv6"`
}

type IPMTableInfo struct {
	TableID uint32
	Name    string
	IsIPv6  bool
}

type MPLSTableInfo struct {
	TableID uint32
	Name    string
}

type MPLSRouteEntry struct {
	Label       uint32          `json:"label"`
	Eos         bool            `json:"eos"`
	EosProto    uint8           `json:"eos_proto"`
	IsMulticast bool            `json:"is_multicast"`
	Paths       []MPLSRoutePath `json:"paths"`
}

type MPLSRoutePath struct {
	SwIfIndex  uint32   `json:"sw_if_index"`
	Interface  string   `json:"interface,omitempty"`
	NextHop    string   `json:"next_hop,omitempty"`
	Weight     uint8    `json:"weight"`
	Preference uint8    `json:"preference"`
	Labels     []uint32 `json:"labels,omitempty"`
}

type MPLSInterfaceInfo struct {
	SwIfIndex uint32 `json:"sw_if_index"`
	Name      string `json:"name,omitempty"`
}
