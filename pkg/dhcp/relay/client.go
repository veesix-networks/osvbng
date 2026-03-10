// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package relay

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/veesix-networks/osvbng/pkg/config/ip"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

var (
	ErrNoServers   = errors.New("no servers configured")
	ErrAllDead     = errors.New("all servers dead")
	ErrTimeout     = errors.New("server timeout")
	ErrClientClose = errors.New("client closed")
)

type ClientStats struct {
	Requests4 uint64 `json:"requests4" prometheus:"name=osvbng_dhcp_relay_requests_v4,help=DHCPv4 relay requests forwarded,type=counter"`
	Replies4  uint64 `json:"replies4" prometheus:"name=osvbng_dhcp_relay_replies_v4,help=DHCPv4 relay replies received,type=counter"`
	Timeouts4 uint64 `json:"timeouts4" prometheus:"name=osvbng_dhcp_relay_timeouts_v4,help=DHCPv4 relay server timeouts,type=counter"`
	Requests6 uint64 `json:"requests6" prometheus:"name=osvbng_dhcp_relay_requests_v6,help=DHCPv6 relay requests forwarded,type=counter"`
	Replies6  uint64 `json:"replies6" prometheus:"name=osvbng_dhcp_relay_replies_v6,help=DHCPv6 relay replies received,type=counter"`
	Timeouts6 uint64 `json:"timeouts6" prometheus:"name=osvbng_dhcp_relay_timeouts_v6,help=DHCPv6 relay server timeouts,type=counter"`
}

type Client struct {
	conn4     *net.UDPConn
	conn6     *net.UDPConn
	pending4  map[uint32]chan<- []byte
	pending6  map[[3]byte]chan<- []byte
	mu4       sync.Mutex
	mu6       sync.Mutex
	replyPool sync.Pool
	closed    chan struct{}
	logger    *slog.Logger

	serversMu sync.RWMutex
	servers   map[string]*Server

	requests4 atomic.Uint64
	replies4  atomic.Uint64
	timeouts4 atomic.Uint64
	requests6 atomic.Uint64
	replies6  atomic.Uint64
	timeouts6 atomic.Uint64
}

var (
	clientOnce sync.Once
	clientInst *Client
)

func GetClient() *Client {
	clientOnce.Do(func() {
		clientInst = newClient()
	})
	return clientInst
}

func newClient() *Client {
	c := &Client{
		pending4: make(map[uint32]chan<- []byte),
		pending6: make(map[[3]byte]chan<- []byte),
		servers:  make(map[string]*Server),
		closed:   make(chan struct{}),
		logger:   logger.Get(logger.IPoERelay),
		replyPool: sync.Pool{
			New: func() interface{} {
				ch := make(chan []byte, 1)
				return ch
			},
		},
	}
	return c
}

func (c *Client) ensureConn4() error {
	if c.conn4 != nil {
		return nil
	}
	// Bind to port 67: DHCP servers reply to GIAddr:67 per RFC 2131 §4.1
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: 67})
	if err != nil {
		return fmt.Errorf("listen udp4:67: %w", err)
	}
	c.conn4 = conn
	go c.readLoop4()
	return nil
}

func (c *Client) ensureConn6() error {
	if c.conn6 != nil {
		return nil
	}
	conn, err := net.ListenUDP("udp6", &net.UDPAddr{Port: 547})
	if err != nil {
		return fmt.Errorf("listen udp: %w", err)
	}
	c.conn6 = conn
	go c.readLoop6()
	return nil
}

func (c *Client) Forward4(pkt []byte, xid uint32, servers []*Server, timeout time.Duration, deadTime time.Duration, deadThreshold int) ([]byte, error) {
	if err := c.ensureConn4(); err != nil {
		return nil, err
	}

	srv := c.pickServer(servers, deadTime)
	if srv == nil {
		return nil, ErrAllDead
	}

	replyCh := c.replyPool.Get().(chan []byte)
	defer func() {
		// drain before returning to pool
		select {
		case <-replyCh:
		default:
		}
		c.replyPool.Put(replyCh)
	}()

	c.mu4.Lock()
	c.pending4[xid] = replyCh
	c.mu4.Unlock()

	defer func() {
		c.mu4.Lock()
		delete(c.pending4, xid)
		c.mu4.Unlock()
	}()

	srv.requests.Add(1)
	c.requests4.Add(1)

	if _, err := c.conn4.WriteToUDP(pkt, srv.Addr); err != nil {
		srv.RecordFailure(deadThreshold)
		c.timeouts4.Add(1)
		return nil, fmt.Errorf("write to %s: %w", srv.Addr, err)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case reply := <-replyCh:
		srv.RecordSuccess()
		c.replies4.Add(1)
		return reply, nil
	case <-timer.C:
		srv.RecordFailure(deadThreshold)
		c.timeouts4.Add(1)
		return nil, ErrTimeout
	case <-c.closed:
		return nil, ErrClientClose
	}
}

func (c *Client) Forward6(pkt []byte, txnID [3]byte, servers []*Server, timeout time.Duration, deadTime time.Duration, deadThreshold int) ([]byte, error) {
	if err := c.ensureConn6(); err != nil {
		return nil, err
	}

	srv := c.pickServer(servers, deadTime)
	if srv == nil {
		return nil, ErrAllDead
	}

	replyCh := c.replyPool.Get().(chan []byte)
	defer func() {
		select {
		case <-replyCh:
		default:
		}
		c.replyPool.Put(replyCh)
	}()

	c.mu6.Lock()
	c.pending6[txnID] = replyCh
	c.mu6.Unlock()

	defer func() {
		c.mu6.Lock()
		delete(c.pending6, txnID)
		c.mu6.Unlock()
	}()

	srv.requests.Add(1)
	c.requests6.Add(1)

	if _, err := c.conn6.WriteToUDP(pkt, srv.Addr); err != nil {
		srv.RecordFailure(deadThreshold)
		c.timeouts6.Add(1)
		return nil, fmt.Errorf("write to %s: %w", srv.Addr, err)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case reply := <-replyCh:
		srv.RecordSuccess()
		c.replies6.Add(1)
		return reply, nil
	case <-timer.C:
		srv.RecordFailure(deadThreshold)
		c.timeouts6.Add(1)
		return nil, ErrTimeout
	case <-c.closed:
		return nil, ErrClientClose
	}
}

func (c *Client) Close() error {
	close(c.closed)
	var errs []error
	if c.conn4 != nil {
		errs = append(errs, c.conn4.Close())
	}
	if c.conn6 != nil {
		errs = append(errs, c.conn6.Close())
	}
	return errors.Join(errs...)
}

func (c *Client) readLoop4() {
	var buf [2048]byte
	for {
		n, _, err := c.conn4.ReadFromUDP(buf[:])
		if err != nil {
			select {
			case <-c.closed:
				return
			default:
			}
			continue
		}
		if n < 8 {
			continue
		}

		xid := uint32(buf[4])<<24 | uint32(buf[5])<<16 | uint32(buf[6])<<8 | uint32(buf[7])
		reply := make([]byte, n)
		copy(reply, buf[:n])

		c.mu4.Lock()
		ch, ok := c.pending4[xid]
		if ok {
			delete(c.pending4, xid)
		}
		c.mu4.Unlock()

		if ok {
			select {
			case ch <- reply:
			default:
			}
		}
	}
}

func (c *Client) readLoop6() {
	var buf [2048]byte
	for {
		n, _, err := c.conn6.ReadFromUDP(buf[:])
		if err != nil {
			select {
			case <-c.closed:
				return
			default:
			}
			continue
		}
		if n < 4 {
			continue
		}

		// DHCPv6 Relay-Reply: msg-type(1) + hop-count(1) + link-addr(16) + peer-addr(16) = 34 bytes header
		// The transaction ID is inside the inner Relay-Message option.
		// For direct server replies (non-relay): msg-type(1) + txn-id(3)
		var txnID [3]byte

		msgType := buf[0]
		if msgType == 13 {
			// Relay-Reply: extract txn-id from inner message
			inner := extractRelayMessage(buf[:n])
			if inner == nil || len(inner) < 4 {
				continue
			}
			copy(txnID[:], inner[1:4])
		} else {
			copy(txnID[:], buf[1:4])
		}

		reply := make([]byte, n)
		copy(reply, buf[:n])

		c.mu6.Lock()
		ch, ok := c.pending6[txnID]
		if ok {
			delete(c.pending6, txnID)
		}
		c.mu6.Unlock()

		if ok {
			select {
			case ch <- reply:
			default:
			}
		}
	}
}

func (c *Client) pickServer(servers []*Server, deadTime time.Duration) *Server {
	if len(servers) == 0 {
		return nil
	}
	for _, s := range servers {
		if !s.IsDead(deadTime) {
			return s
		}
	}
	return nil
}

func (c *Client) GetStats() ClientStats {
	return ClientStats{
		Requests4: c.requests4.Load(),
		Replies4:  c.replies4.Load(),
		Timeouts4: c.timeouts4.Load(),
		Requests6: c.requests6.Load(),
		Replies6:  c.replies6.Load(),
		Timeouts6: c.timeouts6.Load(),
	}
}

func (c *Client) GetServers(deadTime time.Duration) []ServerStatus {
	c.serversMu.RLock()
	defer c.serversMu.RUnlock()

	statuses := make([]ServerStatus, 0, len(c.servers))
	for _, s := range c.servers {
		statuses = append(statuses, s.GetStatus(deadTime))
	}
	return statuses
}

func (c *Client) getOrCreateServer(addr *net.UDPAddr, priority int) *Server {
	key := addr.String()

	c.serversMu.RLock()
	if s, ok := c.servers[key]; ok {
		c.serversMu.RUnlock()
		return s
	}
	c.serversMu.RUnlock()

	c.serversMu.Lock()
	defer c.serversMu.Unlock()
	if s, ok := c.servers[key]; ok {
		return s
	}
	s := &Server{Addr: addr, Priority: priority}
	c.servers[key] = s
	return s
}

func ResolveServers(cfgServers []ip.DHCPRelayServer) ([]*Server, error) {
	if len(cfgServers) == 0 {
		return nil, ErrNoServers
	}

	client := GetClient()
	servers := make([]*Server, 0, len(cfgServers))
	for _, cs := range cfgServers {
		addr, err := net.ResolveUDPAddr("udp", cs.Address)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", cs.Address, err)
		}
		servers = append(servers, client.getOrCreateServer(addr, cs.Priority))
	}

	sort.Slice(servers, func(i, j int) bool {
		return servers[i].Priority > servers[j].Priority
	})

	return servers, nil
}
