package allocator

import (
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"

	"inet.af/netaddr"
	"github.com/veesix-networks/osvbng/pkg/config/ip"
)

type Registry struct {
	allocators   map[string]*PoolAllocator
	profilePools map[string][]string
	mu           sync.RWMutex
}

var (
	globalRegistry *Registry
	registryMu     sync.Mutex
)

func InitGlobalRegistry(profiles map[string]*ip.DHCPProfile) *Registry {
	registryMu.Lock()
	defer registryMu.Unlock()
	globalRegistry = newRegistry(profiles)
	return globalRegistry
}

func GetGlobalRegistry() *Registry {
	registryMu.Lock()
	defer registryMu.Unlock()
	return globalRegistry
}

func newRegistry(profiles map[string]*ip.DHCPProfile) *Registry {
	r := &Registry{
		allocators:   make(map[string]*PoolAllocator),
		profilePools: make(map[string][]string),
	}

	if profiles == nil {
		return r
	}

	for profileName, profile := range profiles {
		type poolRef struct {
			name     string
			priority int
		}
		refs := make([]poolRef, len(profile.Pools))
		for i, p := range profile.Pools {
			refs[i] = poolRef{p.Name, p.Priority}
		}
		sort.Slice(refs, func(i, j int) bool {
			return refs[i].priority < refs[j].priority
		})

		poolNames := make([]string, len(refs))
		for i, ref := range refs {
			poolNames[i] = ref.name
		}
		r.profilePools[profileName] = poolNames

		for _, pool := range profile.Pools {
			if _, exists := r.allocators[pool.Name]; exists {
				continue
			}

			prefix, err := netaddr.ParseIPPrefix(pool.Network)
			if err != nil {
				continue
			}

			rangeStart := pool.RangeStart
			rangeEnd := pool.RangeEnd
			if rangeStart == "" {
				rangeStart = prefix.Range().From().Next().String()
			}
			if rangeEnd == "" {
				rangeEnd = prefix.Range().To().Prior().String()
			}

			rs, err := netip.ParseAddr(rangeStart)
			if err != nil {
				continue
			}
			re, err := netip.ParseAddr(rangeEnd)
			if err != nil {
				continue
			}

			var excludeAddrs []netip.Addr

			gateway := pool.Gateway
			if gateway == "" {
				gateway = profile.Gateway
			}
			if gw, err := netip.ParseAddr(gateway); err == nil {
				excludeAddrs = append(excludeAddrs, gw)
			}

			for _, excl := range pool.Exclude {
				excludeAddrs = append(excludeAddrs, parseExcludeRange(excl)...)
			}

			r.allocators[pool.Name] = NewPoolAllocator(rs, re, excludeAddrs)
		}
	}

	return r
}

func (r *Registry) AllocateFromProfile(profileName, poolOverride, sessionID string) (net.IP, string, error) {
	if r == nil {
		return nil, "", ErrPoolExhausted
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if poolOverride != "" {
		if alloc, ok := r.allocators[poolOverride]; ok {
			allocated, err := alloc.Allocate(sessionID)
			if err == nil {
				return allocated, poolOverride, nil
			}
		}
	}

	for _, poolName := range r.profilePools[profileName] {
		alloc, ok := r.allocators[poolName]
		if !ok {
			continue
		}
		allocated, err := alloc.Allocate(sessionID)
		if err == nil {
			return allocated, poolName, nil
		}
	}

	return nil, "", ErrPoolExhausted
}

func (r *Registry) Release(poolName string, ip net.IP) {
	if r == nil {
		return
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if alloc, ok := r.allocators[poolName]; ok {
		alloc.Release(ip)
	}
}

func (r *Registry) ReleaseIP(ip net.IP) {
	if r == nil {
		return
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, alloc := range r.allocators {
		alloc.Release(ip)
	}
}

func (r *Registry) GetProfilePools(profileName string) []string {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.profilePools[profileName]
}

func parseExcludeRange(s string) []netip.Addr {
	if idx := strings.Index(s, "-"); idx >= 0 {
		start, err := netip.ParseAddr(strings.TrimSpace(s[:idx]))
		if err != nil {
			return nil
		}
		end, err := netip.ParseAddr(strings.TrimSpace(s[idx+1:]))
		if err != nil {
			return nil
		}
		var addrs []netip.Addr
		for addr := start; addr.Compare(end) <= 0; addr = addr.Next() {
			addrs = append(addrs, addr)
		}
		return addrs
	}

	addr, err := netip.ParseAddr(strings.TrimSpace(s))
	if err != nil {
		return nil
	}
	return []netip.Addr{addr}
}
