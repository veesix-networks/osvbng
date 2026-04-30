// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	hapb "github.com/veesix-networks/osvbng/api/proto/ha"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/netbind"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// connectReadyTimeout caps how long PeerClient.Connect waits for the
// gRPC connection to reach Ready. ConnectWithBackoff retries on top of
// this with exponential backoff, so set it short enough that a flapping
// VRF master does not block the heartbeat loop for long.
const connectReadyTimeout = 5 * time.Second

type PeerState struct {
	Connected     bool
	LastHeartbeat time.Time
	RTT           time.Duration
	ClockSkew     time.Duration
	NodeID        string
}

type PeerClient struct {
	address   string
	binding   netbind.Binding
	extraOpts []grpc.DialOption
	logger    *logger.Logger

	conn   *grpc.ClientConn
	client hapb.HAPeerServiceClient
	stream hapb.HAPeerService_HeartbeatClient

	state PeerState
	mu    sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
}

// NewPeerClient stores the binding so every reconnect re-applies
// GRPCDialOpts(b) — SO_BINDTODEVICE survives the lifetime of one TCP
// connection, so a reset must allocate a new one with the bound dialer.
// extraOpts carry credentials (TLS or insecure) chosen by the caller.
func NewPeerClient(address string, binding netbind.Binding, extraOpts []grpc.DialOption, logger *logger.Logger) *PeerClient {
	ctx, cancel := context.WithCancel(context.Background())
	return &PeerClient{
		address:   address,
		binding:   binding,
		extraOpts: extraOpts,
		logger:    logger,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Connect builds a fresh ClientConn through netbind.GRPCDialOpts(b) and
// waits for the gRPC connection to reach Ready (or fail) within
// connectReadyTimeout. Earlier behaviour treated grpc.NewClient
// allocation as reachability, which masked VRF flap because NewClient
// returns immediately.
func (p *PeerClient) Connect() error {
	opts := append([]grpc.DialOption{}, p.extraOpts...)
	opts = append(opts, netbind.GRPCDialOpts(p.binding)...)

	conn, err := grpc.NewClient(p.address, opts...)
	if err != nil {
		return fmt.Errorf("grpc.NewClient %s: %w", p.address, err)
	}

	conn.Connect()

	ctx, cancel := context.WithTimeout(p.ctx, connectReadyTimeout)
	defer cancel()

	for {
		s := conn.GetState()
		if s == connectivity.Ready {
			break
		}
		if s == connectivity.Shutdown {
			_ = conn.Close()
			return fmt.Errorf("grpc connection to %s entered Shutdown", p.address)
		}
		if !conn.WaitForStateChange(ctx, s) {
			_ = conn.Close()
			return fmt.Errorf("grpc connection to %s did not reach Ready within %s (last state: %s)",
				p.address, connectReadyTimeout, s)
		}
	}

	p.mu.Lock()
	if p.conn != nil {
		_ = p.conn.Close()
	}
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
			p.logger.Debug("Peer connection failed, retrying", "address", p.address, "binding", p.binding, "backoff", backoff, "error", err)
			select {
			case <-time.After(backoff):
			case <-p.ctx.Done():
				return
			}
			backoff = time.Duration(math.Min(float64(backoff*2), float64(maxBackoff)))
			continue
		}

		p.logger.Info("Connected to peer", "address", p.address, "binding", p.binding)
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

// CloseConn drops just the underlying ClientConn (not the PeerClient
// itself). Used by the heartbeat reconnect path so the next Connect
// allocates a fresh ClientConn instead of overwriting a leaked one.
func (p *PeerClient) CloseConn() {
	p.mu.Lock()
	if p.stream != nil {
		_ = p.stream.CloseSend()
		p.stream = nil
	}
	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
		p.client = nil
	}
	p.state.Connected = false
	p.mu.Unlock()
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
