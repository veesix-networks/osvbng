// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ipoe

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/allocator"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/session"
	"github.com/veesix-networks/osvbng/pkg/svcgroup"
)

type SessionState struct {
	mu                  sync.Mutex
	SessionID           string
	AcctSessionID       string
	MAC                 net.HardwareAddr
	OuterVLAN           uint16
	InnerVLAN           uint16
	EncapIfIndex        uint32
	IPoESwIfIndex       uint32
	State               string
	IPv4                net.IP
	LeaseTime           uint32
	BoundAt             time.Time
	ActivatedAt         time.Time
	XID                 uint32
	Hostname            string
	ClientID            []byte
	CircuitID           []byte
	RemoteID            []byte
	LastSeen            time.Time
	AAAApproved         bool
	IPoESessionCreated  bool
	PendingDHCPDiscover []byte
	PendingDHCPRequest  []byte
	PendingIPv4Binding  net.IP
	PendingIPv6Binding  net.IP
	PendingPDBinding    *net.IPNet

	IPv6Address          net.IP
	IPv6Prefix           *net.IPNet
	ClientLinkLocal      net.IP
	DHCPv6DUID           []byte
	DHCPv6XID            [3]byte
	IPv6LeaseTime        uint32
	IPv6BoundAt          time.Time
	IPv6Bound            bool
	PendingDHCPv6Solicit []byte
	PendingDHCPv6Request []byte

	Username     string
	Attributes   map[string]string
	VRF          string
	ServiceGroup svcgroup.ServiceGroup
	SRGName      string
	GroupName    string
	AllocCtx     *allocator.Context
	Closing      bool
	AAAInFlight  bool
	MixedAccess  bool

	nextRADue time.Time
}

func (c *Component) isMixedAccessSVLAN(svlan uint16) bool {
	if c.accessResolver == nil {
		return false
	}
	return c.accessResolver.IsMixedAccessSVLAN(svlan)
}

func (c *Component) claimTuple(sess *SessionState) {
	if c.exclusivity == nil || !sess.MixedAccess {
		return
	}
	tk := session.MakeTupleKey(sess.OuterVLAN, sess.InnerVLAN, sess.MAC)
	owner := session.Owner{Protocol: session.ProtoIPoE, SessionID: sess.SessionID, Key: tk}
	if prev := c.exclusivity.Claim(tk, owner); prev != nil && prev.Protocol != session.ProtoIPoE {
		c.eventBus.Publish(events.TopicSubscriberTerminate, events.Event{
			Source:    "ipoe",
			Timestamp: time.Now(),
			Data: &events.SubscriberTerminateEvent{
				SessionID: prev.SessionID,
				Reason:    "evicted by cross-protocol claim",
				Key:       &tk,
			},
		})
	}
}

func (c *Component) releaseTuple(sess *SessionState) {
	if c.exclusivity == nil || !sess.MixedAccess {
		return
	}
	tk := session.MakeTupleKey(sess.OuterVLAN, sess.InnerVLAN, sess.MAC)
	owner := session.Owner{Protocol: session.ProtoIPoE, SessionID: sess.SessionID, Key: tk}
	c.exclusivity.Release(tk, owner)
}

func (c *Component) addSessionToIndexes(sess *SessionState) {
	if sess.AcctSessionID != "" {
		c.acctSessionIndex.Store(sess.AcctSessionID, sess)
	}
	if sess.Username != "" {
		c.usernameIndex.Store(sess.Username, sess)
	}
	if sess.IPv4 != nil {
		c.ipv4Index.Store(sess.IPv4.String(), sess)
	}
	if sess.IPv6Address != nil {
		c.ipv6Index.Store(sess.IPv6Address.String(), sess)
	}
	c.placeSessionInRABucket(sess)
}

func (c *Component) removeSessionFromIndexes(sess *SessionState) {
	if sess.AcctSessionID != "" {
		c.acctSessionIndex.Delete(sess.AcctSessionID)
	}
	if sess.Username != "" {
		c.usernameIndex.Delete(sess.Username)
	}
	if sess.IPv4 != nil {
		c.ipv4Index.Delete(sess.IPv4.String())
	}
	if sess.IPv6Address != nil {
		c.ipv6Index.Delete(sess.IPv6Address.String())
	}
	c.removeSessionFromRABucket(sess)
}

func (c *Component) getVLANCount(svlan, cvlan uint16) int {
	if cvlan == 0 {
		return 1
	}
	return 2
}

func (c *Component) checkSessionLimit(mac net.HardwareAddr, svlan, cvlan uint16) error {
	cfg, _ := c.cfgMgr.GetRunning()
	if cfg == nil {
		return nil
	}

	var policyName string
	if match, ok := c.cfgMgr.LookupSubscriberGroup(svlan, cvlan); ok {
		if match.VR != nil && match.VR.AAA != nil && match.VR.AAA.Policy != "" {
			policyName = match.VR.AAA.Policy
		} else {
			policyName = match.Group.AAAPolicy
		}
	}

	if policyName == "" {
		return nil
	}

	policy := cfg.AAA.GetPolicy(policyName)
	if policy == nil {
		return nil
	}

	maxSessions := policy.MaxConcurrentSessions
	if maxSessions <= 0 {
		return nil
	}

	count, err := c.countExistingSessions(mac, svlan, cvlan)
	if err != nil {
		c.logger.Warn("Failed to count sessions", "error", err)
		return nil
	}

	if count >= maxSessions {
		return fmt.Errorf("session limit reached (%d/%d) for %s on VLAN %d:%d",
			count, maxSessions, mac.String(), svlan, cvlan)
	}

	c.logger.Debug("Session limit check passed", "current", count, "max", maxSessions, "mac", mac.String(), "svlan", svlan, "cvlan", cvlan)

	return nil
}

func (c *Component) countExistingSessions(mac net.HardwareAddr, svlan, cvlan uint16) (int, error) {
	counterKey := fmt.Sprintf("osvbng:session_count:%s:%d:%d", mac.String(), svlan, cvlan)

	val, err := c.cache.Get(c.Ctx, counterKey)
	if err != nil {
		return 0, nil
	}

	var count int64
	if _, err := fmt.Sscanf(string(val), "%d", &count); err != nil {
		return 0, nil
	}

	return int(count), nil
}

const (
	// reclaimGrace covers clock skew and in-flight renews past lease expiry.
	reclaimGrace        = 5 * time.Minute
	halfOpenIdleTimeout = 2 * time.Minute
)

// leaseGrace caps the reclaim grace at a quarter of the lease so short leases
// are not held disproportionately long past expiry.
func leaseGrace(leaseSeconds uint32) time.Duration {
	if g := time.Duration(leaseSeconds) * time.Second / 4; g < reclaimGrace {
		return g
	}
	return reclaimGrace
}

// sessionPastLease reports whether every bound address family is past its lease
// plus grace. Caller holds sess.mu.
func (c *Component) sessionPastLease(sess *SessionState, now time.Time) bool {
	v4Active := sess.IPv4 != nil && sess.LeaseTime > 0 && !sess.BoundAt.IsZero()
	v6Active := sess.IPv6Bound && sess.IPv6LeaseTime > 0 && !sess.IPv6BoundAt.IsZero()
	if !v4Active && !v6Active {
		return false
	}
	if v4Active {
		expiry := sess.BoundAt.Add(time.Duration(sess.LeaseTime)*time.Second + leaseGrace(sess.LeaseTime))
		if now.Before(expiry) {
			return false
		}
	}
	if v6Active {
		expiry := sess.IPv6BoundAt.Add(time.Duration(sess.IPv6LeaseTime)*time.Second + leaseGrace(sess.IPv6LeaseTime))
		if now.Before(expiry) {
			return false
		}
	}
	return true
}

func (c *Component) cleanupSessions() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-c.Ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			var toDelete []struct {
				key  string
				sess *SessionState
			}
			c.sessions.Range(func(k, v any) bool {
				key := k.(string)
				session := v.(*SessionState)
				session.mu.Lock()
				var reap bool
				if session.State == "bound" {
					reap = c.sessionPastLease(session, now)
				} else {
					reap = now.Sub(session.LastSeen) > halfOpenIdleTimeout
				}
				session.mu.Unlock()
				if reap {
					toDelete = append(toDelete, struct {
						key  string
						sess *SessionState
					}{key, session})
				}
				return true
			})
			for _, item := range toDelete {
				c.logger.Debug("Cleaning up stale session", "session_id", item.sess.SessionID)
				item.sess.mu.Lock()
				item.sess.Closing = true
				item.sess.mu.Unlock()
				c.xidIndex.Delete(item.sess.XID)
				c.sessions.Delete(item.key)
				c.sessionIndex.Delete(item.sess.SessionID)
				c.removeSessionFromIndexes(item.sess)
			}

			for _, item := range toDelete {
				sess := item.sess
				sessID := sess.SessionID

				if sess.IPv4 != nil {
					allocator.GetGlobalRegistry().ReleaseIP(sess.IPv4)
				}
				if sess.IPv6Address != nil {
					allocator.GetGlobalRegistry().ReleaseIANAByIP(sess.IPv6Address)
				}
				if sess.IPv6Prefix != nil {
					allocator.GetGlobalRegistry().ReleasePDByPrefix(sess.IPv6Prefix)
				}
				for _, p := range c.dhcp4Providers {
					p.ReleaseLease(sess.MAC.String())
				}

				if c.vpp != nil && sess.IPoESwIfIndex != 0 {
					c.vpp.DeleteIPoESessionAsync(sess.MAC, sess.EncapIfIndex, sess.InnerVLAN, func(err error) {
						if err != nil {
							c.logger.Warn("Failed to delete stale IPoE session", "session_id", sessID, "error", err)
						}
					})
				}

				c.deleteSessionCheckpoint(sessID)

				c.publishSessionLifecycle(&models.IPoESession{
					SessionID:   sessID,
					State:       models.SessionStateReleased,
					AccessType:  string(models.AccessTypeIPoE),
					MAC:         sess.MAC,
					OuterVLAN:   sess.OuterVLAN,
					InnerVLAN:   sess.InnerVLAN,
					VRF:         sess.VRF,
					SRGName:     sess.SRGName,
					Username:    sess.Username,
					IPv4Address: sess.IPv4,
					IPv6Address: sess.IPv6Address,
				})
			}
		}
	}
}

func (c *Component) getLocalMAC(srgName string, encapIfIndex uint32) net.HardwareAddr {
	if c.srgMgr != nil {
		if vmac := c.srgMgr.GetVirtualMAC(srgName); vmac != nil {
			return vmac
		}
	}
	// Walk up the SupSwIfIndex chain because sub-interfaces in VPP report
	// a zero L2Address from swInterfaceDump — the physical MAC lives on
	// the parent. Without this, the per-session rewrite ends up with a
	// zero source MAC on restore-mode bring-up after VPP recovery (and
	// likely also on fresh bring-up against access sub-interfaces).
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

func (c *Component) getSessionMode(svlan, cvlan uint16) subscriber.SessionMode {
	match, ok := c.cfgMgr.LookupSubscriberGroup(svlan, cvlan)
	if !ok {
		return subscriber.SessionModeUnified
	}
	return match.Group.GetSessionMode()
}

func (c *Component) makeSessionKeyV4(mac net.HardwareAddr, svlan, cvlan uint16) string {
	mode := c.getSessionMode(svlan, cvlan)
	if mode == subscriber.SessionModeUnified {
		return fmt.Sprintf("ipoe:%s:%d:%d", mac.String(), svlan, cvlan)
	}
	return fmt.Sprintf("ipoe-v4:%s:%d:%d", mac.String(), svlan, cvlan)
}

func (c *Component) makeSessionKeyV6(mac net.HardwareAddr, svlan, cvlan uint16) string {
	mode := c.getSessionMode(svlan, cvlan)
	if mode == subscriber.SessionModeUnified {
		return fmt.Sprintf("ipoe:%s:%d:%d", mac.String(), svlan, cvlan)
	}
	return fmt.Sprintf("ipoe-v6:%s:%d:%d", mac.String(), svlan, cvlan)
}

func (c *Component) sessionCount() int {
	n := 0
	c.sessions.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}

func (c *Component) ForEachSession(fn func(models.SubscriberSession) bool) {
	c.sessions.Range(func(_, v any) bool {
		sess := v.(*SessionState)
		sess.mu.Lock()
		if sess.State != "bound" {
			sess.mu.Unlock()
			return true
		}

		snapshot := &models.IPoESession{
			SessionID:     sess.SessionID,
			State:         models.SessionStateActive,
			AccessType:    string(models.AccessTypeIPoE),
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
			ClientID:      sess.ClientID,
			Hostname:      sess.Hostname,
			Username:      sess.Username,
			AAASessionID:  sess.AcctSessionID,
			ActivatedAt:   sess.ActivatedAt,
			Attributes:    sess.Attributes,
			RelayInfo:     map[uint8][]byte{},
		}
		if sess.AllocCtx != nil {
			snapshot.IPv4Pool = sess.AllocCtx.AllocatedPool
			snapshot.IANAPool = sess.AllocCtx.AllocatedIANAPool
			snapshot.PDPool = sess.AllocCtx.AllocatedPDPool
		}
		if sess.IPv6Prefix != nil {
			snapshot.IPv6Prefix = sess.IPv6Prefix.String()
		}
		if len(sess.CircuitID) > 0 {
			snapshot.RelayInfo[1] = sess.CircuitID
		}
		if len(sess.RemoteID) > 0 {
			snapshot.RelayInfo[2] = sess.RemoteID
		}
		sess.mu.Unlock()

		return fn(snapshot)
	})
}
