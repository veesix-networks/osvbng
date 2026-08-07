// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package evpnmgr

import (
	"fmt"
	"strings"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"

	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/events"
)

// Start subscribes to AF_BRIDGE neighbor (fdb) events in the given
// netns. zebra expresses each remote VTEP it learns from EVPN type-3
// routes as an all-zero-MAC flood fdb entry on the mirror vxlan
// device; those adds and deletes drive VPP tunnel programming.
// ListExisting replays current entries at subscribe time, so state
// learned while osvbng was down is recovered without a separate dump.
func (m *Manager) Start(ns netns.NsHandle, stop <-chan struct{}) error {
	updates := make(chan netlink.NeighUpdate, 128)

	opts := netlink.NeighSubscribeOptions{
		ListExisting: true,
		ErrorCallback: func(err error) {
			m.logger.Warn("EVPN fdb subscription error", "error", err)
		},
	}
	if ns.IsOpen() {
		opts.Namespace = &ns
	}

	if err := netlink.NeighSubscribeWithOptions(updates, stop, opts); err != nil {
		return fmt.Errorf("subscribe to fdb events: %w", err)
	}

	go func() {
		for {
			select {
			case <-stop:
				return
			case u, ok := <-updates:
				if !ok {
					m.logger.Warn("EVPN fdb subscription channel closed")
					return
				}
				m.handleNeighUpdate(u)
			}
		}
	}()

	m.logger.Info("EVPN remote VTEP watcher started")
	return nil
}

func isFloodEntry(n *netlink.Neigh) bool {
	if n.Family != unix.AF_BRIDGE || n.IP == nil || n.Flags&netlink.NTF_SELF == 0 {
		return false
	}
	for _, b := range n.HardwareAddr {
		if b != 0 {
			return false
		}
	}
	return len(n.HardwareAddr) > 0
}

func (m *Manager) handleNeighUpdate(u netlink.NeighUpdate) {
	if !isFloodEntry(&u.Neigh) {
		return
	}

	link, err := m.nlLinkByIndex(u.LinkIndex)
	if err != nil {
		return
	}
	vx, ok := link.(*netlink.Vxlan)
	if !ok || !strings.HasPrefix(vx.Attrs().Name, vxlanPrefix) {
		return
	}
	vni := uint32(vx.VxlanId)
	vtep := u.IP.String()

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.specs[vni]; !ok {
		return
	}

	switch u.Type {
	case unix.RTM_NEWNEIGH:
		m.addLearnedLocked(vni, vtep)
	case unix.RTM_DELNEIGH:
		m.delLearnedLocked(vni, vtep)
	}
}

func (m *Manager) addLearnedLocked(vni uint32, vtep string) {
	for _, v := range m.learned[vni] {
		if v == vtep {
			return
		}
	}
	m.learned[vni] = append(m.learned[vni], vtep)
	m.logger.Info("EVPN remote VTEP learned", "vni", vni, "vtep", vtep)

	if len(m.learned[vni]) > 1 {
		m.logger.Warn("Multiple remote VTEPs for point-to-point VNI, keeping first", "vni", vni, "vteps", m.learned[vni])
	}
	m.programLocked(vni)
}

func (m *Manager) delLearnedLocked(vni uint32, vtep string) {
	list := m.learned[vni]
	for i, v := range list {
		if v == vtep {
			m.learned[vni] = append(list[:i], list[i+1:]...)
			break
		}
	}
	m.logger.Info("EVPN remote VTEP withdrawn", "vni", vni, "vtep", vtep)

	if m.programmed[vni] == vtep {
		m.unprogramLocked(vni)
		m.programLocked(vni)
	}
}

func (m *Manager) programLocked(vni uint32) {
	list := m.learned[vni]
	if len(list) == 0 {
		return
	}
	dst := list[0]
	if m.programmed[vni] == dst {
		return
	}

	// After an osvbngd restart the programmed map is empty but the VPP
	// tunnel survives; if it already carries the learned dst, adopt it
	// instead of churning the interface under installed sessions.
	spec, ok := m.specs[vni]
	if ok && m.southbound != nil {
		if cur, found := m.southbound.VxlanTunnelDst(spec.Interface); found && cur == dst {
			m.programmed[vni] = dst
			m.logger.Info("Adopted existing EVPN tunnel", "interface", spec.Interface, "vni", vni, "dst", dst)
			return
		}
	}

	m.replaceTunnelLocked(vni, dst)
}

func (m *Manager) unprogramLocked(vni uint32) {
	if _, ok := m.programmed[vni]; !ok {
		return
	}
	m.replaceTunnelLocked(vni, "")
}

// replaceTunnelLocked swaps the tunnel's dst: empty dst reverts to the
// boot-time placeholder (blackhole, transport effectively down). The
// tunnel is recreated because vxlan has no dst-update API; the
// pseudowire mapping is refreshed around it, while the headend's
// output redirect and everything stacked on the headend stay intact.
func (m *Manager) replaceTunnelLocked(vni uint32, dst string) {
	spec, ok := m.specs[vni]
	if !ok || m.southbound == nil {
		return
	}

	if spec.Pseudowire != "" {
		if err := m.southbound.UnbindPseudowire(spec.Pseudowire); err != nil {
			m.logger.Warn("Failed to unbind pseudowire before tunnel replace", "interface", spec.Pseudowire, "transport", spec.Interface, "error", err)
		}
	}
	if err := m.southbound.DeleteInterface(spec.Interface); err != nil {
		m.logger.Error("Failed to delete EVPN tunnel for replace", "interface", spec.Interface, "vni", vni, "error", err)
	}

	cfg := &interfaces.InterfaceConfig{
		Name:    spec.Interface,
		Enabled: true,
		MTU:     spec.MTU,
		Vxlan: &interfaces.VxlanConfig{
			Src:         spec.Src,
			Dst:         dst,
			VNI:         spec.VNI,
			Signaling:   interfaces.VxlanSignalingEVPN,
			PWTransport: spec.Pseudowire != "",
		},
	}
	if err := m.southbound.CreateInterface(cfg); err != nil {
		m.logger.Error("Failed to recreate EVPN tunnel", "interface", spec.Interface, "vni", vni, "dst", dst, "error", err)
		delete(m.programmed, vni)
		return
	}
	mtu := spec.MTU
	if mtu == 0 {
		mtu = interfaces.DefaultMTU
	}
	if err := m.southbound.SetInterfaceMTU(spec.Interface, mtu); err != nil {
		m.logger.Warn("Failed to set EVPN tunnel MTU", "interface", spec.Interface, "error", err)
	}
	if spec.Pseudowire != "" {
		if err := m.southbound.BindPseudowire(spec.Pseudowire, spec.Interface); err != nil {
			m.logger.Error("Failed to rebind pseudowire after tunnel replace", "interface", spec.Pseudowire, "transport", spec.Interface, "error", err)
		}
	}

	if dst == "" {
		delete(m.programmed, vni)
		m.logger.Info("Reverted EVPN tunnel to placeholder", "interface", spec.Interface, "vni", vni)
		return
	}
	m.programmed[vni] = dst
	m.logger.Info("Programmed EVPN tunnel", "interface", spec.Interface, "vni", vni, "dst", dst)

	if m.eventBus != nil {
		m.eventBus.Publish(events.TopicEVPNTunnelProgrammed, events.Event{
			Source: "evpnmgr",
			Data: &events.EVPNTunnelProgrammedEvent{
				Interface: spec.Interface,
				VNI:       vni,
			},
		})
	}
}
