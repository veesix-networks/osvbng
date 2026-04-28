// Copyright 2025 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	hapb "github.com/veesix-networks/osvbng/api/proto/ha"
	"github.com/veesix-networks/osvbng/pkg/allocator"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/netbind"
	"github.com/veesix-networks/osvbng/pkg/opdb"
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
	RequestGARP(srgName string)
}

type SessionIterator interface {
	ForEachSession(fn func(models.SubscriberSession) bool)
}

type Manager struct {
	*component.Base

	cfg      *config.HAConfig
	server   *grpc.Server
	peer     *PeerClient
	srgs     map[string]*SRGStateMachine
	eventBus events.Bus
	logger   *logger.Logger

	dataplane       southbound.SRGDataplane
	routingCtrl     RoutingController
	ifResolver      InterfaceResolver
	garpCollector   GARPCollector
	ifWatchCallback func(uint32)

	syncSender      *SyncSender
	cgnatSyncSender *CGNATSyncSender
	syncReceiver    *SyncReceiver
	registry        *allocator.Registry
	opdbStore       opdb.Store

	peerSyncSeqs   map[string]uint64
	bulkSyncCounts map[string]*atomic.Uint64

	sessionIterators []SessionIterator

	ifToSRG     map[uint32]string
	ifDownCount map[string]int
	peerNodeID  string
	mu          sync.RWMutex

	garpCancels map[string]context.CancelFunc
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

func WithAllocatorRegistry(r *allocator.Registry) ManagerOption {
	return func(m *Manager) { m.registry = r }
}

func WithOpDB(store opdb.Store) ManagerOption {
	return func(m *Manager) { m.opdbStore = store }
}

func NewManager(cfg *config.HAConfig, eventBus events.Bus, opts ...ManagerOption) (*Manager, error) {
	log := logger.Get(logger.HA)

	bulkCounts := make(map[string]*atomic.Uint64, len(cfg.SRGs))
	for name := range cfg.SRGs {
		bulkCounts[name] = &atomic.Uint64{}
	}

	m := &Manager{
		Base:           component.NewBase("ha"),
		cfg:            cfg,
		srgs:           make(map[string]*SRGStateMachine),
		eventBus:       eventBus,
		logger:         log,
		peerSyncSeqs:   make(map[string]uint64),
		bulkSyncCounts: bulkCounts,
		garpCancels:    make(map[string]context.CancelFunc),
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

	if m.opdbStore != nil {
		m.syncReceiver = NewSyncReceiver(m.opdbStore, m.registry, m.logger)
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

	listenBind, err := m.cfg.Listen.Resolve(addrFamily(m.cfg.GetListenAddress()))
	if err != nil {
		return fmt.Errorf("ha listen binding: %w", err)
	}
	lis, err := netbind.ListenTCP(ctx, "tcp", m.cfg.GetListenAddress(), listenBind)
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

		peerBind, err := m.cfg.Peer.Resolve(addrFamily(m.cfg.Peer.Address))
		if err != nil {
			return fmt.Errorf("ha peer binding: %w", err)
		}
		m.peer = NewPeerClient(m.cfg.Peer.Address, peerBind, dialOpts, m.logger)

		srgNames := make([]string, 0, len(m.srgs))
		for name := range m.srgs {
			srgNames = append(srgNames, name)
		}
		m.syncSender = NewSyncSender(m.peer, m.cfg.GetSyncBacklogSize(), srgNames, m.logger)
		m.eventBus.Subscribe(events.TopicSessionLifecycle, m.syncSender.HandleEvent)
		m.eventBus.Subscribe(events.TopicSubscriberMutationResult, m.syncSender.HandleMutationResult)
		m.Go(func() { m.syncSender.Run(m.Ctx) })

		m.cgnatSyncSender = NewCGNATSyncSender(m.peer, m.cfg.GetSyncBacklogSize(), srgNames, m.logger)
		m.eventBus.Subscribe(events.TopicCGNATMapping, m.cgnatSyncSender.HandleEvent)
		m.Go(func() { m.cgnatSyncSender.Run(m.Ctx) })

		hb := NewHeartbeatLoop(m, m.logger,
			m.cfg.GetHeartbeatInterval(),
			m.cfg.GetHeartbeatTimeout())

		m.Go(hb.Run)
		m.Go(func() {
			m.peer.ConnectWithBackoff()
			if err := m.peer.OpenHeartbeatStream(); err != nil {
				m.logger.Warn("Failed to open heartbeat stream", "error", err)
			}
			m.Go(hb.ReceiveLoop)
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
	if !sm.IsActive() {
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

func (m *Manager) RegisterSessionIterator(iter SessionIterator) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessionIterators = append(m.sessionIterators, iter)
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

func (m *Manager) hasWaitingSRGs() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, sm := range m.srgs {
		if sm.State() == SRGStateWaiting {
			return true
		}
	}
	return false
}

func (m *Manager) RequestSwitchover(ctx context.Context, srgNames []string, force bool) error {
	for _, name := range srgNames {
		sm, ok := m.getSRG(name)
		if !ok {
			continue
		}

		transition := sm.Switchover(force)
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
	for _, s := range msg.SrgStatuses {
		m.peerSyncSeqs[s.SrgName] = s.LastSyncSeq
	}
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

			if transition.NewState == SRGStateStandbyAlone {
				m.mu.RLock()
				downCount := m.ifDownCount[sm.Name]
				m.mu.RUnlock()

				if downCount > 0 {
					m.logger.Warn("Tracked interfaces already down, promoting from STANDBY_ALONE",
						"srg", sm.Name,
						"down_count", downCount)
					if t := sm.TrackerPromote(); t != nil {
						m.publishTransition(t)
					}
				}
			}
		}
	}
}

func (m *Manager) buildHeartbeatMessage() *hapb.HeartbeatMessage {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return &hapb.HeartbeatMessage{
		NodeId:      m.cfg.NodeID,
		TimestampNs: time.Now().UnixNano(),
		SrgStatuses: buildSRGStatuses(m.srgs, m.syncSender, m.syncReceiver),
	}
}

func (m *Manager) publishTransition(t *StateTransition) {
	if t == nil {
		return
	}

	m.driveSync(t)
	m.driveDataplane(t)
	m.driveRouting(t)

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
		m.cancelGarpForSRG(t.SRGName)
		if err := m.dataplane.SetSRGState(t.SRGName, false); err != nil {
			m.logger.Error("Failed to set SRG standby in dataplane", "srg", t.SRGName, "error", err)
		}
	}
}

func (m *Manager) driveRouting(t *StateTransition) {
	if m.routingCtrl == nil {
		return
	}

	srgCfg, ok := m.cfg.SRGs[t.SRGName]
	if !ok || len(srgCfg.Networks) == 0 {
		return
	}

	isActive := t.NewState == SRGStateActive || t.NewState == SRGStateActiveSolo
	wasActive := t.OldState == SRGStateActive || t.OldState == SRGStateActiveSolo

	if isActive && !wasActive {
		if err := m.routingCtrl.AdvertiseSRGNetworks(m.Ctx, srgCfg.Networks); err != nil {
			m.logger.Error("Failed to advertise SRG networks", "srg", t.SRGName, "error", err)
		} else {
			m.logger.Info("SRG networks advertised", "srg", t.SRGName, "networks", len(srgCfg.Networks))
		}
	} else if !isActive && wasActive {
		if err := m.routingCtrl.WithdrawSRGNetworks(m.Ctx, srgCfg.Networks); err != nil {
			m.logger.Error("Failed to withdraw SRG networks", "srg", t.SRGName, "error", err)
		} else {
			m.logger.Info("SRG networks withdrawn", "srg", t.SRGName, "networks", len(srgCfg.Networks))
		}
	}
}

func (m *Manager) driveSync(t *StateTransition) {
	isActive := t.NewState == SRGStateActive || t.NewState == SRGStateActiveSolo
	wasActive := t.OldState == SRGStateActive || t.OldState == SRGStateActiveSolo

	if m.syncSender != nil {
		if isActive && !wasActive {
			m.syncSender.SetActive(true)
		} else if !isActive && wasActive {
			m.syncSender.SetActive(false)
		}
	}

	if m.cgnatSyncSender != nil {
		if isActive && !wasActive {
			m.cgnatSyncSender.SetActive(t.SRGName, true)
		} else if !isActive && wasActive {
			m.cgnatSyncSender.SetActive(t.SRGName, false)
		}
	}

	isStandby := t.NewState == SRGStateStandby
	wasStandby := t.OldState == SRGStateStandby
	if isStandby && !wasStandby && m.peer != nil && m.syncReceiver != nil {
		go m.requestCGNATBulkSync(t.SRGName)
	}

	if m.registry != nil && (t.NewState == SRGStateActive || t.NewState == SRGStateStandby) && t.OldState == SRGStateReady {
		sm, ok := m.getSRG(t.SRGName)
		if ok {
			m.mu.RLock()
			peerNodeID := m.peerNodeID
			m.mu.RUnlock()
			ascending := sm.winsElection(peerNodeID)
			m.registry.SetAllocDirection(ascending)
			m.logger.Info("Allocator direction set", "srg", t.SRGName, "ascending", ascending)
		}
	}
}

func (m *Manager) requestCGNATBulkSync(srgName string) {
	ctx, cancel := context.WithTimeout(m.Ctx, 30*time.Second)
	defer cancel()

	stream, err := m.peer.BulkSyncCGNAT(ctx, &hapb.BulkSyncCGNATRequest{
		SrgNames: []string{srgName},
	})
	if err != nil {
		m.logger.Warn("CGNAT bulk sync request failed", "srg", srgName, "error", err)
		return
	}

	var total int
	for {
		resp, err := stream.Recv()
		if err != nil {
			if err != io.EOF {
				m.logger.Warn("CGNAT bulk sync stream error", "srg", srgName, "error", err)
			}
			break
		}

		if err := m.syncReceiver.HandleBulkSyncCGNATPage(ctx, resp); err != nil {
			m.logger.Error("CGNAT bulk sync page failed", "srg", srgName, "error", err)
			break
		}

		total += len(resp.Mappings)

		if resp.LastPage {
			break
		}
	}

	if total > 0 {
		m.logger.Info("CGNAT bulk sync completed", "srg", srgName, "mappings", total)
	}
}

func (m *Manager) sendGarpForSRG(srgName string) {
	m.cancelGarpForSRG(srgName)

	srgCfg, ok := m.cfg.SRGs[srgName]
	if !ok || !srgCfg.GARP.IsEnabled() {
		return
	}
	if m.dataplane == nil {
		return
	}

	m.logger.Debug("GARP flood scheduled (waiting for session restoration)", "srg", srgName)
}

func (m *Manager) RequestGARP(srgName string) {
	if m.dataplane == nil {
		return
	}
	srgCfg, ok := m.cfg.SRGs[srgName]
	if !ok || !srgCfg.GARP.IsEnabled() {
		return
	}

	m.cancelGarpForSRG(srgName)

	ctx, cancel := context.WithCancel(m.Ctx)
	m.mu.Lock()
	m.garpCancels[srgName] = cancel
	m.mu.Unlock()

	go m.executeGarpFlood(ctx, srgName, srgCfg.GARP)
}

func (m *Manager) cancelGarpForSRG(srgName string) {
	m.mu.Lock()
	if cancel, ok := m.garpCancels[srgName]; ok {
		cancel()
		delete(m.garpCancels, srgName)
	}
	m.mu.Unlock()
}

func (m *Manager) executeGarpFlood(ctx context.Context, srgName string, garpCfg *config.SRGGARPConfig) {
	batchSize := garpCfg.GetBatchSize()
	batchDelay := garpCfg.GetBatchDelay()
	repeatCount := garpCfg.GetRepeatCount()
	repeatInterval := garpCfg.GetRepeatInterval()

	entries := m.collectGarpEntries(srgName)
	if len(entries) == 0 {
		m.logger.Debug("No GARP entries to send", "srg", srgName)
		return
	}

	m.logger.Info("Starting GARP flood", "srg", srgName, "entries", len(entries), "repeats", repeatCount)

	for cycle := 0; cycle < repeatCount; cycle++ {
		if ctx.Err() != nil {
			m.logger.Debug("GARP flood cancelled", "srg", srgName, "cycle", cycle)
			return
		}

		if cycle > 0 {
			select {
			case <-ctx.Done():
				m.logger.Debug("GARP flood cancelled during repeat interval", "srg", srgName)
				return
			case <-time.After(repeatInterval):
			}
		}

		sent := 0
		for i := 0; i < len(entries); i += batchSize {
			if ctx.Err() != nil {
				return
			}

			end := i + batchSize
			if end > len(entries) {
				end = len(entries)
			}

			if err := m.dataplane.SendSRGGarp(srgName, entries[i:end]); err != nil {
				m.logger.Error("Failed to send GARP batch", "srg", srgName, "error", err)
				continue
			}
			sent += end - i

			if end < len(entries) {
				time.Sleep(batchDelay)
			}
		}

		m.logger.Info("GARP flood cycle complete", "srg", srgName, "cycle", cycle+1, "sent", sent)
	}
}

func (m *Manager) collectGarpEntries(srgName string) []southbound.SRGGarpEntry {
	m.mu.RLock()
	iterators := m.sessionIterators
	m.mu.RUnlock()

	if len(iterators) == 0 {
		if m.garpCollector != nil {
			return m.garpCollector(srgName)
		}
		return nil
	}

	var entries []southbound.SRGGarpEntry
	for _, iter := range iterators {
		iter.ForEachSession(func(sess models.SubscriberSession) bool {
			if sess.GetSRGName() != srgName {
				return true
			}
			if sess.GetState() != models.SessionStateActive {
				return true
			}
			ifIdx := sess.GetIfIndex()
			if ifIdx == 0 {
				return true
			}
			if ip := sess.GetIPv4Address(); ip != nil {
				entries = append(entries, southbound.SRGGarpEntry{SwIfIndex: ifIdx, IP: ip})
			}
			if ip := sess.GetIPv6Address(); ip != nil {
				entries = append(entries, southbound.SRGGarpEntry{SwIfIndex: ifIdx, IP: ip})
			}
			return true
		})
	}
	return entries
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

type SyncSRGStatus struct {
	SRGName      string  `json:"srg_name"`
	Role         string  `json:"role"`
	LastSyncSeq  uint64  `json:"last_sync_seq"`
	BacklogDepth int     `json:"backlog_depth"`
	PeerAckedSeq uint64  `json:"peer_acked_seq"`
	SyncLagSecs  float64 `json:"sync_lag_seconds"`
	Creates      uint64  `json:"creates"`
	Updates      uint64  `json:"updates"`
	Deletes      uint64  `json:"deletes"`
	BulkSyncs    uint64  `json:"bulk_syncs"`
}

func (m *Manager) GetSyncStatus() []SyncSRGStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]SyncSRGStatus, 0, len(m.srgs))
	for name, sm := range m.srgs {
		status := SyncSRGStatus{SRGName: name}

		isActive := sm.IsActive()
		if isActive {
			status.Role = "active"
		} else if sm.State() == SRGStateStandby || sm.State() == SRGStateStandbyAlone {
			status.Role = "standby"
		}

		if m.syncSender != nil && isActive {
			status.LastSyncSeq = m.syncSender.GetSeq(name)
			if bl := m.syncSender.GetBacklog(name); bl != nil {
				status.BacklogDepth = bl.Size()
			}
			creates, updates, deletes := m.syncSender.GetCounts(name)
			status.Creates = creates
			status.Updates = updates
			status.Deletes = deletes
			if t := m.syncSender.GetLastSendTime(name); !t.IsZero() {
				status.SyncLagSecs = time.Since(t).Seconds()
			}
		}

		if m.syncReceiver != nil && !isActive {
			status.LastSyncSeq = m.syncReceiver.GetLastSeq(name)
			if t := m.syncReceiver.GetLastRecvTime(name); !t.IsZero() {
				status.SyncLagSecs = time.Since(t).Seconds()
			}
		}

		status.PeerAckedSeq = m.peerSyncSeqs[name]
		if c, ok := m.bulkSyncCounts[name]; ok {
			status.BulkSyncs = c.Load()
		}

		result = append(result, status)
	}
	return result
}

func (m *Manager) IncrementBulkSync(srgName string) {
	if c, ok := m.bulkSyncCounts[srgName]; ok {
		c.Add(1)
	}
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

		if sm.State() == SRGStateStandbyAlone {
			m.logger.Warn("Tracked interface down while STANDBY_ALONE, promoting to ACTIVE_SOLO",
				"srg", srgName,
				"interface", ifEv.Name)
			if t := sm.TrackerPromote(); t != nil {
				m.publishTransition(t)
			}
		}
	} else {
		m.logger.Info("Interface up, SRG priority restored",
			"srg", srgName,
			"interface", ifEv.Name,
			"sw_if_index", ifEv.SwIfIndex,
			"down_count", downCount,
			"effective_priority", sm.Priority())
	}
}

// addrFamily returns the IP family of a host:port string. IPv6 hosts are
// either bracketed ([::1]:50051) or hostless with a v6-shaped form. We
// default to v4 when the form is ambiguous since EndpointBinding source
// fields are family-specific (source_ip vs source_ipv6) and the binding
// will surface a family mismatch during validation.
func addrFamily(addr string) netbind.Family {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		return netbind.FamilyV6
	}
	return netbind.FamilyV4
}

