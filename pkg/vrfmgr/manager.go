package vrfmgr

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/vishvananda/netlink"

	"github.com/veesix-networks/osvbng/pkg/config/ip"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models/vrf"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

const (
	tableIDMin = uint32(100)
	tableIDMax = uint32(4095)
)

type vrfEntry struct {
	Name    string
	TableID uint32
	IPv4    bool
	IPv6    bool
}

type Manager struct {
	mu            sync.RWMutex
	vrfs          map[string]*vrfEntry
	vpp           *southbound.VPP
	logger        *slog.Logger
	netlinkHandle *netlink.Handle
}

func New(vpp *southbound.VPP) *Manager {
	return &Manager{
		vrfs:   make(map[string]*vrfEntry),
		vpp:    vpp,
		logger: logger.Get(logger.Routing),
	}
}

func (m *Manager) SetNetlinkHandle(h *netlink.Handle) {
	m.netlinkHandle = h
}

func (m *Manager) CreateVRF(ctx context.Context, name string, ipv4 bool, ipv6 bool) (uint32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if e, ok := m.vrfs[name]; ok {
		return e.TableID, nil
	}

	tableID, err := m.allocateTableID()
	if err != nil {
		return 0, fmt.Errorf("allocate table ID for VRF %q: %w", name, err)
	}

	if err := m.createLinuxVRF(name, tableID); err != nil {
		return 0, fmt.Errorf("create Linux VRF %q: %w", name, err)
	}

	if ipv4 {
		if err := m.vpp.AddIPTable(tableID, false, name); err != nil {
			m.deleteLinuxVRF(name)
			return 0, fmt.Errorf("create VPP IPv4 table for VRF %q: %w", name, err)
		}
	}

	if ipv6 {
		if err := m.vpp.AddIPTable(tableID, true, name); err != nil {
			if ipv4 {
				m.vpp.DelIPTable(tableID, false)
			}
			m.deleteLinuxVRF(name)
			return 0, fmt.Errorf("create VPP IPv6 table for VRF %q: %w", name, err)
		}
	}

	m.vrfs[name] = &vrfEntry{
		Name:    name,
		TableID: tableID,
		IPv4:    ipv4,
		IPv6:    ipv6,
	}

	m.logger.Info("Created VRF", "name", name, "table_id", tableID, "ipv4", ipv4, "ipv6", ipv6)
	return tableID, nil
}

func (m *Manager) DeleteVRF(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.vrfs[name]
	if !ok {
		return fmt.Errorf("VRF %q not found", name)
	}

	if e.IPv4 {
		if err := m.vpp.DelIPTable(e.TableID, false); err != nil {
			m.logger.Warn("Failed to delete VPP IPv4 table", "name", name, "table_id", e.TableID, "error", err)
		}
	}

	if e.IPv6 {
		if err := m.vpp.DelIPTable(e.TableID, true); err != nil {
			m.logger.Warn("Failed to delete VPP IPv6 table", "name", name, "table_id", e.TableID, "error", err)
		}
	}

	m.deleteLinuxVRF(name)
	delete(m.vrfs, name)

	m.logger.Info("Deleted VRF", "name", name, "table_id", e.TableID)
	return nil
}

func (m *Manager) ResolveVRF(name string) (uint32, bool, bool, error) {
	if name == "" {
		return 0, false, false, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	e, ok := m.vrfs[name]
	if !ok {
		return 0, false, false, fmt.Errorf("VRF %q not found", name)
	}

	return e.TableID, e.IPv4, e.IPv6, nil
}

func (m *Manager) GetVRFs() []vrf.VRF {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]vrf.VRF, 0, len(m.vrfs))
	for _, e := range m.vrfs {
		v := vrf.VRF{
			Name:    e.Name,
			TableId: e.TableID,
		}
		if e.IPv4 {
			v.AddressFamilies.IPv4Unicast = &vrf.IPv4UnicastAF{}
		}
		if e.IPv6 {
			v.AddressFamilies.IPv6Unicast = &vrf.IPv6UnicastAF{}
		}
		result = append(result, v)
	}

	return result
}

func (m *Manager) Reconcile(ctx context.Context, desired map[string]*ip.VRFSConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, err := m.discoverLinuxVRFs()
	if err != nil {
		m.logger.Warn("Failed to discover existing Linux VRFs", "error", err)
	}

	for name, cfg := range desired {
		ipv4 := cfg.AddressFamilies.IPv4Unicast != nil
		ipv6 := cfg.AddressFamilies.IPv6Unicast != nil

		if tableID, ok := existing[name]; ok {
			if ipv4 {
				m.vpp.AddIPTable(tableID, false, name)
			}
			if ipv6 {
				m.vpp.AddIPTable(tableID, true, name)
			}
			m.vrfs[name] = &vrfEntry{Name: name, TableID: tableID, IPv4: ipv4, IPv6: ipv6}
			m.logger.Info("Reconciled existing VRF", "name", name, "table_id", tableID)
			continue
		}

		tableID, err := m.allocateTableID()
		if err != nil {
			m.logger.Error("Failed to allocate table ID during reconcile", "name", name, "error", err)
			continue
		}

		if err := m.createLinuxVRF(name, tableID); err != nil {
			m.logger.Error("Failed to create Linux VRF during reconcile", "name", name, "error", err)
			continue
		}

		if ipv4 {
			m.vpp.AddIPTable(tableID, false, name)
		}
		if ipv6 {
			m.vpp.AddIPTable(tableID, true, name)
		}

		m.vrfs[name] = &vrfEntry{Name: name, TableID: tableID, IPv4: ipv4, IPv6: ipv6}
		m.logger.Info("Reconciled new VRF", "name", name, "table_id", tableID)
	}

	for name, tableID := range existing {
		if _, wanted := desired[name]; !wanted {
			m.vpp.DelIPTable(tableID, false)
			m.vpp.DelIPTable(tableID, true)
			m.deleteLinuxVRF(name)
			m.logger.Info("Cleaned up stale VRF", "name", name, "table_id", tableID)
		}
	}

	return nil
}

func (m *Manager) allocateTableID() (uint32, error) {
	used := make(map[uint32]bool)
	for _, e := range m.vrfs {
		used[e.TableID] = true
	}

	existing, _ := m.discoverLinuxVRFs()
	for _, id := range existing {
		used[id] = true
	}

	for id := tableIDMin; id <= tableIDMax; id++ {
		if !used[id] {
			return id, nil
		}
	}

	return 0, fmt.Errorf("no available table IDs in range %d-%d", tableIDMin, tableIDMax)
}

func (m *Manager) nlLinkAdd(link netlink.Link) error {
	if m.netlinkHandle != nil {
		return m.netlinkHandle.LinkAdd(link)
	}
	return netlink.LinkAdd(link)
}

func (m *Manager) nlLinkSetUp(link netlink.Link) error {
	if m.netlinkHandle != nil {
		return m.netlinkHandle.LinkSetUp(link)
	}
	return netlink.LinkSetUp(link)
}

func (m *Manager) nlLinkDel(link netlink.Link) error {
	if m.netlinkHandle != nil {
		return m.netlinkHandle.LinkDel(link)
	}
	return netlink.LinkDel(link)
}

func (m *Manager) nlLinkByName(name string) (netlink.Link, error) {
	if m.netlinkHandle != nil {
		return m.netlinkHandle.LinkByName(name)
	}
	return netlink.LinkByName(name)
}

func (m *Manager) nlLinkList() ([]netlink.Link, error) {
	if m.netlinkHandle != nil {
		return m.netlinkHandle.LinkList()
	}
	return netlink.LinkList()
}

func (m *Manager) createLinuxVRF(name string, tableID uint32) error {
	link := &netlink.Vrf{
		LinkAttrs: netlink.LinkAttrs{Name: name},
		Table:     tableID,
	}

	if err := m.nlLinkAdd(link); err != nil {
		return fmt.Errorf("netlink link add: %w", err)
	}

	if err := m.nlLinkSetUp(link); err != nil {
		m.nlLinkDel(link)
		return fmt.Errorf("netlink link set up: %w", err)
	}

	return nil
}

func (m *Manager) deleteLinuxVRF(name string) {
	link, err := m.nlLinkByName(name)
	if err != nil {
		return
	}
	if err := m.nlLinkDel(link); err != nil {
		m.logger.Warn("Failed to delete Linux VRF device", "name", name, "error", err)
	}
}

func (m *Manager) discoverLinuxVRFs() (map[string]uint32, error) {
	links, err := m.nlLinkList()
	if err != nil {
		return nil, fmt.Errorf("netlink link list: %w", err)
	}

	result := make(map[string]uint32)
	for _, link := range links {
		if v, ok := link.(*netlink.Vrf); ok {
			result[v.Attrs().Name] = v.Table
		}
	}

	return result, nil
}
