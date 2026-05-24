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
			c.echoGen.AddSession(sess.PPPoESessionID, magic)
		}
		return nil
	}

	if mode == SetupModeRestore {
		return c.setupSessionRestore(ctx, sess)
	}

	if sess.IPv4Address == nil {
		return nil
	}

	var localMAC net.HardwareAddr
	if c.ifMgr != nil {
		if iface := c.ifMgr.Get(sess.EncapIfIndex); iface != nil && len(iface.MAC) >= 6 {
			localMAC = net.HardwareAddr(iface.MAC[:6])
		}
	}
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

	var localMAC net.HardwareAddr
	if c.ifMgr != nil {
		if iface := c.ifMgr.Get(sess.EncapIfIndex); iface != nil && len(iface.MAC) >= 6 {
			localMAC = net.HardwareAddr(iface.MAC[:6])
		}
	}
	if localMAC == nil {
		return fmt.Errorf("no local MAC for session %s", sess.SessionID)
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
		sess.EncapIfIndex,
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

	// MixedAccess exclusivity is claimed inside installInMemoryState ->
	// addToIndexes for PPPoE (unlike IPoE which holds the claim
	// separately). No explicit re-claim needed here.

	if c.echoGen != nil {
		c.echoGen.AddSession(sess.PPPoESessionID, sess.LCPMagic)
	}

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

// ErrSetupRestoreNotImplemented is the legacy sentinel for callers that
// queue setupSession(SetupModeRestore) on a build where the restore path
// is still under construction. Retained for source-compatibility; the
// in-tree restore path now returns concrete errors from setupSessionRestore.
var ErrSetupRestoreNotImplemented = errors.New("setupSession restore mode not yet implemented")
