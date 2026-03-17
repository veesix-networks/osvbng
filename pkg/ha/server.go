// Copyright 2025 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"time"

	hapb "github.com/veesix-networks/osvbng/api/proto/ha"
	"github.com/veesix-networks/osvbng/pkg/models"
)

type HAPeerServer struct {
	hapb.UnimplementedHAPeerServiceServer

	manager *Manager
	logger  *slog.Logger
}

func NewHAPeerServer(manager *Manager, logger *slog.Logger) *HAPeerServer {
	return &HAPeerServer{
		manager: manager,
		logger:  logger,
	}
}

func (s *HAPeerServer) Heartbeat(stream hapb.HAPeerService_HeartbeatServer) error {
	s.logger.Info("Peer heartbeat stream opened")

	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			s.logger.Warn("Heartbeat stream error", "error", err)
			return err
		}

		s.manager.handlePeerHeartbeat(msg)

		reply := s.manager.buildHeartbeatMessage()
		if err := stream.Send(reply); err != nil {
			s.logger.Warn("Failed to send heartbeat reply", "error", err)
			return err
		}
	}
}

func (s *HAPeerServer) NotifySRGState(_ context.Context, notification *hapb.SRGStateNotification) (*hapb.SRGStateAck, error) {
	s.logger.Info("Received SRG state notification",
		"srg", notification.SrgName,
		"new_state", notification.NewState,
		"prev_state", notification.PrevState)

	return &hapb.SRGStateAck{Acknowledged: true}, nil
}

func (s *HAPeerServer) RequestSwitchover(_ context.Context, req *hapb.SwitchoverRequest) (*hapb.SwitchoverResponse, error) {
	s.logger.Info("Received switchover request", "srgs", req.SrgNames, "graceful", req.Graceful)

	for _, srgName := range req.SrgNames {
		sm, ok := s.manager.getSRG(srgName)
		if !ok {
			return &hapb.SwitchoverResponse{
				Success: false,
				Message: "unknown SRG: " + srgName,
			}, nil
		}

		transition := sm.Switchover(false)
		if transition != nil {
			s.manager.publishTransition(transition)
			s.logger.Info("SRG switchover executed",
				"srg", srgName,
				"old_state", string(transition.OldState),
				"new_state", string(transition.NewState))
		}
	}

	return &hapb.SwitchoverResponse{
		Success: true,
		Message: "switchover completed at " + time.Now().Format(time.RFC3339),
	}, nil
}

func (s *HAPeerServer) SyncSession(ctx context.Context, req *hapb.SyncSessionRequest) (*hapb.SyncSessionResponse, error) {
	if s.manager.syncReceiver == nil {
		return &hapb.SyncSessionResponse{Success: false}, nil
	}
	return s.manager.syncReceiver.HandleSyncSession(ctx, req)
}

func (s *HAPeerServer) BulkSync(req *hapb.BulkSyncRequest, stream hapb.HAPeerService_BulkSyncServer) error {
	if s.manager.syncSender == nil {
		return nil
	}

	s.logger.Info("Bulk sync requested", "srgs", req.SrgNames, "from_seq", req.FromSequence)

	srgNames := req.SrgNames
	if len(srgNames) == 0 {
		for name := range s.manager.srgs {
			srgNames = append(srgNames, name)
		}
	}

	pageSize := s.manager.cfg.GetSyncPageSize()

	for _, srgName := range srgNames {
		s.manager.IncrementBulkSync(srgName)
		backlog := s.manager.syncSender.GetBacklog(srgName)

		sentFromBacklog := false
		if backlog != nil {
			oldest := backlog.OldestSeq()
			newest := backlog.NewestSeq()
			if oldest != 0 && newest != 0 {
				sentFromBacklog = true
				entries := backlog.Range(oldest, newest)
				for i := 0; i < len(entries); i += pageSize {
					end := i + pageSize
					if end > len(entries) {
						end = len(entries)
					}

					page := make([]*hapb.SessionCheckpoint, 0, end-i)
					for _, entry := range entries[i:end] {
						if entry.Session != nil {
							page = append(page, entry.Session)
						}
					}

					resp := &hapb.BulkSyncResponse{
						SrgName:  srgName,
						Sessions: page,
						Sequence: entries[end-1].Sequence,
						LastPage: end >= len(entries),
					}
					if err := stream.Send(resp); err != nil {
						return err
					}
				}
			}
		}

		if !sentFromBacklog {
			if err := s.bulkSyncFromIterators(srgName, pageSize, stream); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *HAPeerServer) SyncCGNATMapping(ctx context.Context, req *hapb.SyncCGNATMappingRequest) (*hapb.SyncCGNATMappingResponse, error) {
	if s.manager.syncReceiver == nil {
		return &hapb.SyncCGNATMappingResponse{Success: false}, nil
	}
	return s.manager.syncReceiver.HandleSyncCGNATMapping(ctx, req)
}

func (s *HAPeerServer) BulkSyncCGNAT(req *hapb.BulkSyncCGNATRequest, stream hapb.HAPeerService_BulkSyncCGNATServer) error {
	if s.manager.opdbStore == nil {
		return nil
	}

	s.logger.Info("CGNAT bulk sync requested", "srgs", req.SrgNames)

	var snapshotSeq uint64
	if s.manager.cgnatSyncSender != nil {
		for _, name := range req.SrgNames {
			if seq := s.manager.cgnatSyncSender.GetSeq(name); seq > snapshotSeq {
				snapshotSeq = seq
			}
		}
	}

	pageSize := s.manager.cfg.GetSyncPageSize()
	var page []*hapb.CGNATMappingCheckpoint

	err := s.manager.opdbStore.Load(stream.Context(), "cgnat_mappings", func(key string, value []byte) error {
		var m models.CGNATMapping
		if err := json.Unmarshal(value, &m); err != nil {
			return nil
		}

		page = append(page, mappingToCheckpoint("", &m))

		if len(page) >= pageSize {
			if err := stream.Send(&hapb.BulkSyncCGNATResponse{
				Mappings: page,
				Sequence: snapshotSeq,
			}); err != nil {
				return err
			}
			page = nil
		}
		return nil
	})
	if err != nil {
		return err
	}

	return stream.Send(&hapb.BulkSyncCGNATResponse{
		Mappings: page,
		Sequence: snapshotSeq,
		LastPage: true,
	})
}

func (s *HAPeerServer) bulkSyncFromIterators(srgName string, pageSize int, stream hapb.HAPeerService_BulkSyncServer) error {
	s.manager.mu.RLock()
	iterators := s.manager.sessionIterators
	s.manager.mu.RUnlock()

	if len(iterators) == 0 {
		return stream.Send(&hapb.BulkSyncResponse{
			SrgName:  srgName,
			LastPage: true,
		})
	}

	var page []*hapb.SessionCheckpoint
	var seq uint64

	for _, iter := range iterators {
		iter.ForEachSession(func(sess models.SubscriberSession) bool {
			if sess.GetSRGName() != srgName {
				return true
			}

			seq++
			page = append(page, sessionToCheckpoint(sess))

			if len(page) >= pageSize {
				if err := stream.Send(&hapb.BulkSyncResponse{
					SrgName:  srgName,
					Sessions: page,
					Sequence: seq,
				}); err != nil {
					return false
				}
				page = nil
			}
			return true
		})
	}

	return stream.Send(&hapb.BulkSyncResponse{
		SrgName:  srgName,
		Sessions: page,
		Sequence: seq,
		LastPage: true,
	})
}
