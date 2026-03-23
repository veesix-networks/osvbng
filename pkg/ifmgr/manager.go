package ifmgr

import (
	"net"
	"net/netip"
	"sync"
)

type ipIndex struct {
	SwIfIndex  uint32
	FIBTableID uint32
}

type Manager struct {
	bySwIfIndex sync.Map
	byName      sync.Map
	byIPv4      sync.Map
	byIPv6      sync.Map
}

func New() *Manager {
	return &Manager{}
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
				m.byIPv4.Delete(addr)
			}
		}
		for _, ip := range iface.IPv6Addresses {
			if addr, ok := netip.AddrFromSlice(ip); ok {
				m.byIPv6.Delete(addr.Unmap())
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
	iface.mu.Unlock()

	if addr, ok := netip.AddrFromSlice(v4); ok {
		m.byIPv4.Store(addr, &ipIndex{SwIfIndex: swIfIndex, FIBTableID: iface.FIBTableID})
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
				m.byIPv4.Delete(addr)
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
	iface.mu.Unlock()

	if addr, ok := netip.AddrFromSlice(v6); ok {
		m.byIPv6.Store(addr.Unmap(), &ipIndex{SwIfIndex: swIfIndex, FIBTableID: iface.FIBTableID})
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
				m.byIPv6.Delete(addr.Unmap())
			}
			return
		}
	}
	iface.mu.Unlock()
}

func (m *Manager) SetFIBTableID(swIfIndex uint32, tableID uint32) {
	if v, ok := m.bySwIfIndex.Load(swIfIndex); ok {
		v.(*Interface).FIBTableID = tableID
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
	v, found := m.byIPv4.Load(addr)
	if !found {
		return false
	}
	return v.(*ipIndex).FIBTableID == tableID
}
