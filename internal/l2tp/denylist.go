// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"net"
	"sync"
	"time"
)

// PeerDenylist tracks LNS peers the LAC has temporarily given up on.
// An entry expires after its TTL — at which point the peer becomes
// "probe-eligible": the LAC may try it again, but the entry is not
// removed until a fresh success clears it. This matches the Nokia
// denylist semantics described in `references/l2tp/nokia-sr-l2tp.md`
// and the spec-finalize C6 deterministic-fallback contract.
//
// Lookups are O(1) on the peer IP. Cleanup is lazy — entries that have
// expired stay in the map until either ProbeEligible / IsDenied is
// called on them (which returns the right answer) or Remove evicts
// them. A scheduled sweep is unnecessary because the map is bounded
// by the number of distinct LNS endpoints, not by call volume.
type PeerDenylist struct {
	mu      sync.Mutex
	entries map[string]denylistEntry
}

type denylistEntry struct {
	reason   string
	expires  time.Time
	addedAt  time.Time
}

// NewPeerDenylist returns a fresh empty denylist.
func NewPeerDenylist() *PeerDenylist {
	return &PeerDenylist{entries: make(map[string]denylistEntry)}
}

// Add records `peerIP` as denylisted for `ttl` with the given reason.
// A subsequent Add on the same peer resets the TTL and reason.
func (d *PeerDenylist) Add(peerIP net.IP, reason string, ttl time.Duration) {
	if peerIP == nil || ttl <= 0 {
		return
	}
	now := time.Now()
	d.mu.Lock()
	d.entries[peerIP.String()] = denylistEntry{
		reason:  reason,
		expires: now.Add(ttl),
		addedAt: now,
	}
	d.mu.Unlock()
}

// Remove clears the denylist entry for `peerIP`. Called on a successful
// SCCRP from a previously-denylisted peer so that future failures
// don't compound against stale state.
func (d *PeerDenylist) Remove(peerIP net.IP) {
	if peerIP == nil {
		return
	}
	d.mu.Lock()
	delete(d.entries, peerIP.String())
	d.mu.Unlock()
}

// IsDenied reports whether `peerIP` currently sits inside an unexpired
// denylist entry. Once the TTL has passed the peer is no longer denied
// (but the entry remains for ProbeEligible bookkeeping).
func (d *PeerDenylist) IsDenied(peerIP net.IP) bool {
	if peerIP == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	e, ok := d.entries[peerIP.String()]
	if !ok {
		return false
	}
	return time.Now().Before(e.expires)
}

// ProbeEligible reports whether `peerIP` is on the denylist but its
// TTL has expired — meaning the LAC may try it again as a probe.
// Returns false both for peers never denylisted and peers whose TTL
// is still active.
func (d *PeerDenylist) ProbeEligible(peerIP net.IP) bool {
	if peerIP == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	e, ok := d.entries[peerIP.String()]
	if !ok {
		return false
	}
	return !time.Now().Before(e.expires)
}

// Reason returns the reason string recorded at the most recent Add,
// or empty if the peer is not on the denylist.
func (d *PeerDenylist) Reason(peerIP net.IP) string {
	if peerIP == nil {
		return ""
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if e, ok := d.entries[peerIP.String()]; ok {
		return e.reason
	}
	return ""
}

// Len returns the number of entries currently tracked. Includes
// expired (probe-eligible) entries.
func (d *PeerDenylist) Len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.entries)
}
