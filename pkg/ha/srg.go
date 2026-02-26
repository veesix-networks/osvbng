// Copyright 2025 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/config"
)

type SRGState string

const (
	SRGStateInit       SRGState = "INIT"
	SRGStateWaiting    SRGState = "WAITING"
	SRGStateReady      SRGState = "READY"
	SRGStateActive     SRGState = "ACTIVE"
	SRGStateStandby    SRGState = "STANDBY"
	SRGStateActiveSolo SRGState = "ACTIVE_SOLO"
)

type SRGStateMachine struct {
	Name             string
	cfg              *config.SRGConfig
	state            SRGState
	virtualMAC       net.HardwareAddr
	subscriberGroups map[string]bool

	peerPriority uint32
	peerState    SRGState
	localNodeID  string

	lastTransition time.Time
	mu             sync.RWMutex
}

func NewSRGStateMachine(name string, cfg *config.SRGConfig, localNodeID string) (*SRGStateMachine, error) {
	var vmac net.HardwareAddr
	if cfg.VirtualMAC != "" {
		var err error
		vmac, err = net.ParseMAC(cfg.VirtualMAC)
		if err != nil {
			return nil, fmt.Errorf("invalid virtual MAC for SRG %s: %w", name, err)
		}
	}

	groups := make(map[string]bool, len(cfg.SubscriberGroups))
	for _, g := range cfg.SubscriberGroups {
		groups[g] = true
	}

	return &SRGStateMachine{
		Name:             name,
		cfg:              cfg,
		state:            SRGStateInit,
		virtualMAC:       vmac,
		subscriberGroups: groups,
		localNodeID:      localNodeID,
		lastTransition:   time.Now(),
	}, nil
}

func (sm *SRGStateMachine) State() SRGState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.state
}

func (sm *SRGStateMachine) Priority() uint32 {
	return sm.cfg.Priority
}

func (sm *SRGStateMachine) Preempt() bool {
	return sm.cfg.Preempt
}

func (sm *SRGStateMachine) VirtualMAC() net.HardwareAddr {
	return sm.virtualMAC
}

func (sm *SRGStateMachine) OwnsSubscriberGroup(groupName string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.subscriberGroups[groupName]
}

func (sm *SRGStateMachine) SubscriberGroups() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	groups := make([]string, 0, len(sm.subscriberGroups))
	for g := range sm.subscriberGroups {
		groups = append(groups, g)
	}
	return groups
}

func (sm *SRGStateMachine) IsActive() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.state == SRGStateActive || sm.state == SRGStateActiveSolo
}

func (sm *SRGStateMachine) LastTransition() time.Time {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.lastTransition
}

type StateTransition struct {
	SRGName  string
	OldState SRGState
	NewState SRGState
}

func (sm *SRGStateMachine) transitionTo(newState SRGState) *StateTransition {
	old := sm.state
	if old == newState {
		return nil
	}
	sm.state = newState
	sm.lastTransition = time.Now()
	return &StateTransition{
		SRGName:  sm.Name,
		OldState: old,
		NewState: newState,
	}
}

func (sm *SRGStateMachine) Start() *StateTransition {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.state != SRGStateInit {
		return nil
	}
	return sm.transitionTo(SRGStateWaiting)
}

func (sm *SRGStateMachine) PeerDiscovered(peerPriority uint32, peerNodeID string, peerState SRGState) *StateTransition {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.peerPriority = peerPriority
	sm.peerState = peerState

	switch sm.state {
	case SRGStateWaiting:
		return sm.transitionTo(SRGStateReady)
	case SRGStateActiveSolo:
		return sm.transitionTo(SRGStateReady)
	default:
		return nil
	}
}

func (sm *SRGStateMachine) Elect(peerNodeID string) *StateTransition {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.state != SRGStateReady {
		return nil
	}

	if sm.winsElection(peerNodeID) {
		return sm.transitionTo(SRGStateActive)
	}
	return sm.transitionTo(SRGStateStandby)
}

func (sm *SRGStateMachine) winsElection(peerNodeID string) bool {
	if sm.cfg.Priority != sm.peerPriority {
		return sm.cfg.Priority > sm.peerPriority
	}
	return sm.localNodeID < peerNodeID
}

func (sm *SRGStateMachine) PeerLost() *StateTransition {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	switch sm.state {
	case SRGStateActive:
		return sm.transitionTo(SRGStateActiveSolo)
	case SRGStateStandby:
		return sm.transitionTo(SRGStateActive)
	case SRGStateReady:
		return sm.transitionTo(SRGStateWaiting)
	default:
		return nil
	}
}

func (sm *SRGStateMachine) Switchover() *StateTransition {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	switch sm.state {
	case SRGStateActive:
		return sm.transitionTo(SRGStateStandby)
	case SRGStateStandby:
		return sm.transitionTo(SRGStateActive)
	default:
		return nil
	}
}

func (sm *SRGStateMachine) PeerHeartbeatUpdate(peerPriority uint32, peerNodeID string, peerState SRGState) *StateTransition {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.peerPriority = peerPriority
	sm.peerState = peerState

	if sm.cfg.Preempt && sm.state == SRGStateStandby && sm.winsElection(peerNodeID) {
		return sm.transitionTo(SRGStateActive)
	}

	return nil
}
