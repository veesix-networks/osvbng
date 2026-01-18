package operations

import (
	"github.com/veesix-networks/osvbng/pkg/config"
)

// MockDataplane is a no-op dataplane for testing
type MockDataplane struct {
	CreatedInterfaces []string
	DeletedInterfaces []string
	AddedIPv4         map[string][]string
	AddedIPv6         map[string][]string
	AddedRoutes       []*config.StaticRoute
}

func NewMockDataplane() *MockDataplane {
	return &MockDataplane{
		CreatedInterfaces: make([]string, 0),
		DeletedInterfaces: make([]string, 0),
		AddedIPv4:         make(map[string][]string),
		AddedIPv6:         make(map[string][]string),
		AddedRoutes:       make([]*config.StaticRoute, 0),
	}
}

func (m *MockDataplane) CreateInterface(cfg *config.InterfaceConfig) error {
	m.CreatedInterfaces = append(m.CreatedInterfaces, cfg.Name)
	return nil
}

func (m *MockDataplane) DeleteInterface(name string) error {
	m.DeletedInterfaces = append(m.DeletedInterfaces, name)
	return nil
}

func (m *MockDataplane) SetInterfaceDescription(name, description string) error {
	return nil
}

func (m *MockDataplane) SetInterfaceMTU(name string, mtu int) error {
	return nil
}

func (m *MockDataplane) SetInterfaceEnabled(name string, enabled bool) error {
	return nil
}

func (m *MockDataplane) AddIPv4Address(ifName, address string) error {
	if m.AddedIPv4[ifName] == nil {
		m.AddedIPv4[ifName] = make([]string, 0)
	}
	m.AddedIPv4[ifName] = append(m.AddedIPv4[ifName], address)
	return nil
}

func (m *MockDataplane) DelIPv4Address(ifName, address string) error {
	return nil
}

func (m *MockDataplane) AddIPv6Address(ifName, address string) error {
	if m.AddedIPv6[ifName] == nil {
		m.AddedIPv6[ifName] = make([]string, 0)
	}
	m.AddedIPv6[ifName] = append(m.AddedIPv6[ifName], address)
	return nil
}

func (m *MockDataplane) DelIPv6Address(ifName, address string) error {
	return nil
}

func (m *MockDataplane) AddRoute(route *config.StaticRoute) error {
	m.AddedRoutes = append(m.AddedRoutes, route)
	return nil
}

func (m *MockDataplane) DelRoute(route *config.StaticRoute) error {
	return nil
}
