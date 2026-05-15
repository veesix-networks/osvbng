// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime"
	"sync"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	l2tppkt "github.com/veesix-networks/osvbng/pkg/l2tp"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/vishvananda/netns"
)

// KernelUDPTransport is a kernel-UDP-socket implementation of both the
// control-message SendControlFn and a synthetic punt-channel feed. It
// lets the control plane run against a BNG Blaster (or any L2TP peer)
// without VPP in the loop, which is the simplest topology to smoke-
// test the Go control plane.
//
// Topology constraints: nothing else on the host can bind UDP/1701,
// which means VPP must not have a DPDK / memif port set up to
// intercept that port. Switch to a VPP-punt transport once the data
// path is wired.
type KernelUDPTransport struct {
	conn *net.UDPConn

	mu      sync.Mutex
	stopped bool
	stop    chan struct{}
	done    chan struct{}
}

// NewKernelUDPTransport opens a UDP/1701 socket bound inside `nsName`
// (typically the dataplane netns where VPP's LCP taps and L3 IPs live).
// Pass "" to bind in the calling thread's current netns — useful for
// unit tests or topologies where the L2TP backbone IP is in the main
// netns. The returned socket retains its netns; reads and writes from
// any goroutine route through the correct netns regardless of the
// current OS thread.
func NewKernelUDPTransport(listenIP net.IP, nsName string) (*KernelUDPTransport, error) {
	addr := &net.UDPAddr{IP: listenIP, Port: 1701}

	open := func() (*net.UDPConn, error) {
		return net.ListenUDP("udp4", addr)
	}

	if nsName == "" {
		conn, err := open()
		if err != nil {
			return nil, fmt.Errorf("listen udp/1701: %w", err)
		}
		return &KernelUDPTransport{
			conn: conn,
			stop: make(chan struct{}),
			done: make(chan struct{}),
		}, nil
	}

	// Switch this OS thread into the target netns, open the socket, then
	// restore. The socket itself stays attached to the netns it was
	// opened in — goroutines using it later don't need to be in the
	// dataplane netns.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	origNs, err := netns.Get()
	if err != nil {
		return nil, fmt.Errorf("get current netns: %w", err)
	}
	defer origNs.Close()

	targetNs, err := netns.GetFromName(nsName)
	if err != nil {
		return nil, fmt.Errorf("get netns %q: %w", nsName, err)
	}
	defer targetNs.Close()

	if err := netns.Set(targetNs); err != nil {
		return nil, fmt.Errorf("setns %q: %w", nsName, err)
	}

	conn, openErr := open()

	// Restore even if open() failed.
	if setErr := netns.Set(origNs); setErr != nil {
		if conn != nil {
			_ = conn.Close()
		}
		return nil, fmt.Errorf("restore netns: %w", setErr)
	}

	if openErr != nil {
		return nil, fmt.Errorf("listen udp/1701 in netns %q: %w", nsName, openErr)
	}

	return &KernelUDPTransport{
		conn: conn,
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}, nil
}

// Send is a SendControlFn. The header's Ns/Nr come from the per-tunnel
// control channel; this function prepends the L2TP header and writes
// the resulting datagram to the kernel.
func (t *KernelUDPTransport) Send(localIP, peerIP net.IP, localPort, peerPort uint16, header l2tppkt.Header, body []byte) error {
	buf := header.AppendTo(make([]byte, 0, header.HeaderLenBytes()+len(body)), len(body))
	buf = append(buf, body...)
	_, err := t.conn.WriteToUDP(buf, &net.UDPAddr{IP: peerIP, Port: int(peerPort)})
	return err
}

// Feed runs an RX goroutine that reads UDP datagrams from the kernel
// socket, packages them as `*dataplane.ParsedPacket`, and sends them
// to `ch`. Loops until the context is cancelled or Close is called.
func (t *KernelUDPTransport) Feed(ctx context.Context, localIP net.IP, ch chan<- *dataplane.ParsedPacket) {
	defer close(t.done)
	bufPool := sync.Pool{New: func() any { b := make([]byte, 65535); return &b }}

	for {
		select {
		case <-t.stop:
			return
		case <-ctx.Done():
			return
		default:
		}
		pbuf := bufPool.Get().(*[]byte)
		n, peer, err := t.conn.ReadFromUDP(*pbuf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		if n <= 0 {
			bufPool.Put(pbuf)
			continue
		}
		payload := (*pbuf)[:n]

		// Build a minimal ParsedPacket carrying just enough for the
		// L2TP dispatcher: the UDP layer (for src/dst port + payload)
		// and the IP layer (for the src/dst IP that keys our session
		// lookup).
		ip := &layers.IPv4{
			SrcIP: peer.IP.To4(),
			DstIP: localIP.To4(),
		}
		udp := &layers.UDP{
			SrcPort: layers.UDPPort(peer.Port),
			DstPort: 1701,
		}
		udp.BaseLayer.Payload = payload
		pkt := &dataplane.ParsedPacket{
			Protocol: models.ProtocolL2TP,
			IPv4:     ip,
			UDP:      udp,
		}

		select {
		case ch <- pkt:
		case <-ctx.Done():
			bufPool.Put(pbuf)
			return
		case <-t.stop:
			bufPool.Put(pbuf)
			return
		}
	}
}

// Close stops the transport and releases the kernel socket.
func (t *KernelUDPTransport) Close() error {
	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		return nil
	}
	t.stopped = true
	close(t.stop)
	t.mu.Unlock()
	err := t.conn.Close()
	<-t.done
	return err
}

// Ensure type-compatibility with the SendControlFn signature.
var _ SendControlFn = (*KernelUDPTransport)(nil).Send

// Suppress unused imports for builds where gopacket is unused.
var _ = gopacket.NewPacket
