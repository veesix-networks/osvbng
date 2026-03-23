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
	free       []netip.Addr
	ascending  bool
	mu         sync.Mutex
}

func NewPoolAllocator(rangeStart, rangeEnd netip.Addr, excludeAddrs []netip.Addr) *PoolAllocator {
	excluded := make(map[netip.Addr]bool, len(excludeAddrs))
	for _, addr := range excludeAddrs {
		excluded[addr.Unmap()] = true
	}
	a := &PoolAllocator{
		rangeStart: rangeStart.Unmap(),
		rangeEnd:   rangeEnd.Unmap(),
		excluded:   excluded,
		leases:     make(map[netip.Addr]string),
		ascending:  true,
	}
	a.buildFreeList()
	return a
}

func (a *PoolAllocator) buildFreeList() {
	var addrs []netip.Addr
	for addr := a.rangeStart; addr.Compare(a.rangeEnd) <= 0; addr = addr.Next() {
		if a.excluded[addr] {
			continue
		}
		if _, used := a.leases[addr]; used {
			continue
		}
		addrs = append(addrs, addr)
	}

	a.free = make([]netip.Addr, len(addrs))
	if a.ascending {
		for i, addr := range addrs {
			a.free[len(addrs)-1-i] = addr
		}
	} else {
		copy(a.free, addrs)
	}
}

func (a *PoolAllocator) SetDirection(ascending bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ascending == ascending {
		return
	}
	a.ascending = ascending
	a.buildFreeList()
}

func (a *PoolAllocator) Allocate(sessionID string) (net.IP, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.free) == 0 {
		return nil, ErrPoolExhausted
	}

	addr := a.free[len(a.free)-1]
	a.free = a.free[:len(a.free)-1]
	a.leases[addr] = sessionID
	return net.IP(addr.AsSlice()), nil
}

func (a *PoolAllocator) Release(ip net.IP) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return nil
	}
	addr = addr.Unmap()
	if _, exists := a.leases[addr]; exists {
		delete(a.leases, addr)
		a.free = append(a.free, addr)
	}
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
	if _, exists := a.leases[addr]; !exists {
		for i, freeAddr := range a.free {
			if freeAddr == addr {
				a.free = append(a.free[:i], a.free[i+1:]...)
				break
			}
		}
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
	return len(a.free)
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
