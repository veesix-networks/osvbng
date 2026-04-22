// Copyright 2025 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"

	hapb "github.com/veesix-networks/osvbng/api/proto/ha"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
)

type SyncSender struct {
	peer     *PeerClient
	backlogs map[string]*SyncBacklog
	seqNums  map[string]*atomic.Uint64
	counters map[string]*syncCounters
	sendCh   chan *hapb.SyncSessionRequest
	logger   *logger.Logger
	active   atomic.Bool
	mu       sync.RWMutex
}

type syncCounters struct {
	creates    atomic.Uint64
	updates    atomic.Uint64
	deletes    atomic.Uint64
	lastSendNs atomic.Int64
}

func NewSyncSender(peer *PeerClient, backlogSize int, srgNames []string, logger *logger.Logger) *SyncSender {
	backlogs := make(map[string]*SyncBacklog, len(srgNames))
	seqNums := make(map[string]*atomic.Uint64, len(srgNames))
	ctrs := make(map[string]*syncCounters, len(srgNames))
	for _, name := range srgNames {
		backlogs[name] = NewSyncBacklog(backlogSize)
		seqNums[name] = &atomic.Uint64{}
		ctrs[name] = &syncCounters{}
	}
	return &SyncSender{
		peer:     peer,
		backlogs: backlogs,
		seqNums:  seqNums,
		counters: ctrs,
		sendCh:   make(chan *hapb.SyncSessionRequest, 1024),
		logger:   logger,
	}
}

func (s *SyncSender) SetActive(active bool) {
	s.active.Store(active)
}

func (s *SyncSender) HandleEvent(ev events.Event) {
	if !s.active.Load() {
		return
	}

	data, ok := ev.Data.(*events.SessionLifecycleEvent)
	if !ok {
		return
	}

	sess, ok := data.Session.(models.SubscriberSession)
	if !ok {
		return
	}

	srgName := sess.GetSRGName()
	if srgName == "" {
		return
	}

	s.mu.RLock()
	backlog, hasBacklog := s.backlogs[srgName]
	seqCounter, hasSeq := s.seqNums[srgName]
	ctr := s.counters[srgName]
	s.mu.RUnlock()
	if !hasBacklog || !hasSeq {
		return
	}

	var action hapb.SyncAction
	switch data.State {
	case models.SessionStateReleased:
		action = hapb.SyncAction_SYNC_ACTION_DELETE
		ctr.deletes.Add(1)
	default:
		action = hapb.SyncAction_SYNC_ACTION_UPDATE
		ctr.updates.Add(1)
	}

	seq := seqCounter.Add(1)

	req := &hapb.SyncSessionRequest{
		SrgName:  srgName,
		Sequence: seq,
		Action:   action,
		Session:  sessionToCheckpoint(sess),
	}

	backlog.Push(req)

	select {
	case s.sendCh <- req:
	default:
	}
}

func (s *SyncSender) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-s.sendCh:
			if !s.active.Load() {
				continue
			}
			sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			_, err := s.peer.SyncSession(sendCtx, req)
			cancel()
			if err != nil {
				s.logger.Debug("Sync send failed", "srg", req.SrgName, "seq", req.Sequence, "error", err)
			} else if c := s.counters[req.SrgName]; c != nil {
				c.lastSendNs.Store(time.Now().UnixNano())
			}
		}
	}
}

func (s *SyncSender) GetBacklog(srgName string) *SyncBacklog {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.backlogs[srgName]
}

func (s *SyncSender) GetSeq(srgName string) uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if c, ok := s.seqNums[srgName]; ok {
		return c.Load()
	}
	return 0
}

func (s *SyncSender) GetCounts(srgName string) (creates, updates, deletes uint64) {
	s.mu.RLock()
	c := s.counters[srgName]
	s.mu.RUnlock()
	if c == nil {
		return 0, 0, 0
	}
	return c.creates.Load(), c.updates.Load(), c.deletes.Load()
}

func (s *SyncSender) GetLastSendTime(srgName string) time.Time {
	s.mu.RLock()
	c := s.counters[srgName]
	s.mu.RUnlock()
	if c == nil {
		return time.Time{}
	}
	ns := c.lastSendNs.Load()
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}

func (s *SyncSender) HandleMutationResult(ev events.Event) {
	data, ok := ev.Data.(*events.SubscriberMutationResultEvent)
	if !ok || !data.Ok || !s.active.Load() || data.Session == nil {
		return
	}

	srgName := data.Session.GetSRGName()
	if srgName == "" {
		return
	}

	s.mu.RLock()
	backlog, hasBacklog := s.backlogs[srgName]
	seqCounter, hasSeq := s.seqNums[srgName]
	ctr := s.counters[srgName]
	s.mu.RUnlock()
	if !hasBacklog || !hasSeq {
		return
	}

	seq := seqCounter.Add(1)
	req := &hapb.SyncSessionRequest{
		SrgName:  srgName,
		Sequence: seq,
		Action:   hapb.SyncAction_SYNC_ACTION_UPDATE,
		Session:  sessionToCheckpoint(data.Session),
	}

	backlog.Push(req)
	if ctr != nil {
		ctr.updates.Add(1)
	}

	select {
	case s.sendCh <- req:
	default:
	}
}

func sessionToCheckpoint(sess models.SubscriberSession) *hapb.SessionCheckpoint {
	cp := &hapb.SessionCheckpoint{
		SessionId:    sess.GetSessionID(),
		SrgName:      sess.GetSRGName(),
		Mac:          []byte(sess.GetMAC()),
		OuterVlan:    uint32(sess.GetOuterVLAN()),
		InnerVlan:    uint32(sess.GetInnerVLAN()),
		Username:     sess.GetUsername(),
		AaaSessionId: sess.GetAAASessionID(),
		ServiceGroup: sess.GetServiceGroup(),
	}

	if ip := sess.GetIPv4Address(); ip != nil {
		cp.Ipv4Address = ip.To4()
	}
	if ip := sess.GetIPv6Address(); ip != nil {
		cp.Ipv6Address = ip.To16()
	}

	switch s := sess.(type) {
	case *models.IPoESession:
		cp.AccessType = "ipoe"
		cp.Vrf = s.VRF
		cp.Ipv4LeaseTime = s.LeaseTime
		cp.Ipv6LeaseTime = s.IPv6LeaseTime
		cp.ClientId = s.ClientID
		cp.Hostname = s.Hostname
		cp.Dhcpv6Duid = s.DUID
		cp.Ipv4Pool = s.IPv4Pool
		cp.IanaPool = s.IANAPool
		cp.PdPool = s.PDPool
		cp.OuterTpid = uint32(s.OuterTPID)
		cp.AaaAttributes = s.Attributes
		if !s.ActivatedAt.IsZero() {
			cp.BoundAtNs = s.ActivatedAt.UnixNano()
		}
		if relayCircuitID, ok := s.RelayInfo[1]; ok {
			cp.CircuitId = relayCircuitID
		}
		if relayRemoteID, ok := s.RelayInfo[2]; ok {
			cp.RemoteId = relayRemoteID
		}
		if s.IPv6Prefix != "" {
			_, ipNet, err := net.ParseCIDR(s.IPv6Prefix)
			if err == nil {
				cp.Ipv6Prefix = ipNet.IP.To16()
				ones, _ := ipNet.Mask.Size()
				cp.Ipv6PrefixLen = uint32(ones)
			}
		}
	case *models.PPPSession:
		cp.AccessType = "pppoe"
		cp.Vrf = s.VRF
		cp.PppoeSessionId = uint32(s.PPPSessionID)
		cp.LcpState = s.LCPState
		cp.IpcpState = s.IPCPState
		cp.Ipv6CpState = s.IPv6CPState
		cp.Ipv4Pool = s.IPv4Pool
		cp.IanaPool = s.IANAPool
		cp.OuterTpid = uint32(s.OuterTPID)
		cp.LcpMagic = s.LCPMagic
		cp.AaaAttributes = s.Attributes
		cp.NegotiatedPppMtu = uint32(s.NegotiatedPPPMTU)
		cp.Ipv4Mss = uint32(s.IPv4MSS)
		cp.Ipv6Mss = uint32(s.IPv6MSS)
		if !s.ActivatedAt.IsZero() {
			cp.BoundAtNs = s.ActivatedAt.UnixNano()
		}
		if s.IPv6Prefix != "" {
			_, ipNet, err := net.ParseCIDR(s.IPv6Prefix)
			if err == nil {
				cp.Ipv6Prefix = ipNet.IP.To16()
				ones, _ := ipNet.Mask.Size()
				cp.Ipv6PrefixLen = uint32(ones)
			}
		}
	}

	return cp
}
