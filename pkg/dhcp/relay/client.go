// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package relay

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/veesix-networks/osvbng/pkg/config/ip"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/netbind"
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

type v4PendingKey struct {
	xid uint32
	mac [6]byte
}

type v6PendingKey struct {
	txnID    [3]byte
	peerAddr [16]byte
}

type serverCacheKey struct {
	servers string
	binding bindingKey
}

type Client struct {
	openCtx context.Context
	opener  socketOpener

	groupsMu sync.RWMutex
	groups   map[bindingKey]*socketGroup

	replyPool sync.Pool
	closed    chan struct{}
	closeOnce sync.Once
	logger    *logger.Logger

	serversMu     sync.RWMutex
	servers       map[serverEntryKey]*Server
	resolvedCache sync.Map

	requests4 atomic.Uint64
	replies4  atomic.Uint64
	timeouts4 atomic.Uint64
	requests6 atomic.Uint64
	replies6  atomic.Uint64
	timeouts6 atomic.Uint64
}

type serverEntryKey struct {
	address string
	binding bindingKey
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
		openCtx: context.Background(),
		opener:  defaultSocketOpener,
		groups:  make(map[bindingKey]*socketGroup),
		servers: make(map[serverEntryKey]*Server),
		closed:  make(chan struct{}),
		logger:  logger.Get(logger.IPoERelay),
		replyPool: sync.Pool{
			New: func() interface{} {
				ch := make(chan []byte, 1)
				return ch
			},
		},
	}
	return c
}

func (c *Client) isClosed() bool {
	select {
	case <-c.closed:
		return true
	default:
		return false
	}
}

func (c *Client) Forward4(pkt []byte, xid uint32, servers []*Server, timeout time.Duration, deadTime time.Duration, deadThreshold int) ([]byte, error) {
	key := v4PendingKey{xid: xid}
	if len(pkt) >= 34 {
		copy(key.mac[:], pkt[28:34])
	}

	replyCh := c.replyPool.Get().(chan []byte)
	defer func() {
		select {
		case <-replyCh:
		default:
		}
		c.replyPool.Put(replyCh)
	}()

	for _, srv := range servers {
		if srv.IsDead(deadTime) {
			continue
		}

		group, err := c.groupFor(srv.bindingKey(), srv.Binding)
		if err != nil {
			c.logger.Debug("groupFor failed", "server", srv.Addr, "error", err)
			continue
		}

		group.registerV4(key, replyCh)

		srv.requests.Add(1)
		c.requests4.Add(1)

		if err := group.write(pkt, srv.Addr); err != nil {
			group.cancelV4(key)
			srv.RecordFailure(deadThreshold)
			continue
		}

		timer := time.NewTimer(timeout)
		select {
		case reply := <-replyCh:
			timer.Stop()
			srv.RecordSuccess()
			c.replies4.Add(1)
			return reply, nil
		case <-timer.C:
			group.cancelV4(key)
			srv.RecordFailure(deadThreshold)
			c.timeouts4.Add(1)
		case <-c.closed:
			timer.Stop()
			group.cancelV4(key)
			return nil, ErrClientClose
		}
	}

	return nil, ErrAllDead
}

func (c *Client) Forward6(pkt []byte, txnID [3]byte, servers []*Server, timeout time.Duration, deadTime time.Duration, deadThreshold int) ([]byte, error) {
	key := v6PendingKey{txnID: txnID}
	if len(pkt) >= DHCPv6RelayHeaderLen {
		copy(key.peerAddr[:], pkt[18:34])
	}

	replyCh := c.replyPool.Get().(chan []byte)
	defer func() {
		select {
		case <-replyCh:
		default:
		}
		c.replyPool.Put(replyCh)
	}()

	for _, srv := range servers {
		if srv.IsDead(deadTime) {
			continue
		}

		group, err := c.groupFor(srv.bindingKey(), srv.Binding)
		if err != nil {
			c.logger.Debug("groupFor failed", "server", srv.Addr, "error", err)
			continue
		}

		group.registerV6(key, replyCh)

		srv.requests.Add(1)
		c.requests6.Add(1)

		if err := group.write(pkt, srv.Addr); err != nil {
			group.cancelV6(key)
			srv.RecordFailure(deadThreshold)
			continue
		}

		timer := time.NewTimer(timeout)
		select {
		case reply := <-replyCh:
			timer.Stop()
			srv.RecordSuccess()
			c.replies6.Add(1)
			return reply, nil
		case <-timer.C:
			group.cancelV6(key)
			srv.RecordFailure(deadThreshold)
			c.timeouts6.Add(1)
		case <-c.closed:
			timer.Stop()
			group.cancelV6(key)
			return nil, ErrClientClose
		}
	}

	return nil, ErrAllDead
}

func (c *Client) SendOnly4(pkt []byte, servers []*Server, deadTime time.Duration, deadThreshold int) error {
	for _, srv := range servers {
		if srv.IsDead(deadTime) {
			continue
		}
		group, err := c.groupFor(srv.bindingKey(), srv.Binding)
		if err != nil {
			continue
		}
		srv.requests.Add(1)
		c.requests4.Add(1)
		if err := group.write(pkt, srv.Addr); err != nil {
			srv.RecordFailure(deadThreshold)
			continue
		}
		return nil
	}
	return ErrAllDead
}

func (c *Client) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })

	c.groupsMu.Lock()
	defer c.groupsMu.Unlock()
	var errs []error
	for k, g := range c.groups {
		g.close()
		delete(c.groups, k)
	}
	return errors.Join(errs...)
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

func (c *Client) SocketGroupCount() int {
	c.groupsMu.RLock()
	defer c.groupsMu.RUnlock()
	return len(c.groups)
}

func (c *Client) getOrCreateServer(family Family, addr *net.UDPAddr, priority int, b netbind.Binding) *Server {
	bk := makeBindingKey(family, b)
	key := serverEntryKey{address: addr.String(), binding: bk}

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
	s := &Server{
		Addr:     addr,
		Priority: priority,
		Family:   family,
		Binding:  b,
	}
	c.servers[key] = s
	return s
}

func ResolveServers(family Family, cfgServers []ip.DHCPRelayServer, profileBinding netbind.EndpointBinding) ([]*Server, error) {
	if len(cfgServers) == 0 {
		return nil, ErrNoServers
	}

	netbindFamily := netbind.FamilyV4
	if family == FamilyV6 {
		netbindFamily = netbind.FamilyV6
	}

	client := GetClient()

	cacheKey := buildServerCacheKey(family, cfgServers, profileBinding)
	if cached, ok := client.resolvedCache.Load(cacheKey); ok {
		return cached.([]*Server), nil
	}

	servers := make([]*Server, 0, len(cfgServers))
	for _, cs := range cfgServers {
		effective := cs.EndpointBinding.MergeWith(profileBinding)
		bind, err := effective.Resolve(netbindFamily)
		if err != nil {
			return nil, fmt.Errorf("resolve binding for %s: %w", cs.Address, err)
		}

		addr, err := resolveServerUDPAddr(family, cs.Address, bind)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", cs.Address, err)
		}
		servers = append(servers, client.getOrCreateServer(family, addr, cs.Priority, bind))
	}

	sort.Slice(servers, func(i, j int) bool {
		return servers[i].Priority > servers[j].Priority
	})

	client.resolvedCache.Store(cacheKey, servers)
	return servers, nil
}

func resolveServerUDPAddr(family Family, addr string, b netbind.Binding) (*net.UDPAddr, error) {
	defaultPort := family.localPort()

	host, port, err := splitHostPortDefault(addr, defaultPort)
	if err != nil {
		return nil, err
	}

	if ip := net.ParseIP(host); ip != nil {
		return &net.UDPAddr{IP: ip, Port: port}, nil
	}

	resolver := netbind.Resolver(b)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ips, err := resolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if family == FamilyV6 && ip.To4() != nil {
			continue
		}
		if family == FamilyV4 && ip.To4() == nil {
			continue
		}
		return &net.UDPAddr{IP: ip, Port: port}, nil
	}
	return nil, fmt.Errorf("no %s address found for %s", family.network(), host)
}

func splitHostPortDefault(addr string, defaultPort int) (string, int, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, defaultPort, nil
	}
	if portStr == "" {
		return host, defaultPort, nil
	}
	p, err := net.LookupPort("udp", portStr)
	if err != nil {
		return "", 0, err
	}
	return host, p, nil
}

func buildServerCacheKey(family Family, cfgServers []ip.DHCPRelayServer, profile netbind.EndpointBinding) serverCacheKey {
	bk := bindingKey{
		Family:    family,
		VRF:       profile.VRF,
		LocalPort: family.localPort(),
	}
	switch family {
	case FamilyV4:
		bk.SourceIP = profile.SourceIP
	case FamilyV6:
		bk.SourceIP = profile.SourceIPv6
	}

	if len(cfgServers) == 1 {
		s := cfgServers[0]
		return serverCacheKey{
			servers: serverFingerprint(s),
			binding: bk,
		}
	}
	size := 0
	for i := range cfgServers {
		size += len(serverFingerprint(cfgServers[i])) + 1
	}
	b := make([]byte, 0, size)
	for i := range cfgServers {
		b = append(b, serverFingerprint(cfgServers[i])...)
		b = append(b, ',')
	}
	return serverCacheKey{servers: string(b), binding: bk}
}

func serverFingerprint(s ip.DHCPRelayServer) string {
	return s.Address + "|" + s.VRF + "|" + s.SourceIP + "|" + s.SourceIPv6
}
