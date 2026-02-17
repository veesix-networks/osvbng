package allocator

import "net"

type Context struct {
	SessionID string
	MAC       net.HardwareAddr
	SVLAN     uint16
	CVLAN     uint16

	VRF             string
	SubscriberGroup string
	ServiceGroup    string
	ProfileName     string

	IPv4Address net.IP
	IPv4Netmask net.IPMask
	IPv4Gateway net.IP
	IPv6Address net.IP
	IPv6Prefix  *net.IPNet

	DNSv4 []net.IP
	DNSv6 []net.IP

	PoolOverride     string
	IANAPoolOverride string
	PDPoolOverride   string
}
