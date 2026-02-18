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
}

type ClasslessRoute struct {
	Destination *net.IPNet
	NextHop     net.IP
}

type ResolvedDHCPv6 struct {
	IANAAddress       net.IP
	IANAPreferredTime uint32
	IANAValidTime     uint32
	PDPrefix          *net.IPNet
	PDPreferredTime   uint32
	PDValidTime       uint32
	DNS               []net.IP
}
