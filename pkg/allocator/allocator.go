package allocator

import (
	"errors"
	"net"
)

var (
	ErrPoolExhausted  = errors.New("pool exhausted")
	ErrAlreadyReserved = errors.New("address already reserved")
)

type Allocator interface {
	Allocate(sessionID string) (net.IP, error)
	Release(ip net.IP) error
	Reserve(ip net.IP, sessionID string) error
	Available() int
}
