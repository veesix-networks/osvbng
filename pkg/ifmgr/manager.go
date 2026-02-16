package ifmgr

import (
	"net"
	"sync"
)

type Manager struct {
	mu            sync.RWMutex
	bySwIfIndex   map[uint32]*Interface
	byName        map[string]*Interface
}

func New() *Manager {
	return &Manager{
		bySwIfIndex: make(map[uint32]*Interface),
		byName:      make(map[string]*Interface),
	}
}

func (m *Manager) Add(iface *Interface) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.bySwIfIndex[iface.SwIfIndex] = iface
	if iface.Name != "" {
		m.byName[iface.Name] = iface
	}
}

func (m *Manager) Rename(oldName, newName string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if iface, ok := m.byName[oldName]; ok {
		delete(m.byName, oldName)
		iface.Name = newName
		m.byName[newName] = iface
	}
}

func (m *Manager) Remove(swIfIndex uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if iface, ok := m.bySwIfIndex[swIfIndex]; ok {
		delete(m.bySwIfIndex, swIfIndex)
		if iface.Name != "" {
			delete(m.byName, iface.Name)
		}
	}
}

func (m *Manager) Get(swIfIndex uint32) *Interface {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.bySwIfIndex[swIfIndex]
}

func (m *Manager) GetByName(name string) *Interface {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if iface, ok := m.byName[name]; ok {
		return iface
	}
	return m.byName["host-"+name]
}

func (m *Manager) GetSupSwIfIndex(swIfIndex uint32) (uint32, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if iface, ok := m.bySwIfIndex[swIfIndex]; ok {
		return iface.SupSwIfIndex, true
	}
	return 0, false
}

func (m *Manager) GetSwIfIndex(name string) (uint32, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if iface, ok := m.byName[name]; ok {
		return iface.SwIfIndex, true
	}
	if iface, ok := m.byName["host-"+name]; ok {
		return iface.SwIfIndex, true
	}
	return 0, false
}

func (m *Manager) List() []*Interface {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Interface, 0, len(m.bySwIfIndex))
	for _, iface := range m.bySwIfIndex {
		result = append(result, iface)
	}
	return result
}

func (m *Manager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.bySwIfIndex = make(map[uint32]*Interface)
	m.byName = make(map[string]*Interface)
}

func (m *Manager) AddIPv4Address(swIfIndex uint32, ip net.IP) {
	m.mu.Lock()
	defer m.mu.Unlock()

	iface, ok := m.bySwIfIndex[swIfIndex]
	if !ok {
		return
	}

	v4 := ip.To4()
	if v4 == nil {
		return
	}

	for _, existing := range iface.IPv4Addresses {
		if existing.Equal(v4) {
			return
		}
	}
	iface.IPv4Addresses = append(iface.IPv4Addresses, v4)
}

func (m *Manager) RemoveIPv4Address(swIfIndex uint32, ip net.IP) {
	m.mu.Lock()
	defer m.mu.Unlock()

	iface, ok := m.bySwIfIndex[swIfIndex]
	if !ok {
		return
	}

	v4 := ip.To4()
	if v4 == nil {
		return
	}

	for i, existing := range iface.IPv4Addresses {
		if existing.Equal(v4) {
			iface.IPv4Addresses = append(iface.IPv4Addresses[:i], iface.IPv4Addresses[i+1:]...)
			return
		}
	}
}

func (m *Manager) AddIPv6Address(swIfIndex uint32, ip net.IP) {
	m.mu.Lock()
	defer m.mu.Unlock()

	iface, ok := m.bySwIfIndex[swIfIndex]
	if !ok {
		return
	}

	v6 := ip.To16()
	if v6 == nil {
		return
	}

	for _, existing := range iface.IPv6Addresses {
		if existing.Equal(v6) {
			return
		}
	}
	iface.IPv6Addresses = append(iface.IPv6Addresses, v6)
}

func (m *Manager) RemoveIPv6Address(swIfIndex uint32, ip net.IP) {
	m.mu.Lock()
	defer m.mu.Unlock()

	iface, ok := m.bySwIfIndex[swIfIndex]
	if !ok {
		return
	}

	v6 := ip.To16()
	if v6 == nil {
		return
	}

	for i, existing := range iface.IPv6Addresses {
		if existing.Equal(v6) {
			iface.IPv6Addresses = append(iface.IPv6Addresses[:i], iface.IPv6Addresses[i+1:]...)
			return
		}
	}
}

func (m *Manager) SetFIBTableID(swIfIndex uint32, tableID uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if iface, ok := m.bySwIfIndex[swIfIndex]; ok {
		iface.FIBTableID = tableID
	}
}

func (m *Manager) HasIPv4(ip net.IP) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	v4 := ip.To4()
	if v4 == nil {
		return false
	}

	for _, iface := range m.bySwIfIndex {
		for _, addr := range iface.IPv4Addresses {
			if addr.Equal(v4) {
				return true
			}
		}
	}
	return false
}

func (m *Manager) HasIPv4InFIB(ip net.IP, tableID uint32) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	v4 := ip.To4()
	if v4 == nil {
		return false
	}

	for _, iface := range m.bySwIfIndex {
		if iface.FIBTableID != tableID {
			continue
		}
		for _, addr := range iface.IPv4Addresses {
			if addr.Equal(v4) {
				return true
			}
		}
	}
	return false
}
