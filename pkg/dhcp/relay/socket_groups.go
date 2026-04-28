// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package relay

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/netbind"
)

// Bounds the read loop's retry rate on unclassified errors so a
// stuck-erroring socket can't starve the shared control-plane core.
const readBackoffOnUnknownErr = 100 * time.Millisecond

type socketOpener func(ctx context.Context, network string, port int, b netbind.Binding) (*net.UDPConn, error)

func defaultSocketOpener(ctx context.Context, network string, port int, b netbind.Binding) (*net.UDPConn, error) {
	return netbind.ListenUDP(ctx, network, port, b)
}

type socketGroup struct {
	key bindingKey

	conn   *net.UDPConn
	closed atomic.Bool

	mu       sync.Mutex
	pending4 map[v4PendingKey]chan<- []byte
	pending6 map[v6PendingKey]chan<- []byte

	owner  *Client
	logger *logger.Logger
}

func (g *socketGroup) registerV4(key v4PendingKey, ch chan<- []byte) {
	g.mu.Lock()
	g.pending4[key] = ch
	g.mu.Unlock()
}

func (g *socketGroup) cancelV4(key v4PendingKey) {
	g.mu.Lock()
	delete(g.pending4, key)
	g.mu.Unlock()
}

func (g *socketGroup) registerV6(key v6PendingKey, ch chan<- []byte) {
	g.mu.Lock()
	g.pending6[key] = ch
	g.mu.Unlock()
}

func (g *socketGroup) cancelV6(key v6PendingKey) {
	g.mu.Lock()
	delete(g.pending6, key)
	g.mu.Unlock()
}

func (g *socketGroup) write(pkt []byte, dst *net.UDPAddr) error {
	if g.closed.Load() {
		return ErrSocketGroupClosed
	}
	_, err := g.conn.WriteToUDP(pkt, dst)
	if err != nil && isFatalSocketErr(err) {
		g.markClosed("write", err)
	}
	return err
}

func (g *socketGroup) close() {
	if g.closed.Swap(true) {
		return
	}
	if g.conn != nil {
		_ = g.conn.Close()
	}
}

func (g *socketGroup) markClosed(stage string, err error) {
	if g.closed.Swap(true) {
		return
	}
	g.logger.Warn("DHCP relay socket group entering reopen state",
		"stage", stage,
		"binding_key", g.key,
		"error", err)
	if g.conn != nil {
		_ = g.conn.Close()
	}
	g.owner.dropGroup(g.key)
}

func (g *socketGroup) readLoop() {
	var buf [2048]byte
	for {
		if g.closed.Load() || g.owner.isClosed() {
			return
		}
		n, _, err := g.conn.ReadFromUDP(buf[:])
		if err != nil {
			if g.owner.isClosed() {
				return
			}
			if isFatalSocketErr(err) {
				g.markClosed("read", err)
				return
			}
			g.logger.Debug("DHCP relay read error (transient)",
				"binding_key", g.key, "error", err)
			select {
			case <-time.After(readBackoffOnUnknownErr):
			case <-g.owner.closed:
				return
			}
			continue
		}
		if g.key.Family == FamilyV4 {
			g.dispatchV4(buf[:n])
		} else {
			g.dispatchV6(buf[:n])
		}
	}
}

func (g *socketGroup) dispatchV4(raw []byte) {
	if len(raw) < 34 {
		return
	}
	var key v4PendingKey
	key.xid = uint32(raw[4])<<24 | uint32(raw[5])<<16 | uint32(raw[6])<<8 | uint32(raw[7])
	copy(key.mac[:], raw[28:34])

	reply := make([]byte, len(raw))
	copy(reply, raw)

	g.mu.Lock()
	ch, ok := g.pending4[key]
	if ok {
		delete(g.pending4, key)
	}
	g.mu.Unlock()

	if ok {
		select {
		case ch <- reply:
		default:
		}
	}
}

func (g *socketGroup) dispatchV6(raw []byte) {
	if len(raw) < DHCPv6RelayHeaderLen {
		return
	}
	var key v6PendingKey

	msgType := raw[0]
	if msgType == DHCPv6MsgRelayReply {
		copy(key.peerAddr[:], raw[18:34])
		inner := extractRelayMessage(raw)
		if len(inner) < 4 {
			return
		}
		copy(key.txnID[:], inner[1:4])
	} else {
		copy(key.txnID[:], raw[1:4])
	}

	reply := make([]byte, len(raw))
	copy(reply, raw)

	g.mu.Lock()
	ch, ok := g.pending6[key]
	if ok {
		delete(g.pending6, key)
	}
	g.mu.Unlock()

	if ok {
		select {
		case ch <- reply:
		default:
		}
	}
}

// Mixed default + VRF on the same local port works because Linux UDP
// allows two sockets at (addr, port) when their sk_bound_dev_if differs.
func (c *Client) groupFor(key bindingKey, b netbind.Binding) (*socketGroup, error) {
	c.groupsMu.RLock()
	if g, ok := c.groups[key]; ok && !g.closed.Load() {
		c.groupsMu.RUnlock()
		return g, nil
	}
	c.groupsMu.RUnlock()

	c.groupsMu.Lock()
	defer c.groupsMu.Unlock()
	if g, ok := c.groups[key]; ok && !g.closed.Load() {
		return g, nil
	}

	if c.isClosed() {
		return nil, ErrClientClose
	}

	conn, err := c.opener(c.openCtx, key.Family.network(), key.LocalPort, b)
	if err != nil {
		return nil, fmt.Errorf("open dhcp relay socket %s: %w", key.Family.network(), err)
	}

	g := &socketGroup{
		key:      key,
		conn:     conn,
		owner:    c,
		logger:   c.logger,
		pending4: make(map[v4PendingKey]chan<- []byte),
		pending6: make(map[v6PendingKey]chan<- []byte),
	}
	c.groups[key] = g
	go g.readLoop()
	return g, nil
}

func (c *Client) dropGroup(key bindingKey) {
	c.groupsMu.Lock()
	if g, ok := c.groups[key]; ok && g.closed.Load() {
		delete(c.groups, key)
	}
	c.groupsMu.Unlock()
}

func isFatalSocketErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	var sysErr syscall.Errno
	if errors.As(err, &sysErr) {
		switch sysErr {
		case syscall.ENODEV, syscall.ENETDOWN, syscall.EADDRNOTAVAIL, syscall.ENETUNREACH:
			return true
		}
	}
	return false
}

var ErrSocketGroupClosed = errors.New("socket group closed")
