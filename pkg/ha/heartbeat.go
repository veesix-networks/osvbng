// Copyright 2025 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"log/slog"
	"time"

	hapb "github.com/veesix-networks/osvbng/api/proto/ha"
)

const (
	clockSkewWarnThreshold   = 30 * time.Second
	clockSkewRefuseThreshold = 60 * time.Second
)

type HeartbeatLoop struct {
	manager   *Manager
	logger    *slog.Logger
	interval  time.Duration
	timeout   time.Duration
	seq       uint64
	startedAt time.Time
}

func NewHeartbeatLoop(manager *Manager, logger *slog.Logger, interval, timeout time.Duration) *HeartbeatLoop {
	return &HeartbeatLoop{
		manager:   manager,
		logger:    logger,
		interval:  interval,
		timeout:   timeout,
		startedAt: time.Now(),
	}
}

func (h *HeartbeatLoop) Run() {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	timeoutTicker := time.NewTicker(h.interval)
	defer timeoutTicker.Stop()

	for {
		select {
		case <-h.manager.Ctx.Done():
			return
		case <-ticker.C:
			h.sendHeartbeat()
		case <-timeoutTicker.C:
			h.checkPeerTimeout()
		}
	}
}

func (h *HeartbeatLoop) sendHeartbeat() {
	if h.manager.peer == nil {
		return
	}

	h.seq++
	msg := h.manager.buildHeartbeatMessage()
	msg.Sequence = h.seq

	if err := h.manager.peer.SendHeartbeat(msg); err != nil {
		h.logger.Debug("Failed to send heartbeat", "error", err)
	}
}

func (h *HeartbeatLoop) checkPeerTimeout() {
	if h.manager.peer == nil {
		return
	}

	ps := h.manager.peer.GetState()
	if !ps.Connected {
		if time.Since(h.startedAt) > h.timeout && h.manager.hasWaitingSRGs() {
			h.logger.Warn("Peer not connected and SRGs stuck in WAITING, promoting to ACTIVE_SOLO",
				"timeout", h.timeout)
			h.manager.handlePeerLost()
		}
		return
	}

	if time.Since(ps.LastHeartbeat) > h.timeout {
		h.logger.Warn("Peer heartbeat timeout",
			"last_heartbeat", ps.LastHeartbeat,
			"timeout", h.timeout)

		h.manager.handlePeerLost()
	}

	if absSkew := abs(ps.ClockSkew); absSkew > clockSkewRefuseThreshold {
		h.logger.Error("Peer clock skew exceeds threshold, refusing peering",
			"skew", ps.ClockSkew,
			"threshold", clockSkewRefuseThreshold)
		h.manager.handlePeerLost()
	} else if absSkew > clockSkewWarnThreshold {
		h.logger.Warn("Peer clock skew elevated",
			"skew", ps.ClockSkew,
			"threshold", clockSkewWarnThreshold)
	}
}

func (h *HeartbeatLoop) ReceiveLoop() {
	for {
		select {
		case <-h.manager.Ctx.Done():
			return
		default:
		}

		if h.manager.peer == nil {
			time.Sleep(time.Second)
			continue
		}

		msg, err := h.manager.peer.RecvHeartbeat()
		if err != nil {
			h.logger.Debug("Failed to receive heartbeat", "error", err)
			h.manager.handlePeerLost()

			h.reconnectPeer()
			continue
		}

		h.manager.handlePeerHeartbeat(msg)
	}
}

func (h *HeartbeatLoop) reconnectPeer() {
	select {
	case <-time.After(time.Second):
	case <-h.manager.Ctx.Done():
		return
	}

	h.manager.peer.ConnectWithBackoff()
	if err := h.manager.peer.OpenHeartbeatStream(); err != nil {
		h.logger.Warn("Failed to reopen heartbeat stream", "error", err)
	}
}

func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func buildSRGStatuses(srgs map[string]*SRGStateMachine, sender *SyncSender, receiver *SyncReceiver) []*hapb.SRGStatus {
	statuses := make([]*hapb.SRGStatus, 0, len(srgs))
	for name, sm := range srgs {
		s := &hapb.SRGStatus{
			SrgName:  name,
			State:    string(sm.State()),
			Priority: sm.Priority(),
		}
		if sender != nil {
			s.LastSyncSeq = sender.GetSeq(name)
		}
		if receiver != nil {
			s.LastSyncSeq = receiver.GetLastSeq(name)
		}
		statuses = append(statuses, s)
	}
	return statuses
}
