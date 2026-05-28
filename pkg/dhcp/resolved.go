package dhcp

import (
	"net"
	"time"
)

type ResolvedDHCPv4 struct {
	YourIP    net.IP
	Netmask   net.IPMask
	Router    net.IP
	DNS       []net.IP
	LeaseTime time.Duration
	ServerID  net.IP
	PoolName  string

	ClasslessRoutes []ClasslessRoute
	Options         []EncodedOption
}

type ClasslessRoute struct {
	Destination *net.IPNet
	NextHop     net.IP
}

// EncodedOption is one fully-decoded DHCPv4 option ready for emission.
type EncodedOption struct {
	Tag     uint8
	Payload []byte
}

type ResolvedDHCPv6 struct {
	IANAAddress       net.IP
	IANAPreferredTime uint32
	IANAValidTime     uint32
	IANAPoolName      string
	PDPrefix          *net.IPNet
	PDPreferredTime   uint32
	PDValidTime       uint32
	PDPoolName        string
	DNS               []net.IP
	Options           []EncodedDHCPv6Option
}

// EncodedDHCPv6Option is one fully-decoded DHCPv6 option ready for
// emission.
type EncodedDHCPv6Option struct {
	Code    uint16
	Payload []byte
}
