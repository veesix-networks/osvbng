// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

// Package l2gw implements the layer 2 wholesale gateway control plane:
// DHCP-triggered, AAA-authorized cross-connects of subscriber circuits
// between access networks and retail ISP handoff ports. osvbng never
// terminates DHCP or L3 for these subscribers, the retail ISP's BNG
// does; osvbng owns circuit steering, egress VLAN allocation, and
// wholesale accounting.
package l2gw

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/ha"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/opdb"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

const opdbNamespace = "l2gw:circuits"

type Component struct {
	*component.Base

	logger   *logger.Logger
	eventBus events.Bus
	cfgMgr   component.ConfigManager
	vpp      southbound.Southbound
	ifMgr    *ifmgr.Manager
	opdb     opdb.Store
	srgMgr   ha.SRGProvider

	// circuits keyed by access tuple "port:svlan:cvlan"; sessionIndex
	// keyed by session ID for terminate/accounting correlation.
	circuits     sync.Map
	sessionIndex sync.Map

	allocMu    sync.Mutex
	allocators map[string]*vlanAllocator

	armedMu    sync.Mutex
	armedPorts map[uint32]bool

	triggerChan       chan *dataplane.ParsedPacket
	packetTriggerChan <-chan *dataplane.ParsedPacket

	aaaSub       events.Subscription
	terminateSub events.Subscription
	haStateSub   events.Subscription
	evpnSub      events.Subscription
}

func New(deps component.Dependencies, srgMgr ha.SRGProvider, ifMgr *ifmgr.Manager) (*Component, error) {
	c := &Component{
		Base:        component.NewBase("l2gw"),
		logger:      logger.Get(logger.L2GW),
		eventBus:    deps.EventBus,
		cfgMgr:      deps.ConfigManager,
		vpp:         deps.Southbound,
		ifMgr:       ifMgr,
		opdb:        deps.OpDB,
		srgMgr:      srgMgr,
		allocators:  make(map[string]*vlanAllocator),
		armedPorts:  make(map[uint32]bool),
		triggerChan: make(chan *dataplane.ParsedPacket, 1024),
	}
	return c, nil
}

// TriggerChan is the write side handed to the IPoE component's DHCP
// dispatch: packets on subscriber groups with access-type l2gw are
// forwarded here instead of being terminated.
func (c *Component) TriggerChan() chan<- *dataplane.ParsedPacket {
	return c.triggerChan
}

// SetPacketTriggerChan wires the dataplane component's dedicated
// packet-mode trigger channel (any-protocol first-frame punts). Must be
// called before Start.
func (c *Component) SetPacketTriggerChan(ch <-chan *dataplane.ParsedPacket) {
	c.packetTriggerChan = ch
}

func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)
	c.logger.Info("Starting L2GW component")

	c.SetReadyState(component.StateRestoring)

	if err := c.restoreCircuits(ctx); err != nil {
		c.logger.Warn("Failed to restore l2gw circuits from OpDB", "error", err)
	}

	if err := c.applyStaticMaps(); err != nil {
		c.logger.Error("Failed to apply l2gw static maps", "error", err)
	}

	c.evpnSub = c.eventBus.Subscribe(events.TopicEVPNTunnelProgrammed, c.handleEVPNTunnelProgrammed)

	if err := c.armTriggers(); err != nil {
		c.logger.Error("Failed to arm l2gw trigger ranges", "error", err)
	}

	if err := c.restoreSyncedStandby(ctx); err != nil {
		c.logger.Warn("Failed to restore HA-synced l2gw circuits", "error", err)
	}

	c.aaaSub = c.eventBus.Subscribe(events.TopicAAAResponseL2GW, c.handleAAAResponse)
	c.terminateSub = c.eventBus.Subscribe(events.TopicSubscriberTerminate, c.handleSubscriberTerminate)
	c.haStateSub = c.eventBus.Subscribe(events.TopicHAStateChange, c.handleHAStateChange)

	c.Go(c.consumeTriggers)
	if c.packetTriggerChan != nil {
		c.Go(c.consumePacketTriggers)
	}
	c.Go(c.janitor)
	c.Go(c.idleJanitor)

	c.SetReadyState(component.StateReady)
	c.eventBus.Publish(events.TopicComponentReady, events.Event{
		Source: c.Name(),
		Data:   &events.ComponentReadyEvent{Component: c.Name(), State: c.ReadyState().String()},
	})

	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.logger.Info("Stopping L2GW component")
	c.SetReadyState(component.StateDraining)

	if c.aaaSub != nil {
		c.aaaSub.Unsubscribe()
	}
	if c.terminateSub != nil {
		c.terminateSub.Unsubscribe()
	}
	if c.haStateSub != nil {
		c.haStateSub.Unsubscribe()
	}
	if c.evpnSub != nil {
		c.evpnSub.Unsubscribe()
	}

	c.StopContext()
	return nil
}

// armTriggers arms the dataplane DHCP trigger snoop for every l2gw
// subscriber-group VLAN range: the access port gets the l2gw-input
// feature and its S-VLANs are registered so circuit-miss DHCP punts to
// the control plane. Runs at Start; the first DISCOVER/SOLICIT on an
// armed S-VLAN is the wholesale circuit trigger.
func (c *Component) armTriggers() error {
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil || cfg.SubscriberGroups == nil {
		return nil
	}

	seen := make(map[string]bool)
	for _, group := range cfg.SubscriberGroups.Groups {
		if group == nil {
			continue
		}
		for _, vr := range group.VLANs {
			if !vr.HasAccessType(subscriber.AccessTypeL2GW) || seen[vr.ParentInterface] {
				continue
			}
			seen[vr.ParentInterface] = true
			if err := c.armInterfaceTriggers(vr.ParentInterface); err != nil {
				return err
			}
		}
	}
	return nil
}

// armInterfaceTriggers arms the l2gw-input feature and every l2gw VLAN
// range parented on ifName, atomically per port: the check-and-set on
// armedPorts makes it safe to invoke from both Start and the EVPN
// tunnel-programmed event without double-arming ranges.
func (c *Component) armInterfaceTriggers(ifName string) error {
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil || cfg.SubscriberGroups == nil {
		return nil
	}

	portIdx, ok := c.ifMgr.GetSwIfIndex(ifName)
	if !ok {
		c.logger.Error("l2gw access interface not found", "interface", ifName)
		return nil
	}

	c.armedMu.Lock()
	if c.armedPorts[portIdx] {
		c.armedMu.Unlock()
		return nil
	}
	c.armedPorts[portIdx] = true
	c.armedMu.Unlock()

	if err := c.vpp.L2GWEnableInput(ifName, true); err != nil {
		c.armedMu.Lock()
		delete(c.armedPorts, portIdx)
		c.armedMu.Unlock()
		return fmt.Errorf("arm access port %s: %w", ifName, err)
	}
	c.logger.Info("Armed l2gw on port", "interface", ifName, "sw_if_index", portIdx)

	for name, group := range cfg.SubscriberGroups.Groups {
		if group == nil {
			continue
		}
		for _, vr := range group.VLANs {
			if vr.ParentInterface != ifName || !vr.HasAccessType(subscriber.AccessTypeL2GW) {
				continue
			}
			svlans, err := vr.GetSVLANs()
			if err != nil {
				return fmt.Errorf("group %s svlan range: %w", name, err)
			}
			anyProtocol := vr.GetTriggerMode() == subscriber.TriggerModePacket
			for _, r := range contiguousRanges(svlans) {
				if err := c.vpp.L2GWTriggerSVLANRange(ifName, r[0], r[1], anyProtocol, true); err != nil {
					return fmt.Errorf("arm trigger svlans %d-%d on %s: %w",
						r[0], r[1], ifName, err)
				}
			}
			c.logger.Info("Armed l2gw trigger ranges",
				"group", name, "interface", ifName,
				"svlans", vr.SVLAN, "trigger", string(vr.GetTriggerMode()))
		}
	}
	return nil
}

// handleEVPNTunnelProgrammed re-arms triggers for VLAN ranges parented
// on an EVPN-signaled tunnel. Those tunnels do not exist when Start's
// armTriggers runs (discovery races component start), and reprogramming
// after a VTEP withdraw recreates the interface with a fresh
// sw_if_index whose features are unset.
func (c *Component) handleEVPNTunnelProgrammed(evt events.Event) {
	data, ok := evt.Data.(*events.EVPNTunnelProgrammedEvent)
	if !ok {
		return
	}

	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil || cfg.SubscriberGroups == nil {
		return
	}

	parented := false
	for _, group := range cfg.SubscriberGroups.Groups {
		if group == nil {
			continue
		}
		for _, vr := range group.VLANs {
			if vr.ParentInterface == data.Interface && vr.HasAccessType(subscriber.AccessTypeL2GW) {
				parented = true
			}
		}
	}
	if !parented {
		return
	}

	if err := c.armInterfaceTriggers(data.Interface); err != nil {
		c.logger.Error("Failed to arm triggers on programmed evpn tunnel", "interface", data.Interface, "error", err)
	}
}

func contiguousRanges(vlans []uint16) [][2]uint16 {
	if len(vlans) == 0 {
		return nil
	}
	sorted := append([]uint16(nil), vlans...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var out [][2]uint16
	lo, hi := sorted[0], sorted[0]
	for _, v := range sorted[1:] {
		if v == hi || v == hi+1 {
			hi = v
			continue
		}
		out = append(out, [2]uint16{lo, hi})
		lo, hi = v, v
	}
	return append(out, [2]uint16{lo, hi})
}

// armPort enables the l2gw-input feature on a port exactly once.
func (c *Component) armPort(swIfIndex uint32, name string) error {
	c.armedMu.Lock()
	defer c.armedMu.Unlock()
	if c.armedPorts[swIfIndex] {
		return nil
	}
	if err := c.vpp.L2GWEnableInput(name, true); err != nil {
		return err
	}
	c.armedPorts[swIfIndex] = true
	c.logger.Info("Armed l2gw on port", "interface", name, "sw_if_index", swIfIndex)
	return nil
}

// resolvePort maps a (possibly sub-)interface index to its parent port
// index and name, the l2gw plugin keys circuits on the port, tags live
// in the packet.
func (c *Component) resolvePort(swIfIndex uint32) (uint32, string) {
	if c.ifMgr == nil {
		return swIfIndex, ""
	}
	iface := c.ifMgr.Get(swIfIndex)
	if iface == nil {
		return swIfIndex, ""
	}
	portIdx := swIfIndex
	if iface.HasParent() {
		portIdx = iface.SupSwIfIndex
	}
	name := ""
	if parent := c.ifMgr.Get(portIdx); parent != nil {
		name = parent.Name
	}
	return portIdx, name
}
