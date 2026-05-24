// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ipoe

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/veesix-networks/osvbng/pkg/southbound"
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
// Restore mode (returns ErrSetupRestoreNotImplemented here): the synchronous
// replay path that the unified-session-recovery work will add in a later
// commit. Documented at the call site so callers can prepare for it without
// changing signatures again.
func (c *Component) setupSession(ctx context.Context, sess *SessionState, mode SetupMode) error {
	if c.vpp == nil {
		return nil
	}

	if mode == SetupModeRestore {
		return ErrSetupRestoreNotImplemented
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

// ErrSetupRestoreNotImplemented is returned by setupSession in restore mode
// until the synchronous replay path lands. Allows the restoreSessions
// rewrite to land call sites against the final signature without changing
// it again later.
var ErrSetupRestoreNotImplemented = errors.New("setupSession restore mode not yet implemented")
