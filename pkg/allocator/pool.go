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
	leases     map[netip.Addr]string
	ascending  bool
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
		ascending:  true,
	}
}

func (a *PoolAllocator) SetDirection(ascending bool) {
	a.mu.Lock()
	a.ascending = ascending
	a.mu.Unlock()
}

func (a *PoolAllocator) Allocate(sessionID string) (net.IP, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.ascending {
		for addr := a.rangeStart; addr.Compare(a.rangeEnd) <= 0; addr = addr.Next() {
			if a.excluded[addr] {
				continue
			}
			if _, used := a.leases[addr]; !used {
				a.leases[addr] = sessionID
				return net.IP(addr.AsSlice()), nil
			}
		}
	} else {
		for addr := a.rangeEnd; ; addr = prevAddr(addr) {
			if !a.excluded[addr] {
				if _, used := a.leases[addr]; !used {
					a.leases[addr] = sessionID
					return net.IP(addr.AsSlice()), nil
				}
			}
			if addr == a.rangeStart {
				break
			}
		}
	}
	return nil, ErrPoolExhausted
}

func prevAddr(a netip.Addr) netip.Addr {
	if a.Is4() {
		b := a.As4()
		for i := 3; i >= 0; i-- {
			if b[i] > 0 {
				b[i]--
				break
			}
			b[i] = 0xff
		}
		return netip.AddrFrom4(b)
	}
	b := a.As16()
	for i := 15; i >= 0; i-- {
		if b[i] > 0 {
			b[i]--
			break
		}
		b[i] = 0xff
	}
	return netip.AddrFrom16(b)
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

func (a *PoolAllocator) Contains(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	return addr.Compare(a.rangeStart) >= 0 && addr.Compare(a.rangeEnd) <= 0
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
