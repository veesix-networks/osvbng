// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ipoe

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/veesix-networks/osvbng/pkg/allocator"
	"github.com/veesix-networks/osvbng/pkg/config/qos"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/svcgroup"
)

// SetupMode selects between fresh post-auth bring-up and synchronous opdb
// restore. Fresh mode preserves the existing async AddIPoESession + pending
// DHCP packet snapshot-and-drain pattern; restore mode (filled in by the
// restoreSessions rewrite that follows) runs the same step sequence
// synchronously against checkpointed state.
type SetupMode int

const (
	SetupModeFresh SetupMode = iota
	SetupModeRestore
)

// setupSession brings sess to fully-programmed VPP state.
//
// Fresh mode (the only mode wired here): queues AddIPoESession asynchronously
// and returns nil immediately. Completion is observed by the async callback
// which snapshots any DHCP bindings that arrived during the VPP round-trip,
// applies them to the new session interface, and replays any late DHCPv4 /
// DHCPv6 discover/request packets. This preserves the concurrency contract
// already in place at handleAAAResponse.
//
// Restore mode (returns ErrSetupRestoreNotImplemented in earlier
// staging commits, then concrete behaviour once setupSessionRestore
// lands): the synchronous replay path used by restoreSessions to
// replay opdb-checkpointed sessions through the same idempotent
// step sequence as fresh bring-up.
func (c *Component) setupSession(ctx context.Context, sess *SessionState, mode SetupMode) error {
	if c.vpp == nil {
		return nil
	}

	if mode == SetupModeRestore {
		return c.setupSessionRestore(ctx, sess)
	}

	sess.mu.Lock()
	if sess.IPoESessionCreated {
		sess.mu.Unlock()
		c.logger.Debug("IPoE session already created by another handler", "session_id", sess.SessionID)
		return nil
	}
	sessID := sess.SessionID
	mac := sess.MAC
	svlan := sess.OuterVLAN
	cvlan := sess.InnerVLAN
	encapIfIndex := sess.EncapIfIndex
	srgName := sess.SRGName
	vrfName := sess.VRF
	sess.mu.Unlock()

	var decapVrfID uint32
	if vrfName != "" && c.vrfMgr != nil {
		tableID, _, _, err := c.vrfMgr.ResolveVRF(vrfName)
		if err != nil {
			return fmt.Errorf("resolve vrf %q for session %s: %w", vrfName, sessID, err)
		}
		decapVrfID = tableID
	}

	localMAC := c.getLocalMAC(srgName, encapIfIndex)
	if localMAC == nil {
		return fmt.Errorf("no local MAC available for session %s svlan %d", sessID, svlan)
	}

	c.vpp.AddIPoESessionAsync(mac, localMAC, encapIfIndex, svlan, cvlan, decapVrfID, func(swIfIndex uint32, err error) {
		c.onSessionCreated(ctx, sess, sessID, mac, svlan, cvlan, encapIfIndex, srgName, swIfIndex, err)
	})
	return nil
}

// onSessionCreated runs after AddIPoESessionAsync returns and applies the
// post-create programming: snapshot+drain pending DHCP bindings, set
// unnumbered, apply pending IPv4/IPv6/PD bindings, replay late DHCP
// packets. Lifted verbatim from the inline callback that used to sit inside
// handleAAAResponse.
func (c *Component) onSessionCreated(ctx context.Context, sess *SessionState, sessID string, mac net.HardwareAddr, svlan, cvlan uint16, encapIfIndex uint32, srgName string, swIfIndex uint32, err error) {
	sess.mu.Lock()
	if sess.IPoESessionCreated {
		sess.mu.Unlock()
		c.logger.Debug("IPoE session already created by concurrent handler", "session_id", sessID)
		return
	}
	if err != nil {
		sess.mu.Unlock()
		if errors.Is(err, southbound.ErrUnavailable) {
			c.logger.Debug("VPP unavailable, cannot create IPoE session", "session_id", sessID)
		} else {
			c.logger.Error("Failed to create IPoE session in VPP", "session_id", sessID, "error", err)
		}
		return
	}
	sess.IPoESwIfIndex = swIfIndex
	sess.IPoESessionCreated = true
	pendingIPv4 := sess.PendingIPv4Binding
	pendingIPv6 := sess.PendingIPv6Binding
	pendingPD := sess.PendingPDBinding
	sess.PendingIPv4Binding = nil
	sess.PendingIPv6Binding = nil
	sess.PendingPDBinding = nil
	latePendingV6Solicit := sess.PendingDHCPv6Solicit
	latePendingV6Request := sess.PendingDHCPv6Request
	latePendingV4Discover := sess.PendingDHCPDiscover
	latePendingV4Request := sess.PendingDHCPRequest
	lateV6DUID := sess.DHCPv6DUID
	lateAllocCtx := sess.AllocCtx
	sess.PendingDHCPv6Solicit = nil
	sess.PendingDHCPv6Request = nil
	sess.PendingDHCPDiscover = nil
	sess.PendingDHCPRequest = nil
	unnumberedLoopback := c.resolveUnnumberedLoopback(sess)
	sess.mu.Unlock()
	c.logger.Debug("Created IPoE session in VPP", "session_id", sessID, "sw_if_index", swIfIndex)

	c.setupSessionUnnumbered(sessID, swIfIndex, unnumberedLoopback)

	if err := c.applyServiceGroupBindings(sess, swIfIndex); err != nil {
		c.logger.Warn("Failed to apply service group bindings",
			"session_id", sessID, "sw_if_index", swIfIndex, "error", err)
	}

	if pendingIPv4 != nil {
		c.vpp.IPoESetSessionIPv4Async(swIfIndex, pendingIPv4, true, func(err error) {
			if err != nil {
				if errors.Is(err, southbound.ErrUnavailable) {
					c.logger.Debug("VPP unavailable, cannot bind pending IPv4", "session_id", sessID)
				} else {
					c.logger.Error("Failed to bind pending IPv4", "session_id", sessID, "error", err)
				}
				return
			}
			c.logger.Debug("Bound pending IPv4 to IPoE session", "session_id", sessID, "ipv4", pendingIPv4.String())
			c.publishSessionProgrammed(sess, swIfIndex)
		})
	}
	if pendingIPv6 != nil {
		c.vpp.IPoESetSessionIPv6Async(swIfIndex, pendingIPv6, true, func(err error) {
			if err != nil {
				if errors.Is(err, southbound.ErrUnavailable) {
					c.logger.Debug("VPP unavailable, cannot bind pending IPv6", "session_id", sessID)
				} else {
					c.logger.Error("Failed to bind pending IPv6", "session_id", sessID, "error", err)
				}
				return
			}
			c.logger.Debug("Bound pending IPv6 to IPoE session", "session_id", sessID, "ipv6", pendingIPv6.String())
		})
	}
	if pendingPD != nil {
		nextHop := pendingIPv6
		if nextHop == nil {
			nextHop = net.ParseIP("::")
		}
		c.vpp.IPoESetDelegatedPrefixAsync(swIfIndex, *pendingPD, nextHop, true, func(err error) {
			if err != nil {
				if errors.Is(err, southbound.ErrUnavailable) {
					c.logger.Debug("VPP unavailable, cannot bind pending PD", "session_id", sessID)
				} else {
					c.logger.Error("Failed to bind pending delegated prefix", "session_id", sessID, "error", err)
				}
				return
			}
			c.logger.Debug("Bound pending delegated prefix", "session_id", sessID, "prefix", pendingPD.String())
		})
	}

	c.forwardLatePendingPackets(sess, sessID, mac, svlan, cvlan, encapIfIndex, srgName, lateAllocCtx, lateV6DUID, latePendingV4Discover, latePendingV4Request, latePendingV6Solicit, latePendingV6Request)
}

// applyServiceGroupBindings programs the QoS / ACL / uRPF bindings for the
// session's resolved service group onto swIfIndex, resolving QoS policy
// references against the running config. Shared by fresh bring-up and opdb
// restore; the underlying southbound calls are idempotent, so re-applying
// the same configuration is a no-op.
func (c *Component) applyServiceGroupBindings(sess *SessionState, swIfIndex uint32) error {
	cfg, _ := c.cfgMgr.GetRunning()
	var qosPolicies map[string]*qos.Policy
	if cfg != nil {
		qosPolicies = cfg.QoSPolicies
	}
	return svcgroup.ApplyToSession(c.vpp, swIfIndex, sess.ServiceGroup, qosPolicies)
}

// resolveCurrentEncapIfIndex re-resolves the access sub-interface
// sw_if_index for sess against the live VPP state. The sw_if_index
// stored in the opdb checkpoint becomes stale across a VPP restart
// because VPP re-numbers sub-interfaces on cold boot from the order
// LoadFromDataplane / autoconfig replays them, so blindly trusting the
// checkpoint value programs the per-session rewrite onto whatever
// sub-interface happens to occupy that index now — typically the wrong
// S-VLAN, which silently breaks forwarding.
//
// Resolves via subscriber-group config: the (parent-interface, svlan)
// pair maps to the conventional VPP sub-interface name "parent.svlan".
// Returns the checkpoint value unchanged when no subscriber group matches
// (operator-authored sessions outside autoconfig — rare).
func (c *Component) resolveCurrentEncapIfIndex(sess *SessionState) uint32 {
	if c.vpp == nil || c.cfgMgr == nil {
		return sess.EncapIfIndex
	}
	match, ok := c.cfgMgr.LookupSubscriberGroup(sess.OuterVLAN, sess.InnerVLAN)
	if !ok || match.VR == nil || match.VR.ParentInterface == "" {
		return sess.EncapIfIndex
	}
	name := fmt.Sprintf("%s.%d", match.VR.ParentInterface, sess.OuterVLAN)
	idx, err := c.vpp.GetInterfaceIndex(name)
	if err != nil || idx == 0 {
		c.logger.Warn("Failed to resolve current encap sw_if_index, using checkpoint value",
			"session_id", sess.SessionID,
			"name", name,
			"checkpoint_index", sess.EncapIfIndex,
			"error", err)
		return sess.EncapIfIndex
	}
	if uint32(idx) != sess.EncapIfIndex {
		c.logger.Info("encap sw_if_index drifted across restart; using current value",
			"session_id", sess.SessionID,
			"name", name,
			"checkpoint", sess.EncapIfIndex,
			"current", idx)
	}
	return uint32(idx)
}

// setupSessionRestore replays a checkpointed session into the dataplane
// synchronously. Called by restoreSessions for each opdb entry once
// installInMemoryState has populated the lookup indexes. The session
// state on entry is assumed to be past the bring-up race — Pending*
// fields nil — because checkpointed sessions are by construction
// post-DHCP-bind / post-IPCP-up.
//
// Plugin-side idempotency lets every step here
// run safely whether the dataplane state already matches the request or
// is empty, so the same code handles osvbngd-restart with VPP intact AND
// VPP-recovery cold-start without branching. Per-session failure surfaces
// as an error; the caller logs and continues with the next session
// without deleting the opdb entry, so the next osvbngd restart retries.
//
// Publishes TopicSessionRestored (not Lifecycle/Programmed) on success so
// AAA does not emit a duplicate Accounting-Start and HA does not
// re-replicate to the standby.
func (c *Component) setupSessionRestore(ctx context.Context, sess *SessionState) error {
	sessID := sess.SessionID

	var decapVrfID uint32
	if sess.VRF != "" && c.vrfMgr != nil {
		tableID, _, _, err := c.vrfMgr.ResolveVRF(sess.VRF)
		if err != nil {
			return fmt.Errorf("resolve vrf %q: %w", sess.VRF, err)
		}
		decapVrfID = tableID
	}

	encapIfIndex := c.resolveCurrentEncapIfIndex(sess)
	sess.EncapIfIndex = encapIfIndex

	localMAC := c.getLocalMAC(sess.SRGName, encapIfIndex)
	if localMAC == nil {
		return fmt.Errorf("no local MAC for session %s svlan %d", sessID, sess.OuterVLAN)
	}

	swIfIndex, err := c.vpp.AddIPoESession(sess.MAC, localMAC, encapIfIndex,
		sess.OuterVLAN, sess.InnerVLAN, decapVrfID)
	if err != nil {
		return fmt.Errorf("add ipoe session: %w", err)
	}
	sess.mu.Lock()
	sess.IPoESwIfIndex = swIfIndex
	sess.IPoESessionCreated = true
	sess.mu.Unlock()

	c.setupSessionUnnumbered(sessID, swIfIndex, c.resolveUnnumberedLoopback(sess))

	if sess.IPv4 != nil {
		if err := c.vpp.IPoESetSessionIPv4(swIfIndex, sess.IPv4, true); err != nil {
			return fmt.Errorf("set ipoe ipv4: %w", err)
		}
	}
	if sess.IPv6Address != nil {
		if err := c.vpp.IPoESetSessionIPv6(swIfIndex, sess.IPv6Address, true); err != nil {
			return fmt.Errorf("set ipoe ipv6: %w", err)
		}
	}
	if sess.IPv6Prefix != nil {
		nextHop := sess.IPv6Address
		if nextHop == nil {
			nextHop = net.ParseIP("::")
		}
		if err := c.vpp.IPoESetDelegatedPrefix(swIfIndex, *sess.IPv6Prefix, nextHop, true); err != nil {
			return fmt.Errorf("set ipoe delegated prefix: %w", err)
		}
	}

	if err := c.applyServiceGroupBindings(sess, swIfIndex); err != nil {
		return fmt.Errorf("apply service group bindings: %w", err)
	}

	if sess.MixedAccess {
		c.claimTuple(sess)
	}

	// Persist the refreshed SessionState back to opdb so the new
	// EncapIfIndex / IPoESwIfIndex (post-VPP-restart renumbering) and
	// any other derived state survive any subsequent osvbngd restart.
	// Without this the next restore loads stale sw_if_indexes from
	// opdb and other readers (cache, API, HA sync) see incorrect
	// values until the next setupSession cycle re-resolves them.
	c.checkpointSession(sess)

	c.eventBus.Publish(events.TopicSessionRestored, events.Event{
		Source: c.Name(),
		Data: &events.SessionRestoredEvent{
			AccessType:   models.AccessTypeIPoE,
			Protocol:     models.ProtocolDHCPv4,
			SessionID:    sessID,
			Session:      c.buildModelSnapshot(sess),
			RestoreCause: c.currentRestoreCause,
		},
	})

	return nil
}

// installInMemoryState rebuilds the in-memory lookup indexes for a
// session loaded from opdb. Mutating fields like Pending* are reset
// because checkpointed sessions are past the bring-up race; if a
// half-established entry is found (AAAApproved without IPoESessionCreated)
// the caller skips setupSession for it and lets the subscriber
// re-establish via normal handshake.
func (c *Component) installInMemoryState(sess *SessionState) {
	sess.PendingDHCPDiscover = nil
	sess.PendingDHCPRequest = nil
	sess.PendingDHCPv6Solicit = nil
	sess.PendingDHCPv6Request = nil
	sess.PendingIPv4Binding = nil
	sess.PendingIPv6Binding = nil
	sess.PendingPDBinding = nil
	sess.AAAInFlight = false
	sess.Closing = false

	lookupKey := c.makeSessionKeyV4(sess.MAC, sess.OuterVLAN, sess.InnerVLAN)
	c.sessions.Store(lookupKey, sess)
	c.sessionIndex.Store(sess.SessionID, sess)
	c.addSessionToIndexes(sess)

	// Re-stake the session's addresses in the global allocator. Without this,
	// the allocator boots with no knowledge of restored leases and a fresh
	// DHCPDISCOVER (after StateReady flips) can be handed an IP that is
	// already in use by a restored session. PPPoE does the equivalent in
	// SessionState.startNCP — IPoE was previously missing the call.
	// Conflicts are logged but not fatal: the opdb side is the source of
	// truth for the restored session, and the operator should investigate
	// any collision before allowing the daemon to flip to Ready.
	if registry := allocator.GetGlobalRegistry(); registry != nil {
		if sess.IPv4 != nil {
			if err := registry.ReserveIP(sess.IPv4, sess.SessionID); err != nil {
				c.logger.Warn("IPv4 reservation conflict during restore",
					"session_id", sess.SessionID,
					"address", sess.IPv4.String(),
					"error", err)
			}
		}
		if sess.IPv6Address != nil {
			if err := registry.ReserveIANA(sess.IPv6Address, sess.SessionID); err != nil {
				c.logger.Warn("IPv6 IANA reservation conflict during restore",
					"session_id", sess.SessionID,
					"address", sess.IPv6Address.String(),
					"error", err)
			}
		}
		if sess.IPv6Prefix != nil {
			if err := registry.ReservePD(sess.IPv6Prefix, sess.SessionID); err != nil {
				c.logger.Warn("PD reservation conflict during restore",
					"session_id", sess.SessionID,
					"prefix", sess.IPv6Prefix.String(),
					"error", err)
			}
		}
	}
}

// ErrSetupRestoreNotImplemented is the legacy sentinel for callers that
// queue setupSession(SetupModeRestore) on a build where the restore path
// is still under construction. Retained for source-compatibility; the
// in-tree restore path now returns concrete errors from setupSessionRestore.
var ErrSetupRestoreNotImplemented = errors.New("setupSession restore mode not yet implemented")

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
