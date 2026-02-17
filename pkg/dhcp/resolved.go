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

	ClasslessRoutes []ClasslessRoute
}

type ClasslessRoute struct {
	Destination *net.IPNet
	NextHop     net.IP
}
