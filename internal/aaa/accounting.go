// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package aaa

import (
	"context"
	"encoding/json"
	"time"

	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/opdb"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

// AccountingCheckpoint is the AAA-owned persisted representation of a
// session's accounting state. Lives in opdb.NamespaceAcctSessions keyed
// by SessionID. Owned and evolved entirely by the AAA component so
// extending the schema (per-CoS counters, Gigawords, additional RADIUS
// attributes) is local to AAA — no coordination with PPPoE / IPoE
// SessionState required.
//
// Field design notes:
//
//   - LastReported* are the cumulative values most recently acknowledged
//     by the RADIUS Accounting server. They are the billing system's
//     source of truth and only advance on Accounting-Response success.
//
//   - CurrentBaseline* are the VPP per-interface counter values at the
//     moment the current baseline was established (initial bind or
//     post-rebaseline after a VPP restart).
//
//   - PriorDelta* carries the cumulative count contributed by previous
//     VPP restart cycles that the live VPP counter pair no longer
//     includes. The Acct-Interim cumulative formula is:
//
//     cumulative = (current_vpp - CurrentBaseline) + PriorDelta
type AccountingCheckpoint struct {
	SessionID       string            `json:"session_id"`
	AcctSessionID   string            `json:"acct_session_id"`
	AccessType      models.AccessType `json:"access_type"`
	Username        string            `json:"username"`
	MAC             string            `json:"mac"`
	IPv4Address     string            `json:"ipv4_address,omitempty"`
	AuthDate        time.Time         `json:"auth_date"`
	SwIfIndex       uint32            `json:"sw_if_index"`
	SVLAN           uint16            `json:"svlan,omitempty"`
	CVLAN           uint16            `json:"cvlan,omitempty"`
	AccessIfIndex   uint32            `json:"access_sw_if_index,omitempty"`
	AccessInterface string            `json:"access_interface,omitempty"`
	Attributes      map[string]string `json:"attributes,omitempty"`

	LastReportedInOctets   uint64 `json:"last_reported_in_octets,omitempty"`
	LastReportedOutOctets  uint64 `json:"last_reported_out_octets,omitempty"`
	LastReportedInPackets  uint64 `json:"last_reported_in_packets,omitempty"`
	LastReportedOutPackets uint64 `json:"last_reported_out_packets,omitempty"`

	CurrentBaselineInBytes    uint64 `json:"current_baseline_in_bytes,omitempty"`
	CurrentBaselineOutBytes   uint64 `json:"current_baseline_out_bytes,omitempty"`
	CurrentBaselineInPackets  uint64 `json:"current_baseline_in_packets,omitempty"`
	CurrentBaselineOutPackets uint64 `json:"current_baseline_out_packets,omitempty"`

	PriorDeltaInBytes    uint64 `json:"prior_delta_in_bytes,omitempty"`
	PriorDeltaOutBytes   uint64 `json:"prior_delta_out_bytes,omitempty"`
	PriorDeltaInPackets  uint64 `json:"prior_delta_in_packets,omitempty"`
	PriorDeltaOutPackets uint64 `json:"prior_delta_out_packets,omitempty"`
}

// checkpointAcctSession writes the in-memory acctCache entry for sessionID
// to opdb. Called after each successful Accounting-Response (LastReported
// advance) and after each VPP-restart rebaseline. Idempotent.
func (c *Component) checkpointAcctSession(s *AccountingSession) {
	if c.opdb == nil || s == nil {
		return
	}
	cp := &AccountingCheckpoint{
		SessionID:       s.sessionID,
		AcctSessionID:   s.acctSessionID,
		AccessType:      s.accessType,
		Username:        s.username,
		MAC:             s.mac,
		IPv4Address:     s.ipv4Address,
		AuthDate:        s.authDate,
		SwIfIndex:       s.swIfIndex,
		SVLAN:           s.svlan,
		CVLAN:           s.cvlan,
		AccessIfIndex:   s.accessIfIndex,
		AccessInterface: s.accessInterface,
		Attributes:      s.attributes,

		LastReportedInOctets:   s.lastReportedInOctets,
		LastReportedOutOctets:  s.lastReportedOutOctets,
		LastReportedInPackets:  s.lastReportedInPackets,
		LastReportedOutPackets: s.lastReportedOutPackets,

		CurrentBaselineInBytes:    s.currentBaselineInBytes,
		CurrentBaselineOutBytes:   s.currentBaselineOutBytes,
		CurrentBaselineInPackets:  s.currentBaselineInPackets,
		CurrentBaselineOutPackets: s.currentBaselineOutPackets,

		PriorDeltaInBytes:    s.priorDeltaInBytes,
		PriorDeltaOutBytes:   s.priorDeltaOutBytes,
		PriorDeltaInPackets:  s.priorDeltaInPackets,
		PriorDeltaOutPackets: s.priorDeltaOutPackets,
	}
	data, err := json.Marshal(cp)
	if err != nil {
		c.logger.Warn("Failed to marshal acct checkpoint",
			"session_id", s.sessionID, "error", err)
		return
	}
	go func() {
		if err := c.opdb.Put(c.Ctx, opdb.NamespaceAcctSessions, s.sessionID, data); err != nil {
			c.logger.Warn("Failed to checkpoint acct session",
				"session_id", s.sessionID, "error", err)
		}
	}()
}

// deleteAcctCheckpoint removes the persisted accounting state for
// sessionID, typically on Accounting-Stop or session teardown.
func (c *Component) deleteAcctCheckpoint(sessionID string) {
	if c.opdb == nil {
		return
	}
	go func() {
		if err := c.opdb.Delete(c.Ctx, opdb.NamespaceAcctSessions, sessionID); err != nil {
			c.logger.Debug("Failed to delete acct checkpoint",
				"session_id", sessionID, "error", err)
		}
	}()
}

// loadAcctSessions walks opdb.NamespaceAcctSessions and seeds acctCache
// from every persisted checkpoint. Called once at Start() before any
// TopicSessionRestored events arrive. Cache entries land with
// pendingSessionConfirm=true; when the corresponding session restore
// publishes TopicSessionRestored, handleSessionRestored marks the entry
// confirmed. Entries that go unconfirmed past pruneAcctOrphansAfter are
// dropped — they represent sessions whose dataplane state did not come
// back (e.g. session was deleted between checkpoints, or expired by
// some other component before the restart).
func (c *Component) loadAcctSessions(ctx context.Context) (int, error) {
	if c.opdb == nil {
		return 0, nil
	}
	var loaded int
	err := c.opdb.Load(ctx, opdb.NamespaceAcctSessions, func(key string, value []byte) error {
		var cp AccountingCheckpoint
		if err := json.Unmarshal(value, &cp); err != nil {
			c.logger.Warn("Failed to unmarshal acct checkpoint",
				"key", key, "error", err)
			return nil
		}
		c.acctCacheMu.Lock()
		c.acctCache[cp.SessionID] = &AccountingSession{
			sessionID:              cp.SessionID,
			acctSessionID:          cp.AcctSessionID,
			accessType:             cp.AccessType,
			authDate:               cp.AuthDate,
			username:               cp.Username,
			mac:                    cp.MAC,
			ipv4Address:            cp.IPv4Address,
			svlan:                  cp.SVLAN,
			cvlan:                  cp.CVLAN,
			accessIfIndex:          cp.AccessIfIndex,
			accessInterface:        cp.AccessInterface,
			attributes:             cp.Attributes,
			swIfIndex:              cp.SwIfIndex,
			pendingSessionConfirm:  true,
			pendingConfirmDeadline: time.Now().Add(pruneAcctOrphansAfter),

			lastReportedInOctets:   cp.LastReportedInOctets,
			lastReportedOutOctets:  cp.LastReportedOutOctets,
			lastReportedInPackets:  cp.LastReportedInPackets,
			lastReportedOutPackets: cp.LastReportedOutPackets,

			currentBaselineInBytes:    cp.CurrentBaselineInBytes,
			currentBaselineOutBytes:   cp.CurrentBaselineOutBytes,
			currentBaselineInPackets:  cp.CurrentBaselineInPackets,
			currentBaselineOutPackets: cp.CurrentBaselineOutPackets,

			priorDeltaInBytes:    cp.PriorDeltaInBytes,
			priorDeltaOutBytes:   cp.PriorDeltaOutBytes,
			priorDeltaInPackets:  cp.PriorDeltaInPackets,
			priorDeltaOutPackets: cp.PriorDeltaOutPackets,
		}
		c.acctCacheMu.Unlock()
		loaded++
		return nil
	})
	return loaded, err
}

// pruneAcctOrphansAfter is the maximum time a cache entry can remain
// pendingSessionConfirm before being dropped. Sized to comfortably
// exceed the longest restoreSessions cycle (10k sessions × ~1ms each =
// ~10s) plus headroom for slow VPP API responses.
const pruneAcctOrphansAfter = 5 * time.Minute

// pruneOrphanedAcctEntries drops any acctCache entry that was loaded
// from opdb but never confirmed by a TopicSessionRestored emission past
// its pendingConfirmDeadline. Run periodically from a background
// goroutine; a missed confirmation almost always means the session was
// deleted out-of-band between checkpoints and the dataplane state
// rightly never came back.
func (c *Component) pruneOrphanedAcctEntries(now time.Time) int {
	c.acctCacheMu.Lock()
	defer c.acctCacheMu.Unlock()

	var pruned int
	for id, s := range c.acctCache {
		if !s.pendingSessionConfirm {
			continue
		}
		if now.Before(s.pendingConfirmDeadline) {
			continue
		}
		delete(c.acctCache, id)
		pruned++
		c.deleteAcctCheckpoint(id)
		c.logger.Info("Pruned orphaned acct cache entry",
			"session_id", id,
			"username", s.username,
			"acct_session_id", s.acctSessionID)
	}
	return pruned
}

// applyVPPCounters folds the latest snapshot for the session's
// dataplane interface into the persisted baseline / prior-delta scheme
// and returns the cumulative-from-session-start values for the four
// RADIUS accounting octet / packet attributes.
//
// On a counter regress (current < CurrentBaseline) the underlying VPP
// interface has been recreated or renumbered. Pre-regress traffic is
// folded into PriorDelta from LastReported, the new live counter
// becomes CurrentBaseline, and the next cumulative continues from
// LastReported without a billing dip.
//
// Caller must hold s.mu.
func (s *AccountingSession) applyVPPCounters(stats *southbound.InterfaceStats) (rxBytes, txBytes, rxPackets, txPackets uint64) {
	regressed := stats.RxBytes < s.currentBaselineInBytes ||
		stats.TxBytes < s.currentBaselineOutBytes ||
		stats.Rx < s.currentBaselineInPackets ||
		stats.Tx < s.currentBaselineOutPackets

	if regressed {
		s.priorDeltaInBytes = s.lastReportedInOctets
		s.priorDeltaOutBytes = s.lastReportedOutOctets
		s.priorDeltaInPackets = s.lastReportedInPackets
		s.priorDeltaOutPackets = s.lastReportedOutPackets

		// VPP restart zeros the per-interface counter; the live value the
		// stats segment is reporting was accumulated from a fresh start.
		// Anchoring the new baseline at zero lets the next interim continue
		// from LastReported + current rather than stalling at LastReported
		// until the dataplane re-crosses the pre-regress level.
		s.currentBaselineInBytes = 0
		s.currentBaselineOutBytes = 0
		s.currentBaselineInPackets = 0
		s.currentBaselineOutPackets = 0
	}

	rxBytes = (stats.RxBytes - s.currentBaselineInBytes) + s.priorDeltaInBytes
	txBytes = (stats.TxBytes - s.currentBaselineOutBytes) + s.priorDeltaOutBytes
	rxPackets = (stats.Rx - s.currentBaselineInPackets) + s.priorDeltaInPackets
	txPackets = (stats.Tx - s.currentBaselineOutPackets) + s.priorDeltaOutPackets
	return
}

// advanceLastReported records that the billing server has acknowledged
// these cumulative values, so subsequent rebaseline branches preserve
// them.
//
// Caller must hold s.mu.
func (s *AccountingSession) advanceLastReported(rxBytes, txBytes, rxPackets, txPackets uint64) {
	s.lastReportedInOctets = rxBytes
	s.lastReportedOutOctets = txBytes
	s.lastReportedInPackets = rxPackets
	s.lastReportedOutPackets = txPackets
}
