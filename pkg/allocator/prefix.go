package allocator

import (
	"encoding/binary"
	"net"
	"net/netip"
	"sync"
)

type PrefixAllocator struct {
	base         netip.Addr
	networkBits  int
	prefixLength int
	count        uint64
	leases       map[uint64]string
	free         []uint64
	ascending    bool
	mu           sync.Mutex
}

func NewPrefixAllocator(network netip.Prefix, prefixLength int) *PrefixAllocator {
	delegBits := prefixLength - network.Bits()
	if delegBits < 0 || delegBits > 63 {
		return nil
	}
	count := uint64(1) << uint(delegBits)
	a := &PrefixAllocator{
		base:         network.Masked().Addr(),
		networkBits:  network.Bits(),
		prefixLength: prefixLength,
		count:        count,
		leases:       make(map[uint64]string),
		ascending:    true,
	}
	a.buildFreeList()
	return a
}

func (a *PrefixAllocator) buildFreeList() {
	a.free = make([]uint64, 0, a.count)
	if a.ascending {
		for i := a.count - 1; ; i-- {
			if _, used := a.leases[i]; !used {
				a.free = append(a.free, i)
			}
			if i == 0 {
				break
			}
		}
	} else {
		for i := uint64(0); i < a.count; i++ {
			if _, used := a.leases[i]; !used {
				a.free = append(a.free, i)
			}
		}
	}
}

func (a *PrefixAllocator) SetDirection(ascending bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.ascending == ascending {
		return
	}
	a.ascending = ascending
	a.buildFreeList()
}

func (a *PrefixAllocator) Allocate(sessionID string) (*net.IPNet, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.free) == 0 {
		return nil, ErrPoolExhausted
	}

	idx := a.free[len(a.free)-1]
	a.free = a.free[:len(a.free)-1]
	a.leases[idx] = sessionID
	return a.indexToIPNet(idx), nil
}

func (a *PrefixAllocator) Release(prefix *net.IPNet) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if idx, ok := a.prefixToIndex(prefix); ok {
		if _, exists := a.leases[idx]; exists {
			delete(a.leases, idx)
			a.free = append(a.free, idx)
		}
	}
}

func (a *PrefixAllocator) Reserve(prefix *net.IPNet, sessionID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	idx, ok := a.prefixToIndex(prefix)
	if !ok {
		return nil
	}
	if existing, exists := a.leases[idx]; exists && existing != sessionID {
		return ErrAlreadyReserved
	}
	if _, exists := a.leases[idx]; !exists {
		for i, freeIdx := range a.free {
			if freeIdx == idx {
				a.free = append(a.free[:i], a.free[i+1:]...)
				break
			}
		}
	}
	a.leases[idx] = sessionID
	return nil
}

func (a *PrefixAllocator) Contains(prefix *net.IPNet) bool {
	_, ok := a.prefixToIndex(prefix)
	return ok
}

func (a *PrefixAllocator) indexToIPNet(idx uint64) *net.IPNet {
	b := a.base.As16()
	shift := uint(128 - a.prefixLength)

	hi := binary.BigEndian.Uint64(b[:8])
	lo := binary.BigEndian.Uint64(b[8:])

	var addHi, addLo uint64
	if shift >= 64 {
		addHi = idx << (shift - 64)
	} else if shift == 0 {
		addLo = idx
	} else {
		addLo = idx << shift
		addHi = idx >> (64 - shift)
	}

	newLo := lo + addLo
	carry := uint64(0)
	if newLo < lo {
		carry = 1
	}

	binary.BigEndian.PutUint64(b[:8], hi+addHi+carry)
	binary.BigEndian.PutUint64(b[8:], newLo)

	ip := make(net.IP, 16)
	copy(ip, b[:])
	return &net.IPNet{
		IP:   ip,
		Mask: net.CIDRMask(a.prefixLength, 128),
	}
}

func (a *PrefixAllocator) prefixToIndex(prefix *net.IPNet) (uint64, bool) {
	if prefix == nil {
		return 0, false
	}
	ones, bits := prefix.Mask.Size()
	if bits != 128 || ones != a.prefixLength {
		return 0, false
	}

	addr, ok := netip.AddrFromSlice(prefix.IP)
	if !ok {
		return 0, false
	}
	addr = addr.Unmap()

	ab := addr.As16()
	bb := a.base.As16()

	addrHi := binary.BigEndian.Uint64(ab[:8])
	addrLo := binary.BigEndian.Uint64(ab[8:])
	baseHi := binary.BigEndian.Uint64(bb[:8])
	baseLo := binary.BigEndian.Uint64(bb[8:])

	diffLo := addrLo - baseLo
	borrow := uint64(0)
	if addrLo < baseLo {
		borrow = 1
	}
	diffHi := addrHi - baseHi - borrow

	shift := uint(128 - a.prefixLength)
	var idx uint64
	if shift >= 64 {
		idx = diffHi >> (shift - 64)
	} else if shift == 0 {
		idx = diffLo
	} else {
		idx = (diffHi << (64 - shift)) | (diffLo >> shift)
	}

	if idx >= a.count {
		return 0, false
	}

	return idx, true
}
