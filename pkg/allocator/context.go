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

	FramedIPv4       net.IP
	FramedIPv4Mask   net.IPMask
	FramedIPv6       net.IP
	FramedIPv6Prefix *net.IPNet

	DNSv4   []net.IP
	DNSv6   []net.IP
	Gateway net.IP

	PoolOverride     string
	IANAPoolOverride string
	PDPoolOverride   string
}
