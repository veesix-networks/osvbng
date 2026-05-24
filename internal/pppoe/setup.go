// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package pppoe

import (
	"context"
	"errors"
	"net"
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
		return ErrSetupRestoreNotImplemented
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

// ErrSetupRestoreNotImplemented is returned by setupSession in restore mode
// until the synchronous replay path lands. Allows the restoreSessions
// rewrite to land call sites against the final signature without changing
// it again later.
var ErrSetupRestoreNotImplemented = errors.New("setupSession restore mode not yet implemented")
