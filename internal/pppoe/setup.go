// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package pppoe

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/veesix-networks/osvbng/pkg/config/qos"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/svcgroup"
)

// SetupMode selects between fresh post-NCP bring-up and synchronous opdb
// restore. Fresh mode preserves the existing async AddPPPoESession + IPCP /
// IPv6CP-driven binding flow; restore mode (filled in by the restoreSessions
// rewrite that follows) runs the same step sequence synchronously against
// checkpointed state.
type SetupMode int

const (
	SetupModeFresh SetupMode = iota
	SetupModeRestore
)

// setupSession brings sess to fully-programmed VPP state.
//
// Fresh mode (the only mode wired here): queues AddPPPoESession
// asynchronously and returns nil immediately. The onVPPSessionCreated
// callback persists the resulting sw_if_index, checkpoints the session,
// publishes TopicSessionProgrammed, and attaches the LCP echo generator.
// Preserves the contract between checkOpen and the async southbound add
// that fresh PPPoE bring-up relies on.
//
// Restore mode (returns ErrSetupRestoreNotImplemented here): the synchronous
// replay path that the unified-session-recovery work will add in a later
// commit. Documented at the call site so callers can prepare for it without
// changing signatures again.
func (c *Component) setupSession(ctx context.Context, sess *SessionState, mode SetupMode) error {
	if c.vpp == nil {
		if c.echoGen != nil {
			magic := sess.lcp.LocalConfig().Magic
			c.echoGen.AddSession(sess.PPPoESessionID, magic, uint8(sess.EchoSeq))
		}
		return nil
	}

	if mode == SetupModeRestore {
		return c.setupSessionRestore(ctx, sess)
	}

	if sess.IPv4Address == nil {
		return nil
	}

	localMAC := c.effectiveLocalMAC(sess.SRGName, sess.EncapIfIndex)
	if localMAC == nil {
		c.logger.Error("Failed to get local MAC",
			"session_id", sess.SessionID,
			"sw_if_index", sess.EncapIfIndex)
		return errors.New("no local MAC for PPPoE session")
	}

	var decapVrfID uint32
	if sess.VRF != "" && c.vrfMgr != nil {
		tableID, _, _, err := c.vrfMgr.ResolveVRF(sess.VRF)
		if err != nil {
			c.logger.Error("Failed to resolve VRF for session",
				"session_id", sess.SessionID,
				"vrf", sess.VRF,
				"error", err)
			return err
		}
		decapVrfID = tableID
	}

	pppMTU, policy := c.resolveMSSClampPolicy(sess)
	c.vpp.AddPPPoESessionAsync(
		sess.PPPoESessionID,
		sess.IPv4Address,
		sess.MAC,
		localMAC,
		sess.EncapIfIndex,
		sess.OuterVLAN,
		sess.InnerVLAN,
		decapVrfID,
		pppMTU,
		policy,
		sess.onVPPSessionCreated,
	)
	return nil
}

// setupSessionRestore replays a checkpointed PPPoE session into the
// dataplane synchronously. Called by restoreSessions for each opdb entry
// once installInMemoryState has populated the lookup indexes and
// re-initialised the PPP FSMs. Sessions in PhaseOpen go through the full
// VPP add + IPCP-bound state replay; PhaseLACTunneled handling is added
// in the next phase alongside the FSM force-restore work.
//
// Plugin-side idempotency lets every step run safely whether the
// dataplane already matches the request or is empty. Per-session failure
// surfaces as an error; the caller logs and continues without deleting
// the opdb entry. Publishes TopicSessionRestored (not Lifecycle /
// Programmed) on success so AAA does not emit a duplicate
// Accounting-Start and HA does not re-replicate to the standby.
func (c *Component) setupSessionRestore(ctx context.Context, sess *SessionState) error {
	if sess.IPv4Address == nil {
		// PhaseLACTunneled subscribers carry no local IP. Replaying them
		// without the L2TP-side LAC binding (added in the next phase)
		// would forward subscriber traffic to the wrong path; skip for
		// now and leave the session in its loaded in-memory state.
		c.logger.Debug("PPPoE restore: no IPv4 address, skipping VPP add",
			"session_id", sess.SessionID,
			"phase", sess.Phase)
		return nil
	}

	encapIfIndex := c.resolveCurrentEncapIfIndex(sess)
	sess.EncapIfIndex = encapIfIndex

	localMAC := c.effectiveLocalMAC(sess.SRGName, encapIfIndex)
	if localMAC == nil {
		return fmt.Errorf("no local MAC for session %s (encap_if_index=%d)", sess.SessionID, encapIfIndex)
	}

	var decapVrfID uint32
	if sess.VRF != "" && c.vrfMgr != nil {
		tableID, _, _, err := c.vrfMgr.ResolveVRF(sess.VRF)
		if err != nil {
			return fmt.Errorf("resolve vrf %q: %w", sess.VRF, err)
		}
		decapVrfID = tableID
	}

	pppMTU, policy := c.resolveMSSClampPolicy(sess)

	swIfIndex, err := c.vpp.AddPPPoESession(
		sess.PPPoESessionID,
		sess.IPv4Address,
		sess.MAC,
		localMAC,
		encapIfIndex,
		sess.OuterVLAN,
		sess.InnerVLAN,
		decapVrfID,
		pppMTU,
		policy,
	)
	if err != nil {
		return fmt.Errorf("add pppoe session: %w", err)
	}
	sess.SwIfIndex = swIfIndex

	cfg, _ := c.cfgMgr.GetRunning()
	var qosPolicies map[string]*qos.Policy
	if cfg != nil {
		qosPolicies = cfg.QoSPolicies
	}
	if err := svcgroup.ApplyToSession(c.vpp, swIfIndex, sess.ServiceGroup, qosPolicies); err != nil {
		return fmt.Errorf("apply service group bindings: %w", err)
	}

	c.setupSessionUnnumbered(sess.SessionID, swIfIndex, c.resolveUnnumberedLoopback(sess))

	// MixedAccess exclusivity is claimed inside installInMemoryState ->
	// addToIndexes for PPPoE (unlike IPoE which holds the claim
	// separately). No explicit re-claim needed here.

	if c.echoGen != nil {
		c.echoGen.AddSession(sess.PPPoESessionID, sess.LCPMagic, uint8(sess.EchoSeq))
	}

	// Persist the refreshed SessionState back to opdb so the new
	// EncapIfIndex / SwIfIndex (post-VPP-restart renumbering) and any
	// other derived state survive any subsequent osvbngd restart.
	c.checkpointSession(sess)

	payload := &models.PPPSession{
		SessionID:        sess.SessionID,
		State:            models.SessionStateActive,
		AccessType:       string(models.AccessTypePPPoE),
		Protocol:         string(models.ProtocolPPPoESession),
		PPPSessionID:     sess.PPPoESessionID,
		MAC:              sess.MAC,
		OuterVLAN:        sess.OuterVLAN,
		InnerVLAN:        sess.InnerVLAN,
		IfIndex:          sess.SwIfIndex,
		VRF:              sess.VRF,
		ServiceGroup:     sess.ServiceGroup.Name,
		SRGName:          sess.SRGName,
		IPv4Address:      sess.IPv4Address,
		IPv6Address:      sess.IPv6Address,
		Username:         sess.Username,
		AAASessionID:     sess.AcctSessionID,
		ActivatedAt:      sess.BoundAt,
		IPv4Pool:         sess.allocatedPool,
		IANAPool:         sess.allocatedIANAPool,
		OuterTPID:        sess.OuterTPID,
		NegotiatedPPPMTU: sess.NegotiatedPPPMTU,
		IPv4MSS:          sess.IPv4MSS,
		IPv6MSS:          sess.IPv6MSS,
	}

	c.eventBus.Publish(events.TopicSessionRestored, events.Event{
		Source: c.Name(),
		Data: &events.SessionRestoredEvent{
			AccessType:   models.AccessTypePPPoE,
			Protocol:     models.ProtocolPPPoESession,
			SessionID:    sess.SessionID,
			Session:      payload,
			RestoreCause: c.currentRestoreCause,
		},
	})

	return nil
}

// effectiveLocalMAC returns the BNG-side source MAC the per-session
// rewrite should carry on egress. SRG virtual MAC takes precedence over
// the physical interface MAC when redundancy is configured for the
// session's SRG, so that failover-promoted sessions emit frames from
// the same logical BNG identity as the prior active node (matches the
// existing PADO emission path).
//
// Falls back to walking the SupSwIfIndex chain: sub-interfaces in VPP
// report a zero L2Address from swInterfaceDump (the physical MAC lives
// on the parent), so reading ifMgr by the sub-interface's sw_if_index
// can give back a zero MAC after VPP recovery.
func (c *Component) effectiveLocalMAC(srgName string, encapIfIndex uint32) net.HardwareAddr {
	if c.srgMgr != nil && srgName != "" {
		if vmac := c.srgMgr.GetVirtualMAC(srgName); vmac != nil {
			out := make(net.HardwareAddr, len(vmac))
			copy(out, vmac)
			return out
		}
	}
	if c.ifMgr == nil {
		return nil
	}
	idx := encapIfIndex
	for hop := 0; hop < 4; hop++ {
		iface := c.ifMgr.Get(idx)
		if iface == nil {
			return nil
		}
		if len(iface.MAC) >= 6 && !macIsZero(iface.MAC[:6]) {
			out := make(net.HardwareAddr, 6)
			copy(out, iface.MAC[:6])
			return out
		}
		if iface.SupSwIfIndex == idx || iface.SupSwIfIndex == 0 {
			return nil
		}
		idx = iface.SupSwIfIndex
	}
	return nil
}

func macIsZero(mac []byte) bool {
	for _, b := range mac {
		if b != 0 {
			return false
		}
	}
	return true
}

// resolveCurrentEncapIfIndex re-resolves the access sub-interface
// sw_if_index for sess against the live VPP state. The sw_if_index
// stored in the opdb checkpoint becomes stale across a VPP restart
// because VPP re-numbers sub-interfaces on cold boot from the order
// LoadFromDataplane / autoconfig replays them. Resolves via the
// subscriber-group (parent-interface, svlan) -> "parent.svlan" naming
// convention. Returns the checkpoint value unchanged when no subscriber
// group matches (operator-authored sessions outside autoconfig).
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

// resolveUnnumberedLoopback returns the loopback the per-session interface
// should be unnumbered against — the service-group's explicit loopback if
// set, otherwise the subscriber-group's gateway loopback for the session's
// S-VLAN. Empty string means no unnumbered binding (the session interface
// stays with no IP; subscribers can route through but cannot ping the BNG
// directly). Mirrors the IPoE component's implementation.
func (c *Component) resolveUnnumberedLoopback(sess *SessionState) string {
	if sess.ServiceGroup.Unnumbered != "" {
		return sess.ServiceGroup.Unnumbered
	}

	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil || cfg.SubscriberGroups == nil {
		return ""
	}

	if group, _ := cfg.SubscriberGroups.FindGroupBySVLAN(sess.OuterVLAN); group != nil {
		return group.FindGatewayForSVLAN(sess.OuterVLAN)
	}

	return ""
}

// setupSessionUnnumbered binds the per-session interface to the given
// loopback so the BNG can source ICMP replies (and any other locally
// terminated traffic) for the subscriber. Empty loopback is a no-op.
// Uses the async southbound API and fires-and-forgets errors to match
// the existing IPoE pattern; setup-time failures are logged but do not
// abort the session bring-up because the dataplane retries on the next
// adjacency event.
func (c *Component) setupSessionUnnumbered(sessID string, swIfIndex uint32, loopback string) {
	if loopback == "" || c.vpp == nil {
		return
	}
	c.vpp.SetUnnumberedAsync(swIfIndex, loopback, func(err error) {
		if err != nil {
			c.logger.Error("Failed to set unnumbered on PPPoE session",
				"session_id", sessID,
				"sw_if_index", swIfIndex,
				"loopback", loopback,
				"error", err)
		}
	})
}

// ErrSetupRestoreNotImplemented is the legacy sentinel for callers that
// queue setupSession(SetupModeRestore) on a build where the restore path
// is still under construction. Retained for source-compatibility; the
// in-tree restore path now returns concrete errors from setupSessionRestore.
var ErrSetupRestoreNotImplemented = errors.New("setupSession restore mode not yet implemented")
