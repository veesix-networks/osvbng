package ifmgr

import (
	"net"
	"net/netip"
	"sync"
)

// addrIndex is the value under one address in byIPv4 or byIPv6: every
// interface carrying it, each with its FIB table. One address may exist
// once per table (a gateway loopback per customer VRF), so an index
// keyed by address alone holds one of them and answers for the wrong
// VRF. Values are replaced, never mutated, so readers on the punt path
// take no lock; writers serialize on idxMu.
type addrIndex map[uint32]uint32

type Manager struct {
	bySwIfIndex sync.Map
	byName      sync.Map
	byIPv4      sync.Map
	byIPv6      sync.Map
	idxMu       sync.Mutex
}

func New() *Manager {
	return &Manager{}
}

func (m *Manager) indexAdd(idx *sync.Map, addr netip.Addr, swIfIndex, tableID uint32) {
	m.idxMu.Lock()
	defer m.idxMu.Unlock()
	next := addrIndex{}
	if v, ok := idx.Load(addr); ok {
		for k, t := range v.(addrIndex) {
			next[k] = t
		}
	}
	next[swIfIndex] = tableID
	idx.Store(addr, next)
}

func (m *Manager) indexRemove(idx *sync.Map, addr netip.Addr, swIfIndex uint32) {
	m.idxMu.Lock()
	defer m.idxMu.Unlock()
	v, ok := idx.Load(addr)
	if !ok {
		return
	}
	cur := v.(addrIndex)
	if _, held := cur[swIfIndex]; !held {
		return
	}
	if len(cur) == 1 {
		idx.Delete(addr)
		return
	}
	next := addrIndex{}
	for k, t := range cur {
		if k != swIfIndex {
			next[k] = t
		}
	}
	idx.Store(addr, next)
}

func indexHasInFIB(idx *sync.Map, addr netip.Addr, tableID uint32) bool {
	v, ok := idx.Load(addr)
	if !ok {
		return false
	}
	for _, t := range v.(addrIndex) {
		if t == tableID {
			return true
		}
	}
	return false
}

func (m *Manager) Add(iface *Interface) {
	m.bySwIfIndex.Store(iface.SwIfIndex, iface)
	if iface.Name != "" {
		m.byName.Store(iface.Name, iface)
	}
}

func (m *Manager) Rename(oldName, newName string) {
	if v, ok := m.byName.LoadAndDelete(oldName); ok {
		iface := v.(*Interface)
		iface.Name = newName
		m.byName.Store(newName, iface)
	}
}

func (m *Manager) Remove(swIfIndex uint32) {
	if v, ok := m.bySwIfIndex.LoadAndDelete(swIfIndex); ok {
		iface := v.(*Interface)
		if iface.Name != "" {
			m.byName.Delete(iface.Name)
		}
		for _, ip := range iface.IPv4Addresses {
			if addr, ok := netip.AddrFromSlice(ip.To4()); ok {
				m.indexRemove(&m.byIPv4, addr, swIfIndex)
			}
		}
		for _, ip := range iface.IPv6Addresses {
			if addr, ok := netip.AddrFromSlice(ip); ok {
				m.indexRemove(&m.byIPv6, addr.Unmap(), swIfIndex)
			}
		}
	}
}

func (m *Manager) Get(swIfIndex uint32) *Interface {
	if v, ok := m.bySwIfIndex.Load(swIfIndex); ok {
		return v.(*Interface)
	}
	return nil
}

func (m *Manager) GetByName(name string) *Interface {
	if v, ok := m.byName.Load(name); ok {
		return v.(*Interface)
	}
	if v, ok := m.byName.Load("host-" + name); ok {
		return v.(*Interface)
	}
	return nil
}

func (m *Manager) OuterTPID(swIfIndex uint32) uint16 {
	idx := swIfIndex
	for hop := 0; hop < 4; hop++ {
		iface := m.Get(idx)
		if iface == nil {
			return 0
		}
		if iface.OuterTPID != 0 {
			return iface.OuterTPID
		}
		if iface.SupSwIfIndex == idx || iface.SupSwIfIndex == 0 {
			return 0
		}
		idx = iface.SupSwIfIndex
	}
	return 0
}

func (m *Manager) GetSupSwIfIndex(swIfIndex uint32) (uint32, bool) {
	if v, ok := m.bySwIfIndex.Load(swIfIndex); ok {
		return v.(*Interface).SupSwIfIndex, true
	}
	return 0, false
}

func (m *Manager) GetSwIfIndex(name string) (uint32, bool) {
	if v, ok := m.byName.Load(name); ok {
		return v.(*Interface).SwIfIndex, true
	}
	if v, ok := m.byName.Load("host-" + name); ok {
		return v.(*Interface).SwIfIndex, true
	}
	return 0, false
}

func (m *Manager) List() []*Interface {
	var result []*Interface
	m.bySwIfIndex.Range(func(_, v any) bool {
		result = append(result, v.(*Interface))
		return true
	})
	return result
}

func (m *Manager) Clear() {
	m.bySwIfIndex.Range(func(k, _ any) bool {
		m.bySwIfIndex.Delete(k)
		return true
	})
	m.byName.Range(func(k, _ any) bool {
		m.byName.Delete(k)
		return true
	})
	m.byIPv4.Range(func(k, _ any) bool {
		m.byIPv4.Delete(k)
		return true
	})
	m.byIPv6.Range(func(k, _ any) bool {
		m.byIPv6.Delete(k)
		return true
	})
}

func (m *Manager) AddIPv4Address(swIfIndex uint32, ip net.IP) {
	v, ok := m.bySwIfIndex.Load(swIfIndex)
	if !ok {
		return
	}

	v4 := ip.To4()
	if v4 == nil {
		return
	}

	iface := v.(*Interface)
	iface.mu.Lock()
	for _, existing := range iface.IPv4Addresses {
		if existing.Equal(v4) {
			iface.mu.Unlock()
			return
		}
	}
	iface.IPv4Addresses = append(iface.IPv4Addresses, v4)
	tableID := iface.FIBTableID
	iface.mu.Unlock()

	if addr, ok := netip.AddrFromSlice(v4); ok {
		m.indexAdd(&m.byIPv4, addr, swIfIndex, tableID)
	}
}

func (m *Manager) RemoveIPv4Address(swIfIndex uint32, ip net.IP) {
	v, ok := m.bySwIfIndex.Load(swIfIndex)
	if !ok {
		return
	}

	v4 := ip.To4()
	if v4 == nil {
		return
	}

	iface := v.(*Interface)
	iface.mu.Lock()
	for i, existing := range iface.IPv4Addresses {
		if existing.Equal(v4) {
			iface.IPv4Addresses = append(iface.IPv4Addresses[:i], iface.IPv4Addresses[i+1:]...)
			iface.mu.Unlock()
			if addr, ok := netip.AddrFromSlice(v4); ok {
				m.indexRemove(&m.byIPv4, addr, swIfIndex)
			}
			return
		}
	}
	iface.mu.Unlock()
}

func (m *Manager) AddIPv6Address(swIfIndex uint32, ip net.IP) {
	v, ok := m.bySwIfIndex.Load(swIfIndex)
	if !ok {
		return
	}

	v6 := ip.To16()
	if v6 == nil {
		return
	}

	iface := v.(*Interface)
	iface.mu.Lock()
	for _, existing := range iface.IPv6Addresses {
		if existing.Equal(v6) {
			iface.mu.Unlock()
			return
		}
	}
	iface.IPv6Addresses = append(iface.IPv6Addresses, v6)
	tableID := iface.FIBTableID
	iface.mu.Unlock()

	if addr, ok := netip.AddrFromSlice(v6); ok {
		m.indexAdd(&m.byIPv6, addr.Unmap(), swIfIndex, tableID)
	}
}

func (m *Manager) RemoveIPv6Address(swIfIndex uint32, ip net.IP) {
	v, ok := m.bySwIfIndex.Load(swIfIndex)
	if !ok {
		return
	}

	v6 := ip.To16()
	if v6 == nil {
		return
	}

	iface := v.(*Interface)
	iface.mu.Lock()
	for i, existing := range iface.IPv6Addresses {
		if existing.Equal(v6) {
			iface.IPv6Addresses = append(iface.IPv6Addresses[:i], iface.IPv6Addresses[i+1:]...)
			iface.mu.Unlock()
			if addr, ok := netip.AddrFromSlice(v6); ok {
				m.indexRemove(&m.byIPv6, addr.Unmap(), swIfIndex)
			}
			return
		}
	}
	iface.mu.Unlock()
}

func (m *Manager) SetFIBTableID(swIfIndex uint32, tableID uint32) {
	v, ok := m.bySwIfIndex.Load(swIfIndex)
	if !ok {
		return
	}
	iface := v.(*Interface)
	iface.mu.Lock()
	iface.FIBTableID = tableID
	v4s := append([]net.IP(nil), iface.IPv4Addresses...)
	v6s := append([]net.IP(nil), iface.IPv6Addresses...)
	iface.mu.Unlock()
	for _, ip := range v4s {
		if v4 := ip.To4(); v4 != nil {
			if addr, ok := netip.AddrFromSlice(v4); ok {
				m.indexAdd(&m.byIPv4, addr, swIfIndex, tableID)
			}
		}
	}
	for _, ip := range v6s {
		if addr, ok := netip.AddrFromSlice(ip.To16()); ok {
			m.indexAdd(&m.byIPv6, addr.Unmap(), swIfIndex, tableID)
		}
	}
}

func (m *Manager) HasIPv4(ip net.IP) bool {
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	addr, ok := netip.AddrFromSlice(v4)
	if !ok {
		return false
	}
	_, found := m.byIPv4.Load(addr)
	return found
}

func (m *Manager) HasIPv4InFIB(ip net.IP, tableID uint32) bool {
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	addr, ok := netip.AddrFromSlice(v4)
	if !ok {
		return false
	}
	return indexHasInFIB(&m.byIPv4, addr, tableID)
}
