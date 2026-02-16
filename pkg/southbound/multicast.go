package southbound

import "net"

type Multicast interface {
	AddMfibLocalReceiveAllInterfaces(group net.IP, tableID uint32) error
	DumpMroutes() ([]MrouteInfo, error)
}
