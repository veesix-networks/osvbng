// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ipoe

import (
	"context"
	"fmt"
	"sync"

	"github.com/veesix-networks/osvbng/pkg/events"
)

func (c *Component) handleAAAResponse(event events.Event) {
	data, ok := event.Data.(*events.AAAResponseEvent)
	if !ok {
		c.logger.Error("Invalid event data for AAA response")
		return
	}

	sessID := data.SessionID
	allowed := data.Response.Allowed

	val, ok := c.sessionIndex.Load(sessID)
	if !ok {
		c.logger.Error("Session not found for AAA response", "session_id", sessID)
		return
	}
	sess := val.(*SessionState)

	sess.mu.Lock()
	sess.AAAApproved = allowed
	sess.AAAInFlight = false
	if !allowed {
		sess.Closing = true
	}
	pendingDiscover := sess.PendingDHCPDiscover
	pendingRequest := sess.PendingDHCPRequest
	sess.PendingDHCPDiscover = nil
	sess.PendingDHCPRequest = nil
	mac := sess.MAC
	svlan := sess.OuterVLAN
	cvlan := sess.InnerVLAN
	encapIfIndex := sess.EncapIfIndex
	ipoeCreated := sess.IPoESessionCreated
	sess.mu.Unlock()

	if !allowed {
		c.logger.Debug("Session AAA rejected, cleaning up session", "session_id", sessID)
		c.xidIndex.Delete(sess.XID)
		lookupV4 := c.makeSessionKeyV4(mac, svlan, cvlan)
		lookupV6 := c.makeSessionKeyV6(mac, svlan, cvlan)
		c.sessions.Delete(lookupV4)
		c.sessions.Delete(lookupV6)
		c.sessionIndex.Delete(sessID)
		c.removeSessionFromIndexes(sess)
		return
	}

	var subscriberGroup, ipv4Profile, ipv6Profile string
	if match, ok := c.cfgMgr.LookupSubscriberGroup(svlan, cvlan); ok {
		subscriberGroup = match.Name
		ipv4Profile = match.Group.IPv4Profile
		ipv6Profile = match.Group.IPv6Profile
	}
	c.logger.Debug("Session AAA approved",
		"session_id", sessID,
		"subscriber_group", subscriberGroup,
		"ipv4_profile", ipv4Profile,
		"ipv6_profile", ipv6Profile,
	)

	if ipv4Profile == "" {
		if n := countFamilyAttrs(data.Response.Attributes, v4FamilyAttrs); n > 0 {
			aaaAttrDropFamily.WithLabelValues(subscriberGroup, "ipv4").Add(uint64(n))
			c.logger.Warn("Ignoring off-family IPv4 AAA attributes: group has no ipv4-profile",
				"session_id", sessID, "group", subscriberGroup, "count", n)
		}
	}
	if ipv6Profile == "" {
		if n := countFamilyAttrs(data.Response.Attributes, v6FamilyAttrs); n > 0 {
			aaaAttrDropFamily.WithLabelValues(subscriberGroup, "ipv6").Add(uint64(n))
			c.logger.Warn("Ignoring off-family IPv6 AAA attributes: group has no ipv6-profile",
				"session_id", sessID, "group", subscriberGroup, "count", n)
		}
	}

	resolved := c.resolveServiceGroup(svlan, cvlan, data.Response.Attributes)

	var srgName string
	if c.srgMgr != nil && subscriberGroup != "" {
		srgName = c.srgMgr.GetSRGForGroup(subscriberGroup)
	}

	storedAttrs := make(map[string]string, len(data.Response.Attributes))
	for k, v := range data.Response.Attributes {
		storedAttrs[k] = fmt.Sprintf("%v", v)
	}

	sess.mu.Lock()
	sess.Attributes = storedAttrs
	sess.ServiceGroup = resolved
	sess.SRGName = srgName
	sess.mu.Unlock()

	vrfName := resolved.VRF
	if vrfName != "" {
		if c.vrfMgr != nil {
			if _, _, _, err := c.vrfMgr.ResolveVRF(vrfName); err != nil {
				c.logger.Error("Failed to resolve VRF for session", "session_id", sessID, "vrf", vrfName, "error", err)
				return
			}
		}
		sess.mu.Lock()
		sess.VRF = vrfName
		sess.mu.Unlock()
	}

	allocCtx := c.buildAllocContext(sess, data.Response.Attributes)
	c.logger.Debug("Built allocator context",
		"session_id", sessID,
		"profile", allocCtx.ProfileName,
		"pool_override", allocCtx.PoolOverride,
		"iana_pool_override", allocCtx.IANAPoolOverride,
		"pd_pool_override", allocCtx.PDPoolOverride,
	)
	sess.mu.Lock()
	sess.AllocCtx = allocCtx
	sess.mu.Unlock()

	if !ipoeCreated {
		if err := c.setupSession(context.TODO(), sess, SetupModeFresh); err != nil {
			c.logger.Error("setupSession (fresh) failed",
				"session_id", sessID, "error", err)
		}
	}

	sess.mu.Lock()
	if pendingDiscover == nil {
		pendingDiscover = sess.PendingDHCPDiscover
		sess.PendingDHCPDiscover = nil
	}
	if pendingRequest == nil {
		pendingRequest = sess.PendingDHCPRequest
		sess.PendingDHCPRequest = nil
	}
	pendingDHCPv6Solicit := sess.PendingDHCPv6Solicit
	pendingDHCPv6Request := sess.PendingDHCPv6Request
	dhcpv6DUID := sess.DHCPv6DUID
	sess.PendingDHCPv6Solicit = nil
	sess.PendingDHCPv6Request = nil
	sess.mu.Unlock()

	v4Profile := c.resolveIPv4Profile(allocCtx)
	v6Profile := c.resolveIPv6Profile(allocCtx)
	accessIfName := c.resolveAccessInterfaceName(encapIfIndex)
	localMAC := c.getLocalMAC(srgName, encapIfIndex)

	hasV4 := pendingDiscover != nil || pendingRequest != nil
	v6Provider := c.getDHCP6Provider(v6Profile)
	hasV6 := (pendingDHCPv6Solicit != nil || pendingDHCPv6Request != nil) && v6Provider != nil

	var wg sync.WaitGroup

	if hasV4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.forwardPendingDHCPv4(sessID, mac, svlan, cvlan, encapIfIndex, accessIfName, v4Profile, localMAC, allocCtx, pendingDiscover, pendingRequest)
		}()
	}

	if hasV6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.forwardPendingDHCPv6(sess, sessID, mac, svlan, cvlan, encapIfIndex, accessIfName, v6Profile, v6Provider, localMAC, allocCtx, dhcpv6DUID, pendingDHCPv6Solicit, pendingDHCPv6Request)
		}()
	}

	wg.Wait()
}
