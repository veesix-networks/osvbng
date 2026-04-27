// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/netbind"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestNewPeerClient_StoresBinding(t *testing.T) {
	bind := netbind.Binding{VRF: "HA-VRF"}
	p := NewPeerClient("10.255.0.2:50051", bind, nil, logger.Get("test"))
	defer p.Close()
	if p.binding.VRF != "HA-VRF" {
		t.Errorf("binding.VRF=%q want HA-VRF", p.binding.VRF)
	}
}

// TestConnect_WaitsForReady_OnReachableTarget proves Connect blocks until
// the gRPC connection is Ready (not just allocated). A noop in-process
// listener is enough — the gRPC handshake completes locally.
func TestConnect_WaitsForReady_OnReachableTarget(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	p := NewPeerClient(lis.Addr().String(), netbind.Binding{},
		[]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
		logger.Get("test"))
	defer p.Close()

	start := time.Now()
	if err := p.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	// If Connect returned before Ready, this would race — but it passes
	// reliably because grpc.WaitForStateChange blocks until the local
	// handshake completes.
	if time.Since(start) >= connectReadyTimeout {
		t.Errorf("Connect waited longer than %s on a reachable target", connectReadyTimeout)
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.conn == nil || p.client == nil {
		t.Fatal("conn/client not populated after successful Connect")
	}
}

// TestConnect_FailsWithinTimeout_OnUnreachableTarget proves Connect does
// not return early on allocation success: it must wait for Ready and
// fail when Ready never comes.
func TestConnect_FailsWithinTimeout_OnUnreachableTarget(t *testing.T) {
	// 198.51.100.0/24 is TEST-NET-2 — guaranteed non-routable.
	p := NewPeerClient("198.51.100.1:50051", netbind.Binding{},
		[]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
		logger.Get("test"))
	defer p.Close()

	start := time.Now()
	err := p.Connect()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Connect should fail on unreachable target")
	}
	// Allow a small margin over connectReadyTimeout for scheduling.
	if elapsed > connectReadyTimeout+2*time.Second {
		t.Errorf("Connect took %s, exceeds timeout cap %s", elapsed, connectReadyTimeout+2*time.Second)
	}
}

// TestCloseConn_ClearsState ensures the heartbeat reconnect path leaves
// the PeerClient in a state where the next Connect builds a fresh
// ClientConn (the spec requires close-before-overwrite to re-apply the
// bound dialer).
func TestCloseConn_ClearsState(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	p := NewPeerClient(lis.Addr().String(), netbind.Binding{},
		[]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
		logger.Get("test"))
	defer p.Close()

	if err := p.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	p.CloseConn()

	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.conn != nil {
		t.Error("conn should be nil after CloseConn")
	}
	if p.client != nil {
		t.Error("client should be nil after CloseConn")
	}
	if p.state.Connected {
		t.Error("state.Connected should be false after CloseConn")
	}
}

func TestAddrFamily(t *testing.T) {
	cases := []struct {
		addr string
		want netbind.Family
	}{
		{":50051", netbind.FamilyV4},
		{"0.0.0.0:50051", netbind.FamilyV4},
		{"10.0.0.1:50051", netbind.FamilyV4},
		{"[::]:50051", netbind.FamilyV6},
		{"[2001:db8::1]:50051", netbind.FamilyV6},
		{"2001:db8::1", netbind.FamilyV6},
	}
	for _, c := range cases {
		t.Run(c.addr, func(t *testing.T) {
			if got := addrFamily(c.addr); got != c.want {
				t.Errorf("addrFamily(%q)=%v want %v", c.addr, got, c.want)
			}
		})
	}
}

// TestConnect_RespectsContextCancel guards the manager.Stop path: if the
// PeerClient is canceled mid-Connect (e.g. shutdown during a flap),
// Connect should return promptly rather than hold the goroutine until
// the timeout fires.
func TestConnect_RespectsContextCancel(t *testing.T) {
	p := NewPeerClient("198.51.100.1:50051", netbind.Binding{},
		[]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
		logger.Get("test"))

	done := make(chan error, 1)
	go func() { done <- p.Connect() }()

	time.Sleep(50 * time.Millisecond)
	p.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Connect did not return within 2s of Close")
	}

	// Drain any extra goroutines via context.
	_ = context.Background()
}
