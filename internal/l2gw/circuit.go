// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2gw

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

const (
	circuitStateAuthenticating = "authenticating"
	circuitStateInstalled      = "installed"
	circuitStateRejected       = "rejected"
	circuitStateStandby        = "standby"
)

// Circuit is one wholesale subscriber circuit: the access tuple that
// triggered it, the resolved handoff, and the dataplane identifiers.
// JSON-serialized to opdb for restart survival.
type Circuit struct {
	mu sync.Mutex `json:"-"`

	SessionID     string `json:"session_id"`
	AcctSessionID string `json:"acct_session_id"`
	Username      string `json:"username,omitempty"`
	MAC           string `json:"mac,omitempty"`

	AccessInterface string `json:"access_interface"`
	AccessIfIndex   uint32 `json:"access_if_index"`
	AccessSVLAN     uint16 `json:"access_svlan"`
	AccessCVLAN     uint16 `json:"access_cvlan"`
	AccessTPID      uint16 `json:"access_tpid,omitempty"`

	HandoffGroup     string `json:"handoff_group"`
	HandoffInterface string `json:"handoff_interface"`
	HandoffIfIndex   uint32 `json:"handoff_if_index"`
	HandoffSVLAN     uint16 `json:"handoff_svlan,omitempty"`
	HandoffCVLAN     uint16 `json:"handoff_cvlan,omitempty"`
	HandoffTPID      uint16 `json:"handoff_tpid,omitempty"`
	Transparent      bool   `json:"transparent,omitempty"`
	AllocatedEgress  bool   `json:"allocated_egress,omitempty"`

	Static  bool   `json:"static,omitempty"`
	State   string `json:"state"`
	SRGName string `json:"srg_name,omitempty"`

	// Standby marks an HA-synced circuit installed with forwarding
	// disabled; promotion batch-enables it.
	Standby bool `json:"standby,omitempty"`

	CircuitID         uint32 `json:"circuit_id"`
	AccessEntryIndex  uint32 `json:"access_entry_index"`
	HandoffEntryIndex uint32 `json:"handoff_entry_index"`

	CreatedAt  time.Time         `json:"created_at"`
	Attributes map[string]string `json:"attributes,omitempty"`

	Protocol string `json:"protocol,omitempty"`

	// runtime only
	pendingTrigger *pendingTrigger
	rejectedAt     time.Time
	lastPackets    uint64
	lastActivity   time.Time
}

type pendingTrigger struct {
	protocol  models.Protocol
	srcMAC    net.HardwareAddr
	dstMAC    net.HardwareAddr
	l3Payload []byte
}

func circuitKey(portIfIndex uint32, svlan, cvlan uint16) string {
	return fmt.Sprintf("%d:%d:%d", portIfIndex, svlan, cvlan)
}

func (ct *Circuit) key() string {
	return circuitKey(ct.AccessIfIndex, ct.AccessSVLAN, ct.AccessCVLAN)
}

// install programs the paired circuit into the dataplane and records the
// counter indices.
func (c *Component) installCircuit(ct *Circuit) error {
	id, handoffIdx, err := c.vpp.AddL2GWCircuit(southbound.L2GWCircuit{
		AccessIfIndex:  ct.AccessIfIndex,
		AccessSVLAN:    ct.AccessSVLAN,
		AccessCVLAN:    ct.AccessCVLAN,
		AccessTPID:     ct.AccessTPID,
		HandoffIfIndex: ct.HandoffIfIndex,
		HandoffSVLAN:   ct.HandoffSVLAN,
		HandoffCVLAN:   ct.HandoffCVLAN,
		HandoffTPID:    ct.HandoffTPID,
		Transparent:    ct.Transparent,
		Enabled:        !ct.Standby,
	})
	if err != nil {
		return err
	}
	ct.mu.Lock()
	ct.CircuitID = id
	ct.AccessEntryIndex = id
	ct.HandoffEntryIndex = handoffIdx
	if ct.Standby {
		ct.State = circuitStateStandby
	} else {
		ct.State = circuitStateInstalled
	}
	ct.mu.Unlock()
	return nil
}

func (c *Component) removeCircuit(ct *Circuit) {
	if err := c.vpp.DelL2GWCircuit(southbound.L2GWCircuit{
		AccessIfIndex:  ct.AccessIfIndex,
		AccessSVLAN:    ct.AccessSVLAN,
		AccessCVLAN:    ct.AccessCVLAN,
		HandoffIfIndex: ct.HandoffIfIndex,
		HandoffSVLAN:   ct.HandoffSVLAN,
		HandoffCVLAN:   ct.HandoffCVLAN,
	}); err != nil {
		c.logger.Warn("Failed to delete l2gw circuit from dataplane",
			"circuit_id", ct.CircuitID, "error", err)
	}
}

func (c *Component) checkpointCircuit(ct *Circuit) {
	if c.opdb == nil || ct.Static {
		return
	}
	ct.mu.Lock()
	data, err := json.Marshal(ct)
	ct.mu.Unlock()
	if err != nil {
		c.logger.Error("Failed to marshal l2gw circuit", "error", err)
		return
	}
	if err := c.opdb.Put(context.Background(), opdbNamespace, ct.key(), data); err != nil {
		c.logger.Warn("Failed to checkpoint l2gw circuit", "key", ct.key(), "error", err)
	}
}

func (c *Component) deleteCheckpoint(ct *Circuit) {
	if c.opdb == nil || ct.Static {
		return
	}
	if err := c.opdb.Delete(context.Background(), opdbNamespace, ct.key()); err != nil {
		c.logger.Warn("Failed to delete l2gw circuit checkpoint", "key", ct.key(), "error", err)
	}
}

// buildSessionModel snapshots a circuit into the lifecycle event model.
func (ct *Circuit) buildSessionModel(state models.SessionState) *models.L2GWSession {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	mac, _ := net.ParseMAC(ct.MAC)
	attrs := make(map[string]string, len(ct.Attributes))
	for k, v := range ct.Attributes {
		attrs[k] = v
	}

	return &models.L2GWSession{
		SessionID:         ct.SessionID,
		AAASessionID:      ct.AcctSessionID,
		State:             state,
		Protocol:          ct.Protocol,
		MAC:               mac,
		OuterVLAN:         ct.AccessSVLAN,
		InnerVLAN:         ct.AccessCVLAN,
		AccessIfIndex:     ct.AccessIfIndex,
		AccessInterface:   ct.AccessInterface,
		HandoffGroup:      ct.HandoffGroup,
		HandoffInterface:  ct.HandoffInterface,
		HandoffIfIndex:    ct.HandoffIfIndex,
		HandoffSVLAN:      ct.HandoffSVLAN,
		HandoffCVLAN:      ct.HandoffCVLAN,
		Transparent:       ct.Transparent,
		AccessTPID:        ct.AccessTPID,
		HandoffTPID:       ct.HandoffTPID,
		CircuitID:         ct.CircuitID,
		AccessEntryIndex:  ct.AccessEntryIndex,
		HandoffEntryIndex: ct.HandoffEntryIndex,
		Username:          ct.Username,
		SRGName:           ct.SRGName,
		ActivatedAt:       ct.CreatedAt,
		Attributes:        attrs,
	}
}

// teardownCircuit removes a dynamic circuit end-to-end: dataplane,
// allocator, opdb, indexes, and accounting stop via the Released
// lifecycle event.
func (c *Component) teardownCircuit(ct *Circuit, reason string) {
	c.logger.Info("Tearing down l2gw circuit",
		"session_id", ct.SessionID, "circuit_id", ct.CircuitID, "reason", reason)

	c.removeCircuit(ct)
	c.freeEgress(ct)
	c.deleteCheckpoint(ct)
	c.circuits.Delete(ct.key())
	if ct.SessionID != "" {
		c.sessionIndex.Delete(ct.SessionID)
	}

	if !ct.Static && ct.SessionID != "" {
		c.eventBus.Publish(events.TopicSessionLifecycle, events.Event{
			Source: c.Name(),
			Data: &events.SessionLifecycleEvent{
				AccessType: models.AccessTypeL2GW,
				Protocol:   models.Protocol(ct.Protocol),
				SessionID:  ct.SessionID,
				State:      models.SessionStateReleased,
				Session:    ct.buildSessionModel(models.SessionStateReleased),
			},
		})
	}
}
