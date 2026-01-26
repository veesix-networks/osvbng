package pppoe

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"

	"inet.af/netaddr"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
)

type PoolAllocator struct {
	pools      map[string]*ipPool
	leases     map[string]*poolLease
	leasesByIP map[string]*poolLease
	mu         sync.Mutex
}

type ipPool struct {
	name       string
	network    *net.IPNet
	rangeStart net.IP
	rangeEnd   net.IP
	gateway    net.IP
	dns        []net.IP
	priority   int
}

type poolLease struct {
	IP        net.IP
	SessionID string
	PoolName  string
}

func NewPoolAllocator() *PoolAllocator {
	return &PoolAllocator{
		pools:      make(map[string]*ipPool),
		leases:     make(map[string]*poolLease),
		leasesByIP: make(map[string]*poolLease),
	}
}

func (pa *PoolAllocator) Allocate(sessionID string, group *subscriber.SubscriberGroup) (ip, dns1, dns2 net.IP, err error) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	if lease, exists := pa.leases[sessionID]; exists {
		pool := pa.pools[lease.PoolName]
		dns1, dns2 = pa.getDNS(pool)
		return lease.IP, dns1, dns2, nil
	}

	for _, poolCfg := range group.AddressPools {
		pa.ensurePool(poolCfg)
	}

	pool := pa.selectPool(group)
	if pool == nil {
		return nil, nil, nil, fmt.Errorf("no available pool")
	}

	ip, err = pa.allocateFromPool(pool, sessionID)
	if err != nil {
		return nil, nil, nil, err
	}

	dns1, dns2 = pa.getDNS(pool)
	return ip, dns1, dns2, nil
}

func (pa *PoolAllocator) ensurePool(cfg *subscriber.AddressPool) {
	if _, exists := pa.pools[cfg.Name]; exists {
		return
	}

	prefix, err := netaddr.ParseIPPrefix(cfg.Network)
	if err != nil {
		return
	}

	_, ipnet, _ := net.ParseCIDR(cfg.Network)

	rangeStart := prefix.Range().From().Next()
	rangeEnd := prefix.Range().To().Prior()

	pool := &ipPool{
		name:       cfg.Name,
		network:    ipnet,
		rangeStart: net.ParseIP(rangeStart.String()),
		rangeEnd:   net.ParseIP(rangeEnd.String()),
		gateway:    net.ParseIP(cfg.Gateway),
		dns:        make([]net.IP, 0, len(cfg.DNS)),
		priority:   cfg.Priority,
	}

	for _, dns := range cfg.DNS {
		if ip := net.ParseIP(dns); ip != nil {
			pool.dns = append(pool.dns, ip)
		}
	}

	pa.pools[cfg.Name] = pool
}

func (pa *PoolAllocator) selectPool(group *subscriber.SubscriberGroup) *ipPool {
	var best *ipPool
	bestPriority := 999999

	for _, poolCfg := range group.AddressPools {
		pool, exists := pa.pools[poolCfg.Name]
		if !exists {
			continue
		}
		if pool.priority < bestPriority && pa.hasAvailable(pool) {
			best = pool
			bestPriority = pool.priority
		}
	}

	return best
}

func (pa *PoolAllocator) hasAvailable(pool *ipPool) bool {
	start := binary.BigEndian.Uint32(pool.rangeStart.To4())
	end := binary.BigEndian.Uint32(pool.rangeEnd.To4())

	var gateway uint32
	if pool.gateway != nil && pool.gateway.To4() != nil {
		gateway = binary.BigEndian.Uint32(pool.gateway.To4())
	}

	for i := start; i <= end; i++ {
		if i == gateway {
			continue
		}
		ip := make(net.IP, 4)
		binary.BigEndian.PutUint32(ip, i)
		if _, used := pa.leasesByIP[ip.String()]; !used {
			return true
		}
	}
	return false
}

func (pa *PoolAllocator) allocateFromPool(pool *ipPool, sessionID string) (net.IP, error) {
	start := binary.BigEndian.Uint32(pool.rangeStart.To4())
	end := binary.BigEndian.Uint32(pool.rangeEnd.To4())

	var gateway uint32
	if pool.gateway != nil && pool.gateway.To4() != nil {
		gateway = binary.BigEndian.Uint32(pool.gateway.To4())
	}

	for i := start; i <= end; i++ {
		if i == gateway {
			continue
		}

		ip := make(net.IP, 4)
		binary.BigEndian.PutUint32(ip, i)

		if _, used := pa.leasesByIP[ip.String()]; !used {
			lease := &poolLease{
				IP:        ip,
				SessionID: sessionID,
				PoolName:  pool.name,
			}
			pa.leases[sessionID] = lease
			pa.leasesByIP[ip.String()] = lease
			return ip, nil
		}
	}

	return nil, fmt.Errorf("pool %s exhausted", pool.name)
}

func (pa *PoolAllocator) getDNS(pool *ipPool) (net.IP, net.IP) {
	var dns1, dns2 net.IP
	if len(pool.dns) > 0 {
		dns1 = pool.dns[0]
	}
	if len(pool.dns) > 1 {
		dns2 = pool.dns[1]
	}
	return dns1, dns2
}

func (pa *PoolAllocator) Release(sessionID string) {
	pa.mu.Lock()
	defer pa.mu.Unlock()

	if lease, exists := pa.leases[sessionID]; exists {
		delete(pa.leasesByIP, lease.IP.String())
		delete(pa.leases, sessionID)
	}
}
