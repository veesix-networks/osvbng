package allocator

import (
	"net"
	"time"
)

type BindingType int

const (
	BindingLease       BindingType = iota
	BindingReservation
)

func (t BindingType) String() string {
	switch t {
	case BindingLease:
		return "lease"
	case BindingReservation:
		return "reservation"
	default:
		return "unknown"
	}
}

type Binding struct {
	Type      BindingType
	IP        net.IP
	SessionID string
	ExpiresAt time.Time
	Source    string
}
