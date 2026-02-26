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
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

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

	peerNodeID string
	mu         sync.RWMutex
}

func NewManager(cfg *config.HAConfig, eventBus events.Bus) (*Manager, error) {
	log := logger.Get(logger.HA)

	m := &Manager{
		Base:     component.NewBase("ha"),
		cfg:      cfg,
		srgs:     make(map[string]*SRGStateMachine),
		eventBus: eventBus,
		logger:   log,
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
	if m.eventBus == nil || t == nil {
		return
	}

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

