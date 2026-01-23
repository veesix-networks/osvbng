package dataplane

type Protocol uint8

const (
	ProtocolUnknown Protocol = iota
	ProtocolDHCPv4Type
	ProtocolDHCPv6Type
	ProtocolARPType
	ProtocolPPPoEDiscoveryType
	ProtocolPPPoESessionType
	ProtocolIPv6NDType
	ProtocolL2TPType
)

func (p Protocol) String() string {
	switch p {
	case ProtocolDHCPv4Type:
		return ProtocolDHCPv4
	case ProtocolDHCPv6Type:
		return ProtocolDHCPv6
	case ProtocolARPType:
		return ProtocolARP
	case ProtocolPPPoEDiscoveryType:
		return ProtocolPPPoEDiscovery
	case ProtocolPPPoESessionType:
		return ProtocolPPPoESession
	case ProtocolIPv6NDType:
		return ProtocolIPv6ND
	case ProtocolL2TPType:
		return ProtocolL2TP
	default:
		return "unknown"
	}
}
