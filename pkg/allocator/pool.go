package allocator

import (
	"net"
	"net/netip"
	"sync"
)

type PoolAllocator struct {
	rangeStart netip.Addr
	rangeEnd   netip.Addr
	excluded   map[netip.Addr]bool
	leases     map[netip.Addr]string // addr → session ID
	mu         sync.Mutex
}

func NewPoolAllocator(rangeStart, rangeEnd netip.Addr, excludeAddrs []netip.Addr) *PoolAllocator {
	excluded := make(map[netip.Addr]bool, len(excludeAddrs))
	for _, addr := range excludeAddrs {
		excluded[addr.Unmap()] = true
	}
	return &PoolAllocator{
		rangeStart: rangeStart.Unmap(),
		rangeEnd:   rangeEnd.Unmap(),
		excluded:   excluded,
		leases:     make(map[netip.Addr]string),
	}
}

func (a *PoolAllocator) Allocate(sessionID string) (net.IP, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for addr := a.rangeStart; addr.Compare(a.rangeEnd) <= 0; addr = addr.Next() {
		if a.excluded[addr] {
			continue
		}
		if _, used := a.leases[addr]; !used {
			a.leases[addr] = sessionID
			return net.IP(addr.AsSlice()), nil
		}
	}
	return nil, ErrPoolExhausted
}

func (a *PoolAllocator) Release(ip net.IP) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return nil
	}
	delete(a.leases, addr.Unmap())
	return nil
}

func (a *PoolAllocator) Reserve(ip net.IP, sessionID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return nil
	}
	addr = addr.Unmap()
	if existing, exists := a.leases[addr]; exists && existing != sessionID {
		return ErrAlreadyReserved
	}
	a.leases[addr] = sessionID
	return nil
}

func (a *PoolAllocator) Available() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	count := 0
	for addr := a.rangeStart; addr.Compare(a.rangeEnd) <= 0; addr = addr.Next() {
		if a.excluded[addr] {
			continue
		}
		if _, used := a.leases[addr]; !used {
			count++
		}
	}
	return count
}
