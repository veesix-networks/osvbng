package ifmgr

import (
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

	return m.byName[name]
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
