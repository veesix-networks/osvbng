// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2gw

import (
	"time"
)

const idleSweepInterval = 30 * time.Second

func (c *Component) idleJanitor() {
	ticker := time.NewTicker(idleSweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.Ctx.Done():
			return
		case <-ticker.C:
			c.sweepIdleCircuits()
		}
	}
}

// sweepIdleCircuits tears down installed dynamic circuits that carried
// no traffic in either direction for the group's l2gw idle-timeout.
// Counter deltas from the stats segment are the only liveness signal a
// pure L2 circuit has; this is the lease-substitute for packet-mode
// circuits, which have no DHCP lifecycle to expire them.
func (c *Component) sweepIdleCircuits() {
	now := time.Now()

	type candidate struct {
		ct      *Circuit
		timeout time.Duration
	}
	var candidates []candidate

	c.circuits.Range(func(_, v any) bool {
		ct := v.(*Circuit)
		ct.mu.Lock()
		state := ct.State
		static := ct.Static
		svlan, cvlan := ct.AccessSVLAN, ct.AccessCVLAN
		ct.mu.Unlock()

		if static || state != circuitStateInstalled {
			return true
		}
		match, ok := c.cfgMgr.LookupSubscriberGroup(svlan, cvlan)
		if !ok || match.Group.L2GW == nil || match.Group.L2GW.IdleTimeout == 0 {
			return true
		}
		candidates = append(candidates, candidate{
			ct:      ct,
			timeout: time.Duration(match.Group.L2GW.IdleTimeout) * time.Second,
		})
		return true
	})

	if len(candidates) == 0 {
		return
	}

	stats, err := c.vpp.GetL2GWStats()
	if err != nil {
		c.logger.Warn("Failed to read l2gw stats for idle sweep", "error", err)
		return
	}

	for _, cand := range candidates {
		ct := cand.ct
		ct.mu.Lock()
		packets := stats[ct.AccessEntryIndex].Packets + stats[ct.HandoffEntryIndex].Packets
		if ct.lastActivity.IsZero() || packets != ct.lastPackets {
			ct.lastPackets = packets
			ct.lastActivity = now
			ct.mu.Unlock()
			continue
		}
		idleFor := now.Sub(ct.lastActivity)
		ct.mu.Unlock()

		if idleFor > cand.timeout {
			c.logger.Info("l2gw circuit idle timeout",
				"session_id", ct.SessionID, "circuit_id", ct.CircuitID,
				"idle", idleFor.Round(time.Second).String())
			c.teardownCircuit(ct, "idle-timeout")
		}
	}
}
