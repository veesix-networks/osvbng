// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package radius

import (
	"crypto/hmac"
	"crypto/md5"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"layeh.com/radius"
)

var respBufPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, radius.MaxPacketLength)
		return &buf
	},
}

type pendingRequest struct {
	ch     chan *radius.Packet
	secret []byte
}

type radiusConn struct {
	conn    *net.UDPConn
	addr    string
	secret  []byte
	timeout time.Duration

	mu      sync.Mutex
	pending [256]*pendingRequest
	nextID  atomic.Uint32
	closed  atomic.Bool

	failures  atomic.Int32
	dead      atomic.Bool
	deadSince atomic.Int64
}

func newRadiusConn(addr string, secret []byte, timeout time.Duration) (*radiusConn, error) {
	raddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", addr, err)
	}

	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}

	rc := &radiusConn{
		conn:    conn,
		addr:    addr,
		secret:  secret,
		timeout: timeout,
	}

	go rc.readLoop()
	return rc, nil
}

func (rc *radiusConn) close() error {
	rc.closed.Store(true)
	return rc.conn.Close()
}

func (rc *radiusConn) allocID() byte {
	return byte(rc.nextID.Add(1))
}

func (rc *radiusConn) exchange(packet *radius.Packet) (*radius.Packet, error) {
	id := rc.allocID()
	packet.Identifier = id
	packet.Secret = rc.secret

	raw, err := packet.Encode()
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}

	if offset := findAttr80(raw); offset >= 0 {
		h := hmac.New(md5.New, rc.secret)
		h.Write(raw)
		copy(raw[offset:offset+16], h.Sum(nil))
	}

	ch := make(chan *radius.Packet, 1)
	req := &pendingRequest{ch: ch, secret: rc.secret}

	rc.mu.Lock()
	rc.pending[id] = req
	rc.mu.Unlock()

	defer func() {
		rc.mu.Lock()
		rc.pending[id] = nil
		rc.mu.Unlock()
	}()

	if _, err := rc.conn.Write(raw); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}

	timer := time.NewTimer(rc.timeout)
	defer timer.Stop()

	select {
	case resp := <-ch:
		return resp, nil
	case <-timer.C:
		return nil, fmt.Errorf("timeout waiting for response from %s", rc.addr)
	}
}

func (rc *radiusConn) readLoop() {
	for {
		bufPtr := respBufPool.Get().(*[]byte)
		buf := *bufPtr

		n, err := rc.conn.Read(buf)
		if err != nil {
			respBufPool.Put(bufPtr)
			if rc.closed.Load() {
				return
			}
			continue
		}

		resp, err := radius.Parse(buf[:n], rc.secret)
		respBufPool.Put(bufPtr)
		if err != nil {
			continue
		}

		id := resp.Identifier
		rc.mu.Lock()
		req := rc.pending[id]
		if req != nil {
			rc.pending[id] = nil
		}
		rc.mu.Unlock()

		if req != nil {
			select {
			case req.ch <- resp:
			default:
			}
		}
	}
}

func findAttr80(raw []byte) int {
	if len(raw) < 20 {
		return -1
	}
	i := 20
	for i+2 <= len(raw) {
		attrType := raw[i]
		attrLen := int(raw[i+1])
		if attrLen < 2 || i+attrLen > len(raw) {
			break
		}
		if attrType == 80 && attrLen == 18 {
			return i + 2
		}
		i += attrLen
	}
	return -1
}

func (rc *radiusConn) isDead(deadTime time.Duration) bool {
	if !rc.dead.Load() {
		return false
	}
	since := time.Unix(rc.deadSince.Load(), 0)
	if time.Since(since) >= deadTime {
		rc.dead.Store(false)
		rc.failures.Store(0)
		return false
	}
	return true
}

func (rc *radiusConn) recordFailure(threshold int) {
	if int(rc.failures.Add(1)) >= threshold {
		rc.dead.Store(true)
		rc.deadSince.Store(time.Now().Unix())
	}
}

func (rc *radiusConn) recordSuccess() {
	rc.failures.Store(0)
	rc.dead.Store(false)
}
