package southbound

import "net"

type Multicast interface {
	AddMfibLocalReceiveAllInterfaces(group net.IP, tableID uint32) error
	AddMfibAcceptOnInterface(group net.IP, grpPrefixLen uint8, swIfIndex uint32, tableID uint32) error
	DumpMroutes() ([]MrouteInfo, error)
}
