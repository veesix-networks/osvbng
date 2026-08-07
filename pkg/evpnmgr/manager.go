// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package evpnmgr

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/vishvananda/netlink"

	"github.com/veesix-networks/osvbng/pkg/logger"
)

const (
	vxlanPrefix  = "evpn-"
	bridgePrefix = "evbr-"
	vxlanPort    = 4789
)

// Manager maintains non-forwarding kernel vxlan mirror devices for
// EVPN-signaled tunnels. FRR derives its local VNI state from these
// devices via netlink; the real dataplane tunnels live in VPP and are
// programmed separately. Each mirror is a vxlan device enslaved to a
// dedicated empty bridge so zebra detects the VNI while any stray
// kernel-decapped frame is blackholed.
type Manager struct {
	mu            sync.Mutex
	logger        *logger.Logger
	netlinkHandle *netlink.Handle
}

func New() *Manager {
	return &Manager{
		logger: logger.Get(logger.Routing),
	}
}

func (m *Manager) SetNetlinkHandle(h *netlink.Handle) {
	m.netlinkHandle = h
}

func vxlanName(vni uint32) string {
	return fmt.Sprintf("%s%d", vxlanPrefix, vni)
}

func bridgeName(vni uint32) string {
	return fmt.Sprintf("%s%d", bridgePrefix, vni)
}

func (m *Manager) EnsureMirror(vni uint32, vtepIP string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ip := net.ParseIP(vtepIP)
	if ip == nil {
		return fmt.Errorf("invalid vtep ip %q for vni %d", vtepIP, vni)
	}

	if link, err := m.nlLinkByName(vxlanName(vni)); err == nil {
		if vx, ok := link.(*netlink.Vxlan); ok && uint32(vx.VxlanId) == vni && vx.SrcAddr.Equal(ip) {
			return nil
		}
		m.removeMirrorLocked(vni)
	}

	return m.createMirrorLocked(vni, ip)
}

func (m *Manager) RemoveMirror(vni uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeMirrorLocked(vni)
}

func (m *Manager) Reconcile(desired map[uint32]string) error {
	links, err := m.nlLinkList()
	if err != nil {
		return fmt.Errorf("netlink link list: %w", err)
	}

	for _, link := range links {
		name := link.Attrs().Name
		if !strings.HasPrefix(name, vxlanPrefix) {
			continue
		}
		vx, ok := link.(*netlink.Vxlan)
		if !ok {
			continue
		}
		vni := uint32(vx.VxlanId)
		if _, wanted := desired[vni]; !wanted {
			m.RemoveMirror(vni)
			m.logger.Info("Removed stale EVPN mirror device", "vni", vni)
		}
	}

	for vni, vtepIP := range desired {
		if err := m.EnsureMirror(vni, vtepIP); err != nil {
			m.logger.Error("Failed to ensure EVPN mirror device", "vni", vni, "error", err)
		}
	}

	return nil
}

func (m *Manager) createMirrorLocked(vni uint32, vtepIP net.IP) error {
	br := &netlink.Bridge{
		LinkAttrs: netlink.LinkAttrs{Name: bridgeName(vni)},
	}
	if err := m.nlLinkAdd(br); err != nil && !isExists(err) {
		return fmt.Errorf("create mirror bridge for vni %d: %w", vni, err)
	}

	vx := &netlink.Vxlan{
		LinkAttrs: netlink.LinkAttrs{Name: vxlanName(vni)},
		VxlanId:   int(vni),
		SrcAddr:   vtepIP,
		Port:      vxlanPort,
		Learning:  false,
	}
	if err := m.nlLinkAdd(vx); err != nil {
		m.removeMirrorLocked(vni)
		return fmt.Errorf("create mirror vxlan for vni %d: %w", vni, err)
	}

	if err := m.nlLinkSetMaster(vx, br); err != nil {
		m.removeMirrorLocked(vni)
		return fmt.Errorf("enslave mirror vxlan for vni %d: %w", vni, err)
	}

	if err := m.nlLinkSetUp(br); err != nil {
		m.removeMirrorLocked(vni)
		return fmt.Errorf("bring up mirror bridge for vni %d: %w", vni, err)
	}
	if err := m.nlLinkSetUp(vx); err != nil {
		m.removeMirrorLocked(vni)
		return fmt.Errorf("bring up mirror vxlan for vni %d: %w", vni, err)
	}

	m.logger.Info("Created EVPN mirror device", "vni", vni, "vtep_ip", vtepIP.String())
	return nil
}

func (m *Manager) removeMirrorLocked(vni uint32) {
	if link, err := m.nlLinkByName(vxlanName(vni)); err == nil {
		if err := m.nlLinkDel(link); err != nil {
			m.logger.Warn("Failed to delete mirror vxlan device", "vni", vni, "error", err)
		}
	}
	if link, err := m.nlLinkByName(bridgeName(vni)); err == nil {
		if err := m.nlLinkDel(link); err != nil {
			m.logger.Warn("Failed to delete mirror bridge device", "vni", vni, "error", err)
		}
	}
}

func isExists(err error) bool {
	return err != nil && strings.Contains(err.Error(), "exists")
}

func (m *Manager) nlLinkAdd(link netlink.Link) error {
	if m.netlinkHandle != nil {
		return m.netlinkHandle.LinkAdd(link)
	}
	return netlink.LinkAdd(link)
}

func (m *Manager) nlLinkDel(link netlink.Link) error {
	if m.netlinkHandle != nil {
		return m.netlinkHandle.LinkDel(link)
	}
	return netlink.LinkDel(link)
}

func (m *Manager) nlLinkSetUp(link netlink.Link) error {
	if m.netlinkHandle != nil {
		return m.netlinkHandle.LinkSetUp(link)
	}
	return netlink.LinkSetUp(link)
}

func (m *Manager) nlLinkSetMaster(link netlink.Link, master netlink.Link) error {
	if m.netlinkHandle != nil {
		return m.netlinkHandle.LinkSetMaster(link, master)
	}
	return netlink.LinkSetMaster(link, master)
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
