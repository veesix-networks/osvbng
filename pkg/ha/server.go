// Copyright 2025 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"context"
	"io"
	"log/slog"
	"time"

	hapb "github.com/veesix-networks/osvbng/api/proto/ha"
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

		transition := sm.Switchover()
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
