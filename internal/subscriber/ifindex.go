// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package subscriber

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/models"
)

// The QoS show views resolve a scheduler's sw_if_index back to the session
// that owns it. The cache has no index in that direction and scanning it per
// lookup would put a full keyspace walk on the 10s telemetry poll, so the
// component keeps an in-memory map maintained at the same points the cache
// entry is written and deleted. Best-effort by design: a session torn down
// while a dump is in flight simply resolves to nothing.
func (c *Component) indexSessionIfIndex(sess models.SubscriberSession) {
	swIfIndex := sess.GetIfIndex()
	sessionID := sess.GetSessionID()
	if swIfIndex == 0 || sessionID == "" {
		return
	}

	c.ifIndexMu.Lock()
	if old, ok := c.ifIndexBySession[sessionID]; ok && old != swIfIndex {
		delete(c.sessionByIfIndex, old)
	}
	c.sessionByIfIndex[swIfIndex] = sessionID
	c.ifIndexBySession[sessionID] = swIfIndex
	c.ifIndexMu.Unlock()
}

func (c *Component) unindexSessionIfIndex(sessionID string) {
	c.ifIndexMu.Lock()
	if swIfIndex, ok := c.ifIndexBySession[sessionID]; ok {
		delete(c.ifIndexBySession, sessionID)
		delete(c.sessionByIfIndex, swIfIndex)
	}
	c.ifIndexMu.Unlock()
}

// SessionIDByIfIndex resolves a per-session interface back to its session ID.
func (c *Component) SessionIDByIfIndex(swIfIndex uint32) (string, bool) {
	c.ifIndexMu.RLock()
	sessionID, ok := c.sessionByIfIndex[swIfIndex]
	c.ifIndexMu.RUnlock()
	return sessionID, ok
}

// warmSessionIfIndex rebuilds the index from the cache after a restart, when
// sessions persisted by a previous process have no lifecycle event to index
// them. One scan at startup; everything after arrives via persistSession.
func (c *Component) warmSessionIfIndex(ctx context.Context) {
	sessions, err := c.GetSessions(ctx, "", "", 0)
	if err != nil {
		c.logger.Warn("Failed to warm session ifindex map", "error", err)
		return
	}
	for _, sess := range sessions {
		c.indexSessionIfIndex(sess)
	}
	if len(sessions) > 0 {
		c.logger.Debug("Warmed session ifindex map", "sessions", len(sessions))
	}
}
