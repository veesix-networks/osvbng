// Copyright 2025 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	hapb "github.com/veesix-networks/osvbng/api/proto/ha"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type InterfaceResolver func(name string) (uint32, error)

type GARPCollector func(srgName string) []southbound.SRGGarpEntry

type ManagerOption func(*Manager)

type SRGProvider interface {
	GetVirtualMAC(srgName string) net.HardwareAddr
	IsActive(srgName string) bool
	GetSRGForGroup(subscriberGroup string) string
}

type Manager struct {
	*component.Base

	cfg      *config.HAConfig
	server   *grpc.Server
	peer     *PeerClient
	srgs     map[string]*SRGStateMachine
	eventBus events.Bus
	logger   *slog.Logger

	dataplane       southbound.SRGDataplane
	ifResolver      InterfaceResolver
	garpCollector   GARPCollector
	ifWatchCallback func(uint32)

	ifToSRG     map[uint32]string
	ifDownCount map[string]int
	peerNodeID  string
	mu          sync.RWMutex
}

func WithSRGDataplane(dp southbound.SRGDataplane) ManagerOption {
	return func(m *Manager) { m.dataplane = dp }
}

func WithInterfaceResolver(fn InterfaceResolver) ManagerOption {
	return func(m *Manager) { m.ifResolver = fn }
}

func WithGARPCollector(fn GARPCollector) ManagerOption {
	return func(m *Manager) { m.garpCollector = fn }
}

func WithInterfaceWatchCallback(fn func(uint32)) ManagerOption {
	return func(m *Manager) { m.ifWatchCallback = fn }
}

func NewManager(cfg *config.HAConfig, eventBus events.Bus, opts ...ManagerOption) (*Manager, error) {
	log := logger.Get(logger.HA)

	m := &Manager{
		Base:     component.NewBase("ha"),
		cfg:      cfg,
		srgs:     make(map[string]*SRGStateMachine),
		eventBus: eventBus,
		logger:   log,
	}

	for _, opt := range opts {
		opt(m)
	}

	for name, srgCfg := range cfg.SRGs {
		sm, err := NewSRGStateMachine(name, srgCfg, cfg.NodeID)
		if err != nil {
			return nil, err
		}
		m.srgs[name] = sm
	}

	return m, nil
}

func (m *Manager) Start(ctx context.Context) error {
	m.StartContext(ctx)
	m.logger.Info("Starting HA manager",
		"node_id", m.cfg.NodeID,
		"listen", m.cfg.GetListenAddress(),
		"peer", m.cfg.Peer.Address,
		"srgs", len(m.srgs))

	for _, sm := range m.srgs {
		if t := sm.Start(); t != nil {
			m.publishTransition(t)
		}
	}

	m.registerSRGsWithDataplane()
	m.buildInterfaceMap()

	m.eventBus.Subscribe(events.TopicInterfaceState, func(ev events.Event) {
		m.handleInterfaceEvent(ev)
	})

	var serverOpts []grpc.ServerOption
	if m.cfg.TLS.CACert != "" {
		creds, err := loadServerTLS(m.cfg.TLS)
		if err != nil {
			return fmt.Errorf("load server TLS: %w", err)
		}
		serverOpts = append(serverOpts, grpc.Creds(creds))
		m.logger.Info("HA gRPC server using mTLS")
	} else {
		m.logger.Warn("HA gRPC server running without TLS")
	}

	m.server = grpc.NewServer(serverOpts...)
	hapb.RegisterHAPeerServiceServer(m.server, NewHAPeerServer(m, m.logger))

	lis, err := net.Listen("tcp", m.cfg.GetListenAddress())
	if err != nil {
		return err
	}

	m.Go(func() {
		if err := m.server.Serve(lis); err != nil {
			m.logger.Error("gRPC server error", "error", err)
		}
	})

	if m.cfg.Peer.Address != "" {
		var dialOpts []grpc.DialOption
		if m.cfg.TLS.CACert != "" {
			creds, err := loadClientTLS(m.cfg.TLS)
			if err != nil {
				return fmt.Errorf("load client TLS: %w", err)
			}
			dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
		} else {
			dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		}

		m.peer = NewPeerClient(m.cfg.Peer.Address, dialOpts, m.logger)

		hb := NewHeartbeatLoop(m, m.logger,
			m.cfg.GetHeartbeatInterval(),
			m.cfg.GetHeartbeatTimeout())

		m.Go(func() {
			m.peer.ConnectWithBackoff()
			if err := m.peer.OpenHeartbeatStream(); err != nil {
				m.logger.Warn("Failed to open heartbeat stream", "error", err)
			}
			m.Go(hb.ReceiveLoop)
			hb.Run()
		})
	}

	return nil
}

func (m *Manager) Stop(ctx context.Context) error {
	m.logger.Info("Stopping HA manager")

	if m.dataplane != nil {
		for name := range m.srgs {
			if err := m.dataplane.DelSRG(name); err != nil {
				m.logger.Warn("Failed to deregister SRG from dataplane", "srg", name, "error", err)
			}
		}
	}

	if m.server != nil {
		m.server.GracefulStop()
	}

	if m.peer != nil {
		_ = m.peer.Close()
	}

	m.StopContext()
	return nil
}

func (m *Manager) GetVirtualMAC(srgName string) net.HardwareAddr {
	if srgName == "" {
		return nil
	}
	sm, ok := m.getSRG(srgName)
	if !ok {
		return nil
	}
	return sm.VirtualMAC()
}

func (m *Manager) IsActive(srgName string) bool {
	if srgName == "" {
		return true
	}
	sm, ok := m.getSRG(srgName)
	if !ok {
		return true
	}
	return sm.IsActive()
}

func (m *Manager) GetSRGForGroup(subscriberGroup string) string {
	if subscriberGroup == "" {
		return ""
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for name, sm := range m.srgs {
		if sm.OwnsSubscriberGroup(subscriberGroup) {
			return name
		}
	}
	return ""
}

func (m *Manager) GetSRGs() map[string]*SRGStateMachine {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*SRGStateMachine, len(m.srgs))
	for k, v := range m.srgs {
		result[k] = v
	}
	return result
}

func (m *Manager) GetPeerState() PeerState {
	if m.peer == nil {
		return PeerState{}
	}
	return m.peer.GetState()
}

func (m *Manager) GetNodeID() string {
	return m.cfg.NodeID
}

func (m *Manager) getSRG(name string) (*SRGStateMachine, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sm, ok := m.srgs[name]
	return sm, ok
}

func (m *Manager) RequestSwitchover(ctx context.Context, srgNames []string) error {
	for _, name := range srgNames {
		sm, ok := m.getSRG(name)
		if !ok {
			continue
		}

		transition := sm.Switchover()
		if transition != nil {
			m.publishTransition(transition)
			m.logger.Info("Local switchover",
				"srg", name,
				"old_state", string(transition.OldState),
				"new_state", string(transition.NewState))
		}
	}

	if m.peer != nil {
		_, err := m.peer.RequestSwitchover(ctx, &hapb.SwitchoverRequest{
			SrgNames: srgNames,
			Graceful: true,
		})
		if err != nil {
			m.logger.Warn("Failed to notify peer of switchover", "error", err)
			return err
		}
	}

	return nil
}

func (m *Manager) handlePeerHeartbeat(msg *hapb.HeartbeatMessage) {
	m.mu.Lock()
	firstContact := m.peerNodeID == ""
	m.peerNodeID = msg.NodeId
	m.mu.Unlock()

	peerSRGStates := make(map[string]*hapb.SRGStatus)
	for _, s := range msg.SrgStatuses {
		peerSRGStates[s.SrgName] = s
	}

	for name, sm := range m.srgs {
		peerStatus, hasPeer := peerSRGStates[name]
		if !hasPeer {
			continue
		}

		if firstContact || sm.State() == SRGStateWaiting || sm.State() == SRGStateActiveSolo {
			transition := sm.PeerDiscovered(peerStatus.Priority, msg.NodeId, SRGState(peerStatus.State))
			if transition != nil {
				m.publishTransition(transition)
			}

			if sm.State() == SRGStateReady {
				transition := sm.Elect(msg.NodeId)
				if transition != nil {
					m.publishTransition(transition)
					m.logger.Info("SRG election completed",
						"srg", name,
						"result", string(transition.NewState),
						"local_priority", sm.Priority(),
						"peer_priority", peerStatus.Priority)
				}
			}
		} else {
			transition := sm.PeerHeartbeatUpdate(peerStatus.Priority, msg.NodeId, SRGState(peerStatus.State))
			if transition != nil {
				m.publishTransition(transition)
			}
		}
	}
}

func (m *Manager) handlePeerLost() {
	m.mu.Lock()
	m.peerNodeID = ""
	m.mu.Unlock()

	for _, sm := range m.srgs {
		transition := sm.PeerLost()
		if transition != nil {
			m.publishTransition(transition)
			m.logger.Warn("SRG peer lost",
				"srg", sm.Name,
				"old_state", string(transition.OldState),
				"new_state", string(transition.NewState))
		}
	}
}

func (m *Manager) buildHeartbeatMessage() *hapb.HeartbeatMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return &hapb.HeartbeatMessage{
		NodeId:      m.cfg.NodeID,
		TimestampNs: time.Now().UnixNano(),
		SrgStatuses: buildSRGStatuses(m.srgs),
	}
}

func (m *Manager) publishTransition(t *StateTransition) {
	if t == nil {
		return
	}

	m.driveDataplane(t)

	if m.eventBus != nil {
		m.eventBus.Publish(events.TopicHAStateChange, events.Event{
			Type:      "ha.srg.state_change",
			Timestamp: time.Now(),
			Source:    "ha",
			Data: events.HAStateChangeEvent{
				SRGName:  t.SRGName,
				OldState: string(t.OldState),
				NewState: string(t.NewState),
			},
		})
	}
}

func (m *Manager) driveDataplane(t *StateTransition) {
	if m.dataplane == nil {
		return
	}

	isActive := t.NewState == SRGStateActive || t.NewState == SRGStateActiveSolo
	wasActive := t.OldState == SRGStateActive || t.OldState == SRGStateActiveSolo

	if isActive && !wasActive {
		if err := m.dataplane.SetSRGState(t.SRGName, true); err != nil {
			m.logger.Error("Failed to set SRG active in dataplane", "srg", t.SRGName, "error", err)
		}
		m.sendGarpForSRG(t.SRGName)
	} else if !isActive && wasActive {
		if err := m.dataplane.SetSRGState(t.SRGName, false); err != nil {
			m.logger.Error("Failed to set SRG standby in dataplane", "srg", t.SRGName, "error", err)
		}
	}
}

func (m *Manager) sendGarpForSRG(srgName string) {
	if m.garpCollector == nil {
		return
	}
	entries := m.garpCollector(srgName)
	if len(entries) == 0 {
		return
	}
	if err := m.dataplane.SendSRGGarp(srgName, entries); err != nil {
		m.logger.Error("Failed to send GARP flood", "srg", srgName, "entries", len(entries), "error", err)
	} else {
		m.logger.Info("GARP flood sent", "srg", srgName, "entries", len(entries))
	}
}

func (m *Manager) registerSRGsWithDataplane() {
	if m.dataplane == nil {
		return
	}

	for name, sm := range m.srgs {
		mac := sm.VirtualMAC()
		if mac == nil {
			continue
		}

		swIfIndices := m.resolveInterfaces(name)
		if err := m.dataplane.AddSRG(name, mac, swIfIndices); err != nil {
			m.logger.Error("Failed to register SRG with dataplane", "srg", name, "error", err)
		} else {
			m.logger.Info("Registered SRG with dataplane", "srg", name, "interfaces", len(swIfIndices))
		}
	}
}

func (m *Manager) resolveInterfaces(srgName string) []uint32 {
	if m.ifResolver == nil {
		return nil
	}

	srgCfg, ok := m.cfg.SRGs[srgName]
	if !ok {
		return nil
	}

	var indices []uint32
	for _, ifName := range srgCfg.Interfaces {
		idx, err := m.ifResolver(ifName)
		if err != nil {
			m.logger.Warn("Failed to resolve interface for SRG", "srg", srgName, "interface", ifName, "error", err)
			continue
		}
		indices = append(indices, idx)
	}
	return indices
}

func (m *Manager) GetSRGDataplane() southbound.SRGDataplane {
	return m.dataplane
}

func (m *Manager) GetInterfaceDownCounts() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]int, len(m.ifDownCount))
	for k, v := range m.ifDownCount {
		result[k] = v
	}
	return result
}

func (m *Manager) GetTrackedInterfaceCount(srgName string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, name := range m.ifToSRG {
		if name == srgName {
			count++
		}
	}
	return count
}

func (m *Manager) buildInterfaceMap() {
	m.ifToSRG = make(map[uint32]string)
	m.ifDownCount = make(map[string]int)
	if m.ifResolver == nil {
		return
	}

	for name, srgCfg := range m.cfg.SRGs {
		if srgCfg.TrackPriorityDecrement == 0 {
			continue
		}
		for _, ifName := range srgCfg.Interfaces {
			idx, err := m.ifResolver(ifName)
			if err != nil {
				m.logger.Warn("Failed to resolve tracked interface",
					"srg", name, "interface", ifName, "error", err)
				continue
			}
			m.ifToSRG[idx] = name
			if m.ifWatchCallback != nil {
				m.ifWatchCallback(idx)
			}
		}
	}

	if len(m.ifToSRG) > 0 {
		m.logger.Info("Interface tracking enabled", "tracked_interfaces", len(m.ifToSRG))
	}
}

func (m *Manager) handleInterfaceEvent(ev events.Event) {
	ifEv, ok := ev.Data.(events.InterfaceStateEvent)
	if !ok {
		return
	}

	srgName, tracked := m.ifToSRG[ifEv.SwIfIndex]
	if !tracked {
		return
	}

	sm, ok := m.getSRG(srgName)
	if !ok {
		return
	}

	srgCfg, ok := m.cfg.SRGs[srgName]
	if !ok {
		return
	}

	m.mu.Lock()
	wasDown := m.ifDownCount[srgName]
	if !ifEv.LinkUp || ifEv.Deleted {
		m.ifDownCount[srgName]++
	} else if wasDown > 0 {
		m.ifDownCount[srgName]--
	}
	downCount := m.ifDownCount[srgName]
	m.mu.Unlock()

	delta := -int32(srgCfg.TrackPriorityDecrement) * int32(downCount)
	sm.AdjustPriority(delta)

	if !ifEv.LinkUp || ifEv.Deleted {
		m.logger.Warn("Interface down, SRG priority decremented",
			"srg", srgName,
			"interface", ifEv.Name,
			"sw_if_index", ifEv.SwIfIndex,
			"down_count", downCount,
			"effective_priority", sm.Priority())
	} else {
		m.logger.Info("Interface up, SRG priority restored",
			"srg", srgName,
			"interface", ifEv.Name,
			"sw_if_index", ifEv.SwIfIndex,
			"down_count", downCount,
			"effective_priority", sm.Priority())
	}
}

