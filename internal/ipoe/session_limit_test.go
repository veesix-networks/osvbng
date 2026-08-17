// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ipoe

import (
	"fmt"
	"net"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/aaa"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

const (
	limitSVLAN = 100
	limitCVLAN = 10
)

func sessionLimitComponent(t *testing.T, max int, mode subscriber.SessionMode) *Component {
	t.Helper()

	cfg := &config.Config{
		AAA: aaa.AAAConfig{
			Policy: []aaa.AAAPolicy{{Name: "dhcp-pol", MaxConcurrentSessions: max}},
		},
		SubscriberGroups: &subscriber.SubscriberGroupsConfig{
			Groups: map[string]*subscriber.SubscriberGroup{
				"grp": {
					AAAPolicy:   "dhcp-pol",
					SessionMode: mode,
					VLANs:       []subscriber.VLANRange{{SVLAN: "100", CVLAN: "10"}},
				},
			},
		},
	}

	return &Component{
		logger: logger.NewTest(),
		cfgMgr: &fakeConfigManager{cfg: cfg},
	}
}

func boundV4Session(c *Component, mac net.HardwareAddr, ip string) *SessionState {
	sess := &SessionState{
		SessionID: "v4-" + mac.String(),
		MAC:       mac,
		OuterVLAN: limitSVLAN,
		InnerVLAN: limitCVLAN,
		State:     "bound",
		IPv4:      net.ParseIP(ip),
	}
	c.sessions.Store(c.makeSessionKeyV4(mac, limitSVLAN, limitCVLAN), sess)
	return sess
}

// TestSessionLimitDerivesFromLiveSessions pins the admission count to the
// live session map. It used to come from a separate cache counter that only
// the DHCP RELEASE handlers decremented, so a subscriber torn down any other
// way (the stale-session reaper, a CoA terminate) leaked its count and was
// locked out until the key's TTL expired. BNG Blaster monkey churn kills
// sessions without a RELEASE, which drained every subscriber one by one.
func TestSessionLimitDerivesFromLiveSessions(t *testing.T) {
	mac := net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x00, 0x57}

	t.Run("live_bound_session_blocks_a_second", func(t *testing.T) {
		c := sessionLimitComponent(t, 1, subscriber.SessionModeUnified)
		boundV4Session(c, mac, "10.0.0.5")

		if err := c.checkSessionLimit(mac, limitSVLAN, limitCVLAN); err == nil {
			t.Fatal("expected the limit to reject while a bound session is live")
		}
	})

	t.Run("reaped_session_admits_the_next_discover", func(t *testing.T) {
		c := sessionLimitComponent(t, 1, subscriber.SessionModeUnified)
		sess := boundV4Session(c, mac, "10.0.0.5")

		// Exactly what cleanupSessions does, and nothing more: no RELEASE
		// was seen, so no handler ran to release the subscriber's count.
		sess.Closing = true
		c.sessions.Delete(c.makeSessionKeyV4(mac, limitSVLAN, limitCVLAN))

		if err := c.checkSessionLimit(mac, limitSVLAN, limitCVLAN); err != nil {
			t.Fatalf("reaped session must not block re-establishment: %v", err)
		}
	})

	t.Run("closing_session_does_not_block", func(t *testing.T) {
		c := sessionLimitComponent(t, 1, subscriber.SessionModeUnified)
		sess := boundV4Session(c, mac, "10.0.0.5")

		// The reaper marks Closing under sess.mu before it deletes from the
		// map; a DISCOVER landing in that window must not be rejected.
		sess.Closing = true

		if err := c.checkSessionLimit(mac, limitSVLAN, limitCVLAN); err != nil {
			t.Fatalf("closing session must not block re-establishment: %v", err)
		}
	})

	t.Run("independent_mode_dual_stack_counts_once", func(t *testing.T) {
		c := sessionLimitComponent(t, 1, subscriber.SessionModeIndependent)
		boundV4Session(c, mac, "10.0.0.5")

		// In independent mode the same subscriber holds a second SessionState
		// for IPv6 under its own key. The limit counts IPv4 bindings, so the
		// v6 half must not consume the subscriber's single allowed session.
		v6 := &SessionState{
			SessionID:   "v6-" + mac.String(),
			MAC:         mac,
			OuterVLAN:   limitSVLAN,
			InnerVLAN:   limitCVLAN,
			State:       "bound",
			IPv6Address: net.ParseIP("2001:db8::1"),
			IPv6Bound:   true,
		}
		c.sessions.Store(c.makeSessionKeyV6(mac, limitSVLAN, limitCVLAN), v6)

		if got := c.countExistingSessions(mac, limitSVLAN, limitCVLAN); got != 1 {
			t.Fatalf("dual-stack subscriber counted %d times, want 1", got)
		}
	})

	t.Run("other_subscribers_are_not_counted", func(t *testing.T) {
		c := sessionLimitComponent(t, 1, subscriber.SessionModeUnified)
		boundV4Session(c, net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x00, 0x58}, "10.0.0.6")

		if err := c.checkSessionLimit(mac, limitSVLAN, limitCVLAN); err != nil {
			t.Fatalf("another subscriber's session must not block: %v", err)
		}
	})

	t.Run("unbound_session_does_not_block", func(t *testing.T) {
		c := sessionLimitComponent(t, 1, subscriber.SessionModeUnified)
		// AAA approved, DISCOVER in flight, no lease yet: nothing to count.
		c.sessions.Store(c.makeSessionKeyV4(mac, limitSVLAN, limitCVLAN), &SessionState{
			SessionID: "pending",
			MAC:       mac,
			OuterVLAN: limitSVLAN,
			InnerVLAN: limitCVLAN,
			State:     "discovering",
		})

		if err := c.checkSessionLimit(mac, limitSVLAN, limitCVLAN); err != nil {
			t.Fatalf("session without an IPv4 binding must not block: %v", err)
		}
	})

	t.Run("session_under_other_mode_key_still_counts", func(t *testing.T) {
		c := sessionLimitComponent(t, 1, subscriber.SessionModeIndependent)

		// A session established while the group ran in unified mode sits
		// under the "ipoe:" key. If the group is then reconfigured to
		// independent mode, a fresh DISCOVER computes the "ipoe-v4:" key
		// and misses it, but the subscriber still holds a live IPv4 lease
		// and must still count against the limit.
		sess := &SessionState{
			SessionID: "unified-" + mac.String(),
			MAC:       mac,
			OuterVLAN: limitSVLAN,
			InnerVLAN: limitCVLAN,
			State:     "bound",
			IPv4:      net.ParseIP("10.0.0.5"),
		}
		c.sessions.Store(fmt.Sprintf("ipoe:%s:%d:%d", mac.String(), limitSVLAN, limitCVLAN), sess)

		if err := c.checkSessionLimit(mac, limitSVLAN, limitCVLAN); err == nil {
			t.Fatal("expected the limit to reject a live session stored under the other mode's key")
		}
	})

	t.Run("limit_disabled_admits_everything", func(t *testing.T) {
		c := sessionLimitComponent(t, 0, subscriber.SessionModeUnified)
		boundV4Session(c, mac, "10.0.0.5")

		if err := c.checkSessionLimit(mac, limitSVLAN, limitCVLAN); err != nil {
			t.Fatalf("max_concurrent_sessions=0 must disable the check: %v", err)
		}
	})
}
