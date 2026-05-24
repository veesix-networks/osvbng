// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ipoe

import (
	"context"
	"errors"
	"fmt"
	"net"

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
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil || cfg.SubscriberGroups == nil {
		return sess.EncapIfIndex
	}
	group, vlanRange := cfg.SubscriberGroups.FindGroupBySVLAN(sess.OuterVLAN)
	if group == nil || vlanRange == nil || vlanRange.ParentInterface == "" {
		return sess.EncapIfIndex
	}
	name := fmt.Sprintf("%s.%d", vlanRange.ParentInterface, sess.OuterVLAN)
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

	cfg, _ := c.cfgMgr.GetRunning()
	var qosPolicies map[string]*qos.Policy
	if cfg != nil {
		qosPolicies = cfg.QoSPolicies
	}
	if err := svcgroup.ApplyToSession(c.vpp, swIfIndex, sess.ServiceGroup, qosPolicies); err != nil {
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
}

// ErrSetupRestoreNotImplemented is the legacy sentinel for callers that
// queue setupSession(SetupModeRestore) on a build where the restore path
// is still under construction. Retained for source-compatibility; the
// in-tree restore path now returns concrete errors from setupSessionRestore.
var ErrSetupRestoreNotImplemented = errors.New("setupSession restore mode not yet implemented")
