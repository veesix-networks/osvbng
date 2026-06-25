// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package pppoe

import (
	"fmt"
	"hash/fnv"
	"net"
	"time"

	"github.com/veesix-networks/osvbng/internal/ra"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/ppp"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

const (
	raTickInterval   = time.Second
	raMinBucketCount = 10
	raMaxBucketCount = 600
)

// computeRABucketCount sizes the emitter wheel so a session is visited within
// the shortest resolved refresh interval across RA-advertising groups, keeping
// the per-tick batch bounded (one bucket walked per second).
func (c *Component) computeRABucketCount(cfg *config.Config) int {
	minRefresh := raMaxBucketCount
	if cfg != nil && cfg.SubscriberGroups != nil {
		for _, g := range cfg.SubscriberGroups.Groups {
			if g == nil || g.IPv6Profile == "" {
				continue
			}
			rc, _ := ra.ResolveGroupRA(cfg, g)
			if ri := int(ra.RefreshInterval(rc) / time.Second); ri > 0 && ri < minRefresh {
				minRefresh = ri
			}
		}
	}
	if minRefresh < raMinBucketCount {
		minRefresh = raMinBucketCount
	}
	if minRefresh > raMaxBucketCount {
		minRefresh = raMaxBucketCount
	}
	return minRefresh
}

func (c *Component) raBucketOf(sessionID string) int {
	if c.raBucketCount <= 0 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(sessionID))
	return int(h.Sum32() % uint32(c.raBucketCount))
}

// placeSessionInRABucket enrolls a session in periodic RA emission. Called when
// IPv6CP comes up (and on restore of an already-open session), never on the
// broad PADR-time session add — only locally-terminated IPv6-up sessions are
// advertised to.
func (c *Component) placeSessionInRABucket(s *SessionState) {
	if c.raBucketCount <= 0 || s.SessionID == "" {
		return
	}
	b := c.raBucketOf(s.SessionID)
	c.raBucketMu.Lock()
	for _, id := range c.raBuckets[b] {
		if id == s.SessionID {
			c.raBucketMu.Unlock()
			return
		}
	}
	c.raBuckets[b] = append(c.raBuckets[b], s.SessionID)
	c.raBucketMu.Unlock()
}

// removeSessionFromRABucket withdraws a session from periodic RA emission. Called
// on IPv6CP down, LAC handoff, and session teardown.
func (c *Component) removeSessionFromRABucket(s *SessionState) {
	if c.raBucketCount <= 0 || s.SessionID == "" {
		return
	}
	b := c.raBucketOf(s.SessionID)
	c.raBucketMu.Lock()
	ids := c.raBuckets[b]
	for i, id := range ids {
		if id == s.SessionID {
			c.raBuckets[b] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	c.raBucketMu.Unlock()
}

// periodicRAEmitter walks one RA bucket per tick, re-sending each enrolled
// session's RA before its Router Lifetime so the subscriber's default route
// never expires (RFC 4861 §6.2.1).
func (c *Component) periodicRAEmitter() {
	if c.raBucketCount <= 0 {
		return
	}
	ticker := time.NewTicker(raTickInterval)
	defer ticker.Stop()
	bucket := 0
	for {
		select {
		case <-c.Ctx.Done():
			return
		case <-ticker.C:
			c.emitRABucket(bucket)
			bucket = (bucket + 1) % c.raBucketCount
		}
	}
}

func (c *Component) emitRABucket(bucket int) {
	c.raBucketMu.RLock()
	ids := append([]string(nil), c.raBuckets[bucket]...)
	c.raBucketMu.RUnlock()
	if len(ids) == 0 {
		return
	}
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil {
		return
	}
	now := time.Now()
	for _, id := range ids {
		c.sessionMu.RLock()
		s := c.sessionIDIndex[id]
		c.sessionMu.RUnlock()
		if s == nil {
			continue
		}
		c.emitPeriodicRA(s, cfg, now)
	}
}

// emitPeriodicRA re-sends a session's unsolicited RA (to ff02::1 over the
// point-to-point link) when it is due. If the group's IPv6 was disabled while
// the session was advertising, it sends a single Router-Lifetime-0 RA to drop
// the route now (RFC 4861 §6.2.5).
func (c *Component) emitPeriodicRA(s *SessionState, cfg *config.Config, now time.Time) {
	s.mu.Lock()
	ipv6up := s.ipv6cpOpen
	phase := s.Phase
	svlan := s.OuterVLAN
	cvlan := s.InnerVLAN
	due := s.nextRADue
	s.mu.Unlock()

	if !ipv6up || phase == ppp.PhaseLACTunneled {
		return
	}

	srgName := c.resolveSRGName(svlan, cvlan)
	if c.srgMgr != nil && !c.srgMgr.IsActive(srgName) {
		return
	}

	match, ok := c.cfgMgr.LookupSubscriberGroup(svlan, cvlan)
	if !ok || match.Group == nil || match.Group.IPv6Profile == "" {
		if !due.IsZero() {
			c.ceaseSessionRA(s)
			s.mu.Lock()
			s.nextRADue = time.Time{}
			s.mu.Unlock()
		}
		return
	}

	bngMAC, parentSwIfIndex := s.bngSourceMAC()
	if bngMAC == nil {
		return
	}

	key := fmt.Sprintf("%s|%s|%d", match.Name, srgName, parentSwIfIndex)
	st := c.raEngine.GroupStateFor(key, cfg, match.Group, bngMAC)
	if st == nil {
		return
	}
	if !due.IsZero() && now.Before(due) {
		return
	}

	raw := make([]byte, len(st.RawData))
	copy(raw, st.RawData)
	copy(raw[24:40], net.IPv6linklocalallnodes)
	ra.PatchChecksum(raw)
	s.sendIPv6Packet(raw)

	s.mu.Lock()
	s.nextRADue = now.Add(st.Refresh)
	s.mu.Unlock()
}

// ceaseSessionRA sends a single Router-Lifetime-0 RA so the subscriber drops its
// default route immediately when the BNG stops being its default router (group
// IPv6 disabled). Never sent on a transient restart, which keeps the route alive
// via opdb restore.
func (c *Component) ceaseSessionRA(s *SessionState) {
	bngMAC, _ := s.bngSourceMAC()
	if bngMAC == nil {
		return
	}
	raw, err := ra.BuildRARawData(
		southbound.IPv6RAConfig{Managed: true, Other: true, RouterLifetime: 0},
		nil, bngMAC, ra.LinkLocalFromMAC(bngMAC), net.IPv6linklocalallnodes, false, c.logger)
	if err != nil {
		return
	}
	s.sendIPv6Packet(raw)
}
