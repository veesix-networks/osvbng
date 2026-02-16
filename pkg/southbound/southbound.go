package southbound

import "fmt"

var ErrUnavailable = fmt.Errorf("southbound dataplane unavailable")

type Southbound interface {
	Interfaces
	Addressing
	Routing
	IPv6
	Punt
	MPLS
	Multicast
	Sessions
	Statistics
	Tables
	System
}
