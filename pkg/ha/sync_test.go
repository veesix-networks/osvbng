// Copyright 2025 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"net"
	"testing"
	"time"

	hapb "github.com/veesix-networks/osvbng/api/proto/ha"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestSender(srgNames ...string) *SyncSender {
	return NewSyncSender(nil, 100, srgNames, logger.NewTest())
}

func makeLifecycleEvent(sessID, srgName string, state models.SessionState, accessType models.AccessType) events.Event {
	var sess models.SubscriberSession
	switch accessType {
	case models.AccessTypePPPoE:
		sess = &models.PPPSession{
			SessionID:    sessID,
			State:        state,
			MAC:          net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
			OuterVLAN:    100,
			InnerVLAN:    200,
			VRF:          "default",
			SRGName:      srgName,
			ServiceGroup: "sg1",
			Username:     "user@example.com",
			PPPSessionID: 42,
			LCPState:     "Opened",
			IPCPState:    "Opened",
			IPv4Address:  net.ParseIP("10.0.0.1"),
			ActivatedAt:  time.Now(),
		}
	default:
		sess = &models.IPoESession{
			SessionID:    sessID,
			State:        state,
			MAC:          net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
			OuterVLAN:    100,
			InnerVLAN:    200,
			VRF:          "default",
			SRGName:      srgName,
			ServiceGroup: "sg1",
			Username:     "user@example.com",
			IPv4Address:  net.ParseIP("10.0.0.1"),
			LeaseTime:    3600,
			ActivatedAt:  time.Now(),
		}
	}

	return events.Event{
		Data: &events.SessionLifecycleEvent{
			AccessType: accessType,
			SessionID:  sessID,
			State:      state,
			Session:    sess,
		},
	}
}

func TestSyncSender_InactiveSkipsEvents(t *testing.T) {
	s := newTestSender("srg1")
	ev := makeLifecycleEvent("sess1", "srg1", models.SessionStateActive, models.AccessTypeIPoE)
	s.HandleEvent(ev)

	assert.Equal(t, uint64(0), s.GetSeq("srg1"))
	assert.Equal(t, 0, s.GetBacklog("srg1").Size())
}

func TestSyncSender_ActivePushesToBacklog(t *testing.T) {
	s := newTestSender("srg1")
	s.SetActive(true)

	ev := makeLifecycleEvent("sess1", "srg1", models.SessionStateActive, models.AccessTypeIPoE)
	s.HandleEvent(ev)

	assert.Equal(t, uint64(1), s.GetSeq("srg1"))
	assert.Equal(t, 1, s.GetBacklog("srg1").Size())

	// Drain channel and verify
	select {
	case req := <-s.sendCh:
		assert.Equal(t, "srg1", req.SrgName)
		assert.Equal(t, uint64(1), req.Sequence)
		assert.Equal(t, hapb.SyncAction_SYNC_ACTION_UPDATE, req.Action)
		assert.Equal(t, "sess1", req.Session.SessionId)
		assert.Equal(t, "ipoe", req.Session.AccessType)
	default:
		t.Fatal("expected message on sendCh")
	}
}

func TestSyncSender_IncrementalSeq(t *testing.T) {
	s := newTestSender("srg1")
	s.SetActive(true)

	for i := 0; i < 5; i++ {
		ev := makeLifecycleEvent("sess1", "srg1", models.SessionStateActive, models.AccessTypeIPoE)
		s.HandleEvent(ev)
	}

	assert.Equal(t, uint64(5), s.GetSeq("srg1"))
	assert.Equal(t, 5, s.GetBacklog("srg1").Size())
}

func TestSyncSender_PerSRGSequencing(t *testing.T) {
	s := newTestSender("srg1", "srg2")
	s.SetActive(true)

	s.HandleEvent(makeLifecycleEvent("s1", "srg1", models.SessionStateActive, models.AccessTypeIPoE))
	s.HandleEvent(makeLifecycleEvent("s2", "srg1", models.SessionStateActive, models.AccessTypeIPoE))
	s.HandleEvent(makeLifecycleEvent("s3", "srg2", models.SessionStateActive, models.AccessTypeIPoE))

	assert.Equal(t, uint64(2), s.GetSeq("srg1"))
	assert.Equal(t, uint64(1), s.GetSeq("srg2"))
}

func TestSyncSender_DeleteAction(t *testing.T) {
	s := newTestSender("srg1")
	s.SetActive(true)

	ev := makeLifecycleEvent("sess1", "srg1", models.SessionStateReleased, models.AccessTypeIPoE)
	s.HandleEvent(ev)

	select {
	case req := <-s.sendCh:
		assert.Equal(t, hapb.SyncAction_SYNC_ACTION_DELETE, req.Action)
	default:
		t.Fatal("expected message on sendCh")
	}
}

func TestSyncSender_UnknownSRGIgnored(t *testing.T) {
	s := newTestSender("srg1")
	s.SetActive(true)

	ev := makeLifecycleEvent("sess1", "srg-unknown", models.SessionStateActive, models.AccessTypeIPoE)
	s.HandleEvent(ev)

	assert.Equal(t, uint64(0), s.GetSeq("srg1"))
}

func TestSyncSender_NoSRGNameIgnored(t *testing.T) {
	s := newTestSender("srg1")
	s.SetActive(true)

	ev := makeLifecycleEvent("sess1", "", models.SessionStateActive, models.AccessTypeIPoE)
	s.HandleEvent(ev)

	assert.Equal(t, uint64(0), s.GetSeq("srg1"))
}

func TestSyncSender_PPPoECheckpoint(t *testing.T) {
	s := newTestSender("srg1")
	s.SetActive(true)

	ev := makeLifecycleEvent("ppp1", "srg1", models.SessionStateActive, models.AccessTypePPPoE)
	s.HandleEvent(ev)

	select {
	case req := <-s.sendCh:
		assert.Equal(t, "pppoe", req.Session.AccessType)
		assert.Equal(t, uint32(42), req.Session.PppoeSessionId)
		assert.Equal(t, "Opened", req.Session.LcpState)
		assert.Equal(t, "Opened", req.Session.IpcpState)
	default:
		t.Fatal("expected message on sendCh")
	}
}

func TestSessionToCheckpoint_IPoE(t *testing.T) {
	sess := &models.IPoESession{
		SessionID:    "ipoe:aa:bb:cc:dd:ee:ff:100",
		State:        models.SessionStateActive,
		MAC:          net.HardwareAddr{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff},
		OuterVLAN:    100,
		InnerVLAN:    200,
		VRF:          "vrf-1",
		SRGName:      "srg1",
		ServiceGroup: "sg-residential",
		Username:     "user@isp.com",
		IPv4Address:  net.ParseIP("10.0.0.5"),
		LeaseTime:    7200,
		IPv6Address:  net.ParseIP("2001:db8::5"),
		IPv6Prefix:   "2001:db8:1::/48",
		IPv6LeaseTime: 3600,
		ClientID:     []byte{0x01, 0x02},
		Hostname:     "cpe-1",
		DUID:         []byte{0x00, 0x03, 0x00, 0x01},
		RelayInfo:    map[uint8][]byte{1: {0x10, 0x20}, 2: {0x30, 0x40}},
		ActivatedAt:  time.Unix(1000000, 0),
		IPv4Pool:     "residential/pool-1",
		IANAPool:     "residential-v6/iana-1",
		PDPool:       "residential-v6/pd-1",
		OuterTPID:    0x88a8,
	}

	cp := sessionToCheckpoint(sess)
	assert.Equal(t, "ipoe", cp.AccessType)
	assert.Equal(t, "vrf-1", cp.Vrf)
	assert.Equal(t, "sg-residential", cp.ServiceGroup)
	assert.Equal(t, uint32(100), cp.OuterVlan)
	assert.Equal(t, uint32(200), cp.InnerVlan)
	assert.Equal(t, net.ParseIP("10.0.0.5").To4(), net.IP(cp.Ipv4Address))
	assert.Equal(t, net.ParseIP("2001:db8::5").To16(), net.IP(cp.Ipv6Address))
	assert.Equal(t, uint32(7200), cp.Ipv4LeaseTime)
	assert.Equal(t, uint32(3600), cp.Ipv6LeaseTime)
	assert.Equal(t, []byte{0x01, 0x02}, cp.ClientId)
	assert.Equal(t, "cpe-1", cp.Hostname)
	assert.Equal(t, []byte{0x00, 0x03, 0x00, 0x01}, cp.Dhcpv6Duid)
	assert.Equal(t, []byte{0x10, 0x20}, cp.CircuitId)
	assert.Equal(t, []byte{0x30, 0x40}, cp.RemoteId)

	require.NotNil(t, cp.Ipv6Prefix)
	assert.Equal(t, uint32(48), cp.Ipv6PrefixLen)
	assert.Equal(t, time.Unix(1000000, 0).UnixNano(), cp.BoundAtNs)

	assert.Equal(t, "residential/pool-1", cp.Ipv4Pool)
	assert.Equal(t, "residential-v6/iana-1", cp.IanaPool)
	assert.Equal(t, "residential-v6/pd-1", cp.PdPool)
	assert.Equal(t, uint32(0x88a8), cp.OuterTpid)
}

func TestSessionToCheckpoint_PPPoE(t *testing.T) {
	sess := &models.PPPSession{
		SessionID:    "ppp:42",
		State:        models.SessionStateActive,
		MAC:          net.HardwareAddr{0x11, 0x22, 0x33, 0x44, 0x55, 0x66},
		OuterVLAN:    300,
		InnerVLAN:    400,
		VRF:          "vrf-2",
		SRGName:      "srg1",
		ServiceGroup: "sg-business",
		PPPSessionID: 42,
		LCPState:     "Opened",
		IPCPState:    "Opened",
		IPv6CPState:  "Opened",
		IPv4Address:  net.ParseIP("10.1.0.1"),
		IPv6Prefix:   "2001:db8:2::/56",
		ActivatedAt:  time.Unix(2000000, 0),
		IPv4Pool:     "business/pool-1",
		IANAPool:     "business-v6/iana-1",
		OuterTPID:    0x8100,
	}

	cp := sessionToCheckpoint(sess)
	assert.Equal(t, "pppoe", cp.AccessType)
	assert.Equal(t, "vrf-2", cp.Vrf)
	assert.Equal(t, uint32(42), cp.PppoeSessionId)
	assert.Equal(t, "Opened", cp.LcpState)
	assert.Equal(t, "Opened", cp.IpcpState)
	assert.Equal(t, "Opened", cp.Ipv6CpState)
	assert.Equal(t, uint32(56), cp.Ipv6PrefixLen)

	assert.Equal(t, "business/pool-1", cp.Ipv4Pool)
	assert.Equal(t, "business-v6/iana-1", cp.IanaPool)
	assert.Equal(t, uint32(0x8100), cp.OuterTpid)
}
