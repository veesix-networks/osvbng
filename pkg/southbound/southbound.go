package southbound

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

var _ Southbound = (*VPP)(nil)
