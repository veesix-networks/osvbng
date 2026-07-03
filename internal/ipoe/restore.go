// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ipoe

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	hapb "github.com/veesix-networks/osvbng/api/proto/ha"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/ha"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/opdb"
	"google.golang.org/protobuf/proto"
)

func (c *Component) checkpointSession(sess *SessionState) {
	c.addSessionToIndexes(sess)

	if c.opdb == nil {
		return
	}

	sess.mu.Lock()
	sessID := sess.SessionID
	data, err := json.Marshal(sess)
	sess.mu.Unlock()
	if err != nil {
		c.logger.Warn("Failed to marshal session for checkpoint", "session_id", sessID, "error", err)
		return
	}

	go func() {
		if err := c.opdb.Put(c.Ctx, opdb.NamespaceIPoESessions, sessID, data); err != nil {
			c.logger.Warn("Failed to checkpoint session", "session_id", sessID, "error", err)
		}
	}()
}

func (c *Component) deleteSessionCheckpoint(sessionID string) {
	if c.opdb == nil {
		return
	}

	if err := c.opdb.Delete(c.Ctx, opdb.NamespaceIPoESessions, sessionID); err != nil {
		c.logger.Warn("Failed to delete session checkpoint", "session_id", sessionID, "error", err)
	}
}

func (c *Component) restoreSessions(ctx context.Context) error {
	if c.opdb == nil {
		return nil
	}

	c.currentRestoreCause = c.detectRestoreCause()
	defer func() { c.currentRestoreCause = "" }()

	var count, expired, failed, halfEstablished int
	sessionCounts := make(map[string]int)
	now := time.Now()

	err := c.opdb.Load(ctx, opdb.NamespaceIPoESessions, func(key string, value []byte) error {
		var sess SessionState
		if err := json.Unmarshal(value, &sess); err != nil {
			c.logger.Warn("Failed to unmarshal session from opdb", "key", key, "error", err)
			return nil
		}

		if c.isSessionExpired(&sess, now) {
			if err := c.opdb.Delete(ctx, opdb.NamespaceIPoESessions, key); err != nil {
				c.logger.Warn("Failed to delete expired session", "key", key, "error", err)
			}
			expired++
			return nil
		}

		// AAA-approved but VPP-side never created: the session was caught
		// mid-handshake at crash time. Reset auth state and let the
		// subscriber re-establish via normal handshake. No setupSession
		// replay because there is nothing to replay.
		if sess.AAAApproved && !sess.IPoESessionCreated {
			c.logger.Debug("Session approved but IPoE never created, resetting AAA state",
				"session_id", sess.SessionID)
			sess.AAAApproved = false
			data, mErr := json.Marshal(&sess)
			if mErr == nil {
				if err := c.opdb.Put(ctx, opdb.NamespaceIPoESessions, sess.SessionID, data); err != nil {
					c.logger.Warn("Failed to persist reset session", "session_id", sess.SessionID, "error", err)
				}
			}
			c.installInMemoryState(&sess)
			halfEstablished++
			return nil
		}

		c.installInMemoryState(&sess)

		if err := c.setupSession(ctx, &sess, SetupModeRestore); err != nil {
			c.logger.Error("Failed to restore session in VPP",
				"session_id", sess.SessionID, "error", err)
			failed++
			// Do NOT delete the opdb entry. Next osvbngd restart retries.
			return nil
		}

		if sess.State == "bound" && sess.MAC != nil {
			counterKey := fmt.Sprintf("osvbng:session_count:%s:%d:%d",
				sess.MAC.String(), sess.OuterVLAN, sess.InnerVLAN)
			sessionCounts[counterKey]++
		}

		if sess.State == "bound" {
			c.restoreSessionToCache(ctx, &sess, now)
		}

		count++
		return nil
	})
	if err != nil {
		return fmt.Errorf("restore ipoe sessions: %w", err)
	}

	for counterKey, cnt := range sessionCounts {
		for i := 0; i < cnt; i++ {
			c.cache.Incr(ctx, counterKey)
		}
	}

	c.logger.Info("Restored IPoE sessions from OpDB",
		"restored", count,
		"expired", expired,
		"failed", failed,
		"half_established", halfEstablished,
		"cause", string(c.currentRestoreCause))
	return nil
}

// detectRestoreCause inspects VPP state to identify which recovery
// scenario produced this restoreSessions call. The cause is informational:
// the unified setupSession path handles every case identically thanks to
// plugin-side idempotency; only TopicSessionRestored consumers that care
// to branch on the cause use this field.
func (c *Component) detectRestoreCause() events.RestoreCause {
	if c.vpp == nil {
		return events.RestoreCauseColdBoot
	}
	ifaces, err := c.vpp.DumpInterfaces()
	if err != nil || len(ifaces) == 0 {
		return events.RestoreCauseColdBoot
	}
	// Any IPoE session interface still present in VPP means the dataplane
	// was preserved across the osvbngd restart.
	for _, iface := range ifaces {
		if strings.HasPrefix(iface.Name, "ipoe_session") {
			return events.RestoreCauseOsvbngdRestart
		}
	}
	return events.RestoreCauseVPPRecovery
}

func (c *Component) isSessionExpired(sess *SessionState, now time.Time) bool {
	if sess.State != "bound" {
		return false
	}

	if sess.IPv4 != nil && sess.LeaseTime > 0 && !sess.BoundAt.IsZero() {
		expiresAt := sess.BoundAt.Add(time.Duration(sess.LeaseTime) * time.Second)
		if now.After(expiresAt) {
			return true
		}
	}

	if sess.IPv6Bound && sess.IPv6LeaseTime > 0 && !sess.IPv6BoundAt.IsZero() {
		expiresAt := sess.IPv6BoundAt.Add(time.Duration(sess.IPv6LeaseTime) * time.Second)
		if now.After(expiresAt) {
			return true
		}
	}

	return false
}

func (c *Component) restoreSessionToCache(ctx context.Context, sess *SessionState, now time.Time) {
	cacheKey := fmt.Sprintf("osvbng:sessions:%s", sess.SessionID)

	protocol := string(models.ProtocolDHCPv4)
	if sess.IPv4 == nil && sess.IPv6Bound {
		protocol = string(models.ProtocolDHCPv6)
	}

	ipoeSess := &models.IPoESession{
		SessionID:     sess.SessionID,
		State:         models.SessionStateActive,
		AccessType:    string(models.AccessTypeIPoE),
		Protocol:      protocol,
		MAC:           sess.MAC,
		OuterVLAN:     sess.OuterVLAN,
		InnerVLAN:     sess.InnerVLAN,
		VLANCount:     c.getVLANCount(sess.OuterVLAN, sess.InnerVLAN),
		IfIndex:       sess.IPoESwIfIndex,
		VRF:           sess.VRF,
		ServiceGroup:  sess.ServiceGroup.Name,
		SRGName:       sess.SRGName,
		IPv4Address:   sess.IPv4,
		LeaseTime:     sess.LeaseTime,
		IPv6Address:   sess.IPv6Address,
		IPv6LeaseTime: sess.IPv6LeaseTime,
		DUID:          sess.DHCPv6DUID,
		Username:      sess.Username,
		Hostname:      sess.Hostname,
		ClientID:      sess.ClientID,
		AAASessionID:  sess.AcctSessionID,
		ActivatedAt:   sess.ActivatedAt,
		Attributes:    sess.Attributes,
	}
	if sess.IPv6Prefix != nil {
		ipoeSess.IPv6Prefix = sess.IPv6Prefix.String()
	}

	data, err := json.Marshal(ipoeSess)
	if err != nil {
		c.logger.Warn("Failed to marshal session for cache restore", "session_id", sess.SessionID, "error", err)
		return
	}

	var ttl time.Duration
	if sess.LeaseTime > 0 && !sess.BoundAt.IsZero() {
		expiresAt := sess.BoundAt.Add(time.Duration(sess.LeaseTime) * time.Second)
		ttl = expiresAt.Sub(now)
		if ttl < 0 {
			ttl = 0
		}
	}
	if sess.IPv6LeaseTime > 0 && !sess.IPv6BoundAt.IsZero() {
		expiresAt := sess.IPv6BoundAt.Add(time.Duration(sess.IPv6LeaseTime) * time.Second)
		v6ttl := expiresAt.Sub(now)
		if v6ttl > ttl {
			ttl = v6ttl
		}
	}

	if err := c.cache.Set(ctx, cacheKey, data, ttl); err != nil {
		c.logger.Warn("Failed to restore session to cache", "session_id", sess.SessionID, "error", err)
	}
}

func (c *Component) RecoverSessions(ctx context.Context) error {
	total := c.sessionCount()

	if total == 0 {
		c.logger.Debug("No IPoE sessions to recover")
		return nil
	}

	c.logger.Debug("Recovering IPoE sessions from OpDB", "total_in_memory", total)

	if err := c.restoreSessions(ctx); err != nil {
		return fmt.Errorf("recover ipoe sessions: %w", err)
	}

	recovered := c.sessionCount()

	c.logger.Debug("IPoE session recovery complete", "recovered", recovered)
	return nil
}

func (c *Component) handleHAStateChange(event events.Event) {
	data, ok := event.Data.(events.HAStateChangeEvent)
	if !ok {
		return
	}

	wasActive := data.OldState == string(ha.SRGStateActive) || data.OldState == string(ha.SRGStateActiveSolo)
	isActive := data.NewState == string(ha.SRGStateActive) || data.NewState == string(ha.SRGStateActiveSolo)
	wasStandbyAlone := data.OldState == string(ha.SRGStateStandbyAlone)

	if isActive && !wasActive && wasStandbyAlone {
		c.logger.Debug("SRG promoted from standby alone, restoring synced IPoE sessions", "srg", data.SRGName)
		go c.restoreFromHASync(data.SRGName)
	}
}

func (c *Component) restoreFromHASync(srgName string) {
	if c.opdb == nil || c.vpp == nil {
		return
	}

	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil {
		c.logger.Error("Failed to get running config for HA restore", "error", err)
		return
	}

	srgCfg, ok := cfg.HA.SRGs[srgName]
	if !ok || len(srgCfg.Interfaces) == 0 {
		c.logger.Error("SRG config not found or no interfaces", "srg", srgName)
		return
	}

	// At some point when we want to support multi-interfaces on the same
	// srg, we can't hardcode the ifIndex to 0, I'm not sure the best action here
	// so its a problem in the future, SRGs right now are mostly tied to S-VLAN
	// therefore its probably a very specific scenario someone wants this feature...
	encapIfIndex, ok := c.ifMgr.GetSwIfIndex(srgCfg.Interfaces[0])
	if !ok {
		c.logger.Error("Failed to resolve SRG access interface",
			"srg", srgName,
			"interface", srgCfg.Interfaces[0])
		return
	}

	type checkpoint struct {
		key  string
		data []byte
	}
	var checkpoints []checkpoint

	c.opdb.Load(c.Ctx, opdb.NamespaceHASyncedIPoE, func(key string, value []byte) error {
		cp := make([]byte, len(value))
		copy(cp, value)
		checkpoints = append(checkpoints, checkpoint{key: key, data: cp})
		return nil
	})

	if len(checkpoints) == 0 {
		c.logger.Debug("No synced IPoE sessions to restore", "srg", srgName)
		return
	}

	c.logger.Debug("Restoring synced IPoE sessions", "srg", srgName, "count", len(checkpoints))

	var restored, failed int
	now := time.Now()

	for _, entry := range checkpoints {
		var cp hapb.SessionCheckpoint
		if err := proto.Unmarshal(entry.data, &cp); err != nil {
			c.logger.Warn("Failed to unmarshal synced IPoE checkpoint", "key", entry.key, "error", err)
			failed++
			continue
		}

		if cp.SrgName != srgName {
			continue
		}

		mac := net.HardwareAddr(cp.Mac)
		outerVLAN := uint16(cp.OuterVlan)
		innerVLAN := uint16(cp.InnerVlan)

		var decapVrfID uint32
		if cp.Vrf != "" && c.vrfMgr != nil {
			tableID, _, _, err := c.vrfMgr.ResolveVRF(cp.Vrf)
			if err != nil {
				c.logger.Error("Failed to resolve VRF for HA restore",
					"session_id", cp.SessionId, "vrf", cp.Vrf, "error", err)
				failed++
				continue
			}
			decapVrfID = tableID
		}

		localMAC := c.getLocalMAC(srgName, encapIfIndex)
		if localMAC == nil {
			c.logger.Error("No local MAC available for HA restore", "session_id", cp.SessionId)
			failed++
			continue
		}

		swIfIndex, err := c.vpp.AddIPoESession(mac, localMAC, encapIfIndex, outerVLAN, innerVLAN, decapVrfID)
		if err != nil {
			c.logger.Error("Failed to create IPoE session from HA sync",
				"session_id", cp.SessionId, "error", err)
			failed++
			continue
		}

		var ipv4 net.IP
		if len(cp.Ipv4Address) > 0 {
			ipv4 = net.IP(cp.Ipv4Address)
		}

		var ipv6 net.IP
		if len(cp.Ipv6Address) > 0 {
			ipv6 = net.IP(cp.Ipv6Address)
		}

		var ipv6Prefix *net.IPNet
		if len(cp.Ipv6Prefix) > 0 && cp.Ipv6PrefixLen > 0 {
			ipv6Prefix = &net.IPNet{
				IP:   net.IP(cp.Ipv6Prefix),
				Mask: net.CIDRMask(int(cp.Ipv6PrefixLen), 128),
			}
		}

		var boundAt time.Time
		if cp.BoundAtNs > 0 {
			boundAt = time.Unix(0, cp.BoundAtNs)
		} else {
			boundAt = now
		}

		sess := &SessionState{
			SessionID:          cp.SessionId,
			AcctSessionID:      cp.AaaSessionId,
			MAC:                mac,
			OuterVLAN:          outerVLAN,
			InnerVLAN:          innerVLAN,
			EncapIfIndex:       encapIfIndex,
			IPoESwIfIndex:      swIfIndex,
			IPoESessionCreated: true,
			State:              "bound",
			IPv4:               ipv4,
			IPv6Address:        ipv6,
			IPv6Prefix:         ipv6Prefix,
			IPv6Bound:          ipv6 != nil || ipv6Prefix != nil,
			LeaseTime:          cp.Ipv4LeaseTime,
			IPv6LeaseTime:      cp.Ipv6LeaseTime,
			BoundAt:            boundAt,
			ActivatedAt:        boundAt,
			IPv6BoundAt:        boundAt,
			AAAApproved:        true,
			Username:           cp.Username,
			VRF:                cp.Vrf,
			SRGName:            srgName,
			CircuitID:          cp.CircuitId,
			RemoteID:           cp.RemoteId,
			ClientID:           cp.ClientId,
			Hostname:           cp.Hostname,
			DHCPv6DUID:         cp.Dhcpv6Duid,
		}

		if cp.ServiceGroup != "" {
			var aaaAttrs map[string]interface{}
			if len(cp.AaaAttributes) > 0 {
				aaaAttrs = make(map[string]interface{}, len(cp.AaaAttributes))
				for k, v := range cp.AaaAttributes {
					aaaAttrs[k] = v
				}
				sess.Attributes = cp.AaaAttributes
			}
			sess.ServiceGroup = c.svcGroupResolver.Resolve(cp.ServiceGroup, cp.ServiceGroup, aaaAttrs)
		}

		unnumberedLoopback := c.resolveUnnumberedLoopback(sess)
		c.setupSessionUnnumbered(cp.SessionId, swIfIndex, unnumberedLoopback)

		if ipv4 != nil {
			if err := c.vpp.IPoESetSessionIPv4(swIfIndex, ipv4, true); err != nil {
				c.logger.Error("Failed to bind IPv4 during HA restore",
					"session_id", cp.SessionId, "error", err)
			}
		}

		if ipv6 != nil {
			if err := c.vpp.IPoESetSessionIPv6(swIfIndex, ipv6, true); err != nil {
				c.logger.Error("Failed to bind IPv6 during HA restore",
					"session_id", cp.SessionId, "error", err)
			}
		}

		if ipv6Prefix != nil {
			nextHop := ipv6
			if nextHop == nil {
				nextHop = net.ParseIP("::")
			}
			if err := c.vpp.IPoESetDelegatedPrefix(swIfIndex, *ipv6Prefix, nextHop, true); err != nil {
				c.logger.Error("Failed to bind delegated prefix during HA restore",
					"session_id", cp.SessionId, "error", err)
			}
		}

		lookupKey := c.makeSessionKeyV4(mac, outerVLAN, innerVLAN)

		c.sessions.Store(lookupKey, sess)
		c.sessionIndex.Store(cp.SessionId, sess)

		c.restoreSessionToCache(c.Ctx, sess, now)
		c.checkpointSession(sess)
		c.publishSessionProgrammed(sess, swIfIndex)

		c.opdb.Delete(c.Ctx, opdb.NamespaceHASyncedIPoE, cp.SessionId)

		restored++
		c.logger.Debug("Restored IPoE session from HA sync",
			"session_id", cp.SessionId,
			"mac", mac,
			"ipv4", ipv4,
			"sw_if_index", swIfIndex)
	}

	c.logger.Debug("HA IPoE session restore complete",
		"srg", srgName,
		"restored", restored,
		"failed", failed)

	if restored > 0 && c.srgMgr != nil {
		c.srgMgr.RequestGARP(srgName)
	}
}

func (c *Component) checkpointSessionSync(sess *SessionState) error {
	if c.opdb == nil {
		return nil
	}
	data, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	return c.opdb.Put(c.Ctx, opdb.NamespaceIPoESessions, sess.SessionID, data)
}

func (c *Component) buildModelSnapshot(sess *SessionState) *models.IPoESession {
	snapshot := &models.IPoESession{
		SessionID:     sess.SessionID,
		State:         models.SessionState(sess.State),
		AccessType:    string(models.AccessTypeIPoE),
		MAC:           sess.MAC,
		OuterVLAN:     sess.OuterVLAN,
		InnerVLAN:     sess.InnerVLAN,
		IfIndex:       sess.IPoESwIfIndex,
		VRF:           sess.VRF,
		ServiceGroup:  sess.ServiceGroup.Name,
		SRGName:       sess.SRGName,
		IPv4Address:   sess.IPv4,
		LeaseTime:     sess.LeaseTime,
		Hostname:      sess.Hostname,
		ClientID:      sess.ClientID,
		IPv6Address:   sess.IPv6Address,
		IPv6LeaseTime: sess.IPv6LeaseTime,
		DUID:          sess.DHCPv6DUID,
		Username:      sess.Username,
		AAASessionID:  sess.AcctSessionID,
		ActivatedAt:   sess.ActivatedAt,
		Attributes:    sess.Attributes,
	}
	if sess.IPv6Prefix != nil {
		snapshot.IPv6Prefix = sess.IPv6Prefix.String()
	}
	return snapshot
}
