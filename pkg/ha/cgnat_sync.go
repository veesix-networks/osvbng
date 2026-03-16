// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	hapb "github.com/veesix-networks/osvbng/api/proto/ha"
	"github.com/veesix-networks/osvbng/pkg/models"
)

type CGNATSyncCallback func(srgName string, mapping *models.CGNATMapping, isAdd bool)

type CGNATSyncSender struct {
	peer     *PeerClient
	backlogs map[string]*SyncBacklog
	seqNums  map[string]*atomic.Uint64
	active   map[string]*atomic.Bool
	sendCh   chan *hapb.SyncCGNATMappingRequest
	logger   *slog.Logger
	mu       sync.RWMutex

	creates    atomic.Uint64
	deletes    atomic.Uint64
	lastSendNs atomic.Int64
}

func NewCGNATSyncSender(peer *PeerClient, backlogSize int, srgNames []string, logger *slog.Logger) *CGNATSyncSender {
	backlogs := make(map[string]*SyncBacklog, len(srgNames))
	seqNums := make(map[string]*atomic.Uint64, len(srgNames))
	activeMap := make(map[string]*atomic.Bool, len(srgNames))
	for _, name := range srgNames {
		backlogs[name] = NewSyncBacklog(backlogSize)
		seqNums[name] = &atomic.Uint64{}
		activeMap[name] = &atomic.Bool{}
	}
	return &CGNATSyncSender{
		peer:     peer,
		backlogs: backlogs,
		seqNums:  seqNums,
		active:   activeMap,
		sendCh:   make(chan *hapb.SyncCGNATMappingRequest, 1024),
		logger:   logger,
	}
}

func (s *CGNATSyncSender) SetActive(srgName string, active bool) {
	s.mu.RLock()
	flag, ok := s.active[srgName]
	s.mu.RUnlock()
	if ok {
		flag.Store(active)
	}
}

func (s *CGNATSyncSender) Send(srgName string, mapping *models.CGNATMapping, isAdd bool) {
	s.mu.RLock()
	flag, hasFlag := s.active[srgName]
	seqCounter, hasSeq := s.seqNums[srgName]
	s.mu.RUnlock()

	if !hasFlag || !hasSeq || !flag.Load() {
		return
	}

	var action hapb.SyncAction
	if isAdd {
		action = hapb.SyncAction_SYNC_ACTION_CREATE
		s.creates.Add(1)
	} else {
		action = hapb.SyncAction_SYNC_ACTION_DELETE
		s.deletes.Add(1)
	}

	seq := seqCounter.Add(1)

	req := &hapb.SyncCGNATMappingRequest{
		SrgName:  srgName,
		Sequence: seq,
		Action:   action,
		Mapping:  mappingToCheckpoint(srgName, mapping),
	}

	select {
	case s.sendCh <- req:
	default:
	}
}

func (s *CGNATSyncSender) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-s.sendCh:
			s.mu.RLock()
			flag := s.active[req.SrgName]
			s.mu.RUnlock()
			if flag != nil && !flag.Load() {
				continue
			}
			sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			_, err := s.peer.SyncCGNATMapping(sendCtx, req)
			cancel()
			if err != nil {
				s.logger.Debug("CGNAT sync send failed", "srg", req.SrgName, "seq", req.Sequence, "error", err)
			} else {
				s.lastSendNs.Store(time.Now().UnixNano())
			}
		}
	}
}

func (s *CGNATSyncSender) GetSeq(srgName string) uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if c, ok := s.seqNums[srgName]; ok {
		return c.Load()
	}
	return 0
}

func (s *CGNATSyncSender) GetCounts() (creates, deletes uint64) {
	return s.creates.Load(), s.deletes.Load()
}

func (s *CGNATSyncSender) GetLastSendTime() time.Time {
	ns := s.lastSendNs.Load()
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}

func mappingToCheckpoint(srgName string, m *models.CGNATMapping) *hapb.CGNATMappingCheckpoint {
	cp := &hapb.CGNATMappingCheckpoint{
		SessionId:      m.SessionID,
		SrgName:        srgName,
		PoolName:       m.PoolName,
		PortBlockStart: uint32(m.PortBlockStart),
		PortBlockEnd:   uint32(m.PortBlockEnd),
		InsideVrfId:    m.InsideVRFID,
		SwIfIndex:      m.SwIfIndex,
	}
	if m.InsideIP != nil {
		cp.InsideIp = m.InsideIP.To4()
	}
	if m.OutsideIP != nil {
		cp.OutsideIp = m.OutsideIP.To4()
	}
	return cp
}

func checkpointToMapping(cp *hapb.CGNATMappingCheckpoint) *models.CGNATMapping {
	return &models.CGNATMapping{
		SessionID:      cp.SessionId,
		PoolName:       cp.PoolName,
		InsideIP:       net.IP(cp.InsideIp),
		OutsideIP:      net.IP(cp.OutsideIp),
		PortBlockStart: uint16(cp.PortBlockStart),
		PortBlockEnd:   uint16(cp.PortBlockEnd),
		InsideVRFID:    cp.InsideVrfId,
		SwIfIndex:      cp.SwIfIndex,
	}
}
