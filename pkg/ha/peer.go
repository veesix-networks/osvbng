// Copyright 2025 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"time"

	hapb "github.com/veesix-networks/osvbng/api/proto/ha"
	"google.golang.org/grpc"
)

type PeerState struct {
	Connected     bool
	LastHeartbeat time.Time
	RTT           time.Duration
	ClockSkew     time.Duration
	NodeID        string
}

type PeerClient struct {
	address  string
	dialOpts []grpc.DialOption
	logger   *slog.Logger

	conn   *grpc.ClientConn
	client hapb.HAPeerServiceClient
	stream hapb.HAPeerService_HeartbeatClient

	state PeerState
	mu    sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
}

func NewPeerClient(address string, dialOpts []grpc.DialOption, logger *slog.Logger) *PeerClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &PeerClient{
		address:  address,
		dialOpts: dialOpts,
		logger:   logger,
		ctx:      ctx,
		cancel:   cancel,
	}
}

func (p *PeerClient) Connect() error {
	conn, err := grpc.NewClient(p.address, p.dialOpts...)
	if err != nil {
		return err
	}

	p.mu.Lock()
	p.conn = conn
	p.client = hapb.NewHAPeerServiceClient(conn)
	p.mu.Unlock()

	return nil
}

func (p *PeerClient) ConnectWithBackoff() {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-p.ctx.Done():
			return
		default:
		}

		if err := p.Connect(); err != nil {
			p.logger.Debug("Peer connection failed, retrying", "address", p.address, "backoff", backoff, "error", err)
			select {
			case <-time.After(backoff):
			case <-p.ctx.Done():
				return
			}
			backoff = time.Duration(math.Min(float64(backoff*2), float64(maxBackoff)))
			continue
		}

		p.logger.Info("Connected to peer", "address", p.address)
		return
	}
}

func (p *PeerClient) OpenHeartbeatStream() error {
	p.mu.RLock()
	client := p.client
	p.mu.RUnlock()

	if client == nil {
		return errNotConnected
	}

	stream, err := client.Heartbeat(p.ctx)
	if err != nil {
		return err
	}

	p.mu.Lock()
	p.stream = stream
	p.mu.Unlock()

	return nil
}

func (p *PeerClient) SendHeartbeat(msg *hapb.HeartbeatMessage) error {
	p.mu.RLock()
	stream := p.stream
	p.mu.RUnlock()

	if stream == nil {
		return errNoStream
	}

	return stream.Send(msg)
}

func (p *PeerClient) RecvHeartbeat() (*hapb.HeartbeatMessage, error) {
	p.mu.RLock()
	stream := p.stream
	p.mu.RUnlock()

	if stream == nil {
		return nil, errNoStream
	}

	msg, err := stream.Recv()
	if err != nil {
		p.mu.Lock()
		p.state.Connected = false
		p.stream = nil
		p.mu.Unlock()
		return nil, err
	}

	now := time.Now()
	rtt := now.Sub(time.Unix(0, msg.TimestampNs))
	skew := time.Duration(now.UnixNano()-msg.TimestampNs) - rtt/2

	p.mu.Lock()
	p.state.Connected = true
	p.state.LastHeartbeat = now
	p.state.RTT = rtt
	p.state.ClockSkew = skew
	p.state.NodeID = msg.NodeId
	p.mu.Unlock()

	return msg, nil
}

func (p *PeerClient) NotifySRGState(ctx context.Context, notification *hapb.SRGStateNotification) error {
	p.mu.RLock()
	client := p.client
	p.mu.RUnlock()

	if client == nil {
		return errNotConnected
	}

	_, err := client.NotifySRGState(ctx, notification)
	return err
}

func (p *PeerClient) RequestSwitchover(ctx context.Context, req *hapb.SwitchoverRequest) (*hapb.SwitchoverResponse, error) {
	p.mu.RLock()
	client := p.client
	p.mu.RUnlock()

	if client == nil {
		return nil, errNotConnected
	}

	return client.RequestSwitchover(ctx, req)
}

func (p *PeerClient) BulkSync(ctx context.Context, req *hapb.BulkSyncRequest) (hapb.HAPeerService_BulkSyncClient, error) {
	p.mu.RLock()
	client := p.client
	p.mu.RUnlock()

	if client == nil {
		return nil, errNotConnected
	}

	return client.BulkSync(ctx, req)
}

func (p *PeerClient) SyncSession(ctx context.Context, req *hapb.SyncSessionRequest) (*hapb.SyncSessionResponse, error) {
	p.mu.RLock()
	client := p.client
	p.mu.RUnlock()

	if client == nil {
		return nil, errNotConnected
	}

	return client.SyncSession(ctx, req)
}

func (p *PeerClient) SyncCGNATMapping(ctx context.Context, req *hapb.SyncCGNATMappingRequest) (*hapb.SyncCGNATMappingResponse, error) {
	p.mu.RLock()
	client := p.client
	p.mu.RUnlock()

	if client == nil {
		return nil, errNotConnected
	}

	return client.SyncCGNATMapping(ctx, req)
}

func (p *PeerClient) BulkSyncCGNAT(ctx context.Context, req *hapb.BulkSyncCGNATRequest) (hapb.HAPeerService_BulkSyncCGNATClient, error) {
	p.mu.RLock()
	client := p.client
	p.mu.RUnlock()

	if client == nil {
		return nil, errNotConnected
	}

	return client.BulkSyncCGNAT(ctx, req)
}

func (p *PeerClient) GetState() PeerState {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.state
}

func (p *PeerClient) Close() error {
	p.cancel()

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stream != nil {
		_ = p.stream.CloseSend()
		p.stream = nil
	}

	if p.conn != nil {
		err := p.conn.Close()
		p.conn = nil
		p.client = nil
		return err
	}

	return nil
}

var (
	errNotConnected = &peerError{"peer not connected"}
	errNoStream     = &peerError{"heartbeat stream not open"}
)

type peerError struct {
	msg string
}

func (e *peerError) Error() string {
	return e.msg
}
