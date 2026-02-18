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
	allocators       map[string]*PoolAllocator
	profilePools     map[string][]string
	ianaAllocators   map[string]*PoolAllocator
	profileIANAPools map[string][]string
	pdAllocators     map[string]*PrefixAllocator
	profilePDPools   map[string][]string
	mu               sync.RWMutex
}

var (
	globalRegistry *Registry
	registryMu     sync.Mutex
)

func InitGlobalRegistry(v4Profiles map[string]*ip.IPv4Profile, v6Profiles map[string]*ip.IPv6Profile) *Registry {
	registryMu.Lock()
	defer registryMu.Unlock()
	globalRegistry = newRegistry(v4Profiles, v6Profiles)
	return globalRegistry
}

func GetGlobalRegistry() *Registry {
	registryMu.Lock()
	defer registryMu.Unlock()
	return globalRegistry
}

func ResetGlobalRegistry() {
	registryMu.Lock()
	defer registryMu.Unlock()
	globalRegistry = nil
}

func newRegistry(v4Profiles map[string]*ip.IPv4Profile, v6Profiles map[string]*ip.IPv6Profile) *Registry {
	r := &Registry{
		allocators:       make(map[string]*PoolAllocator),
		profilePools:     make(map[string][]string),
		ianaAllocators:   make(map[string]*PoolAllocator),
		profileIANAPools: make(map[string][]string),
		pdAllocators:     make(map[string]*PrefixAllocator),
		profilePDPools:   make(map[string][]string),
	}

	r.initV4Pools(v4Profiles)
	r.initV6Pools(v6Profiles)

	return r
}

func (r *Registry) initV4Pools(profiles map[string]*ip.IPv4Profile) {
	if profiles == nil {
		return
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
			poolNames[i] = profileName + "/" + ref.name
		}
		r.profilePools[profileName] = poolNames

		for _, pool := range profile.Pools {
			key := profileName + "/" + pool.Name
			if _, exists := r.allocators[key]; exists {
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

			r.allocators[key] = NewPoolAllocator(rs, re, excludeAddrs)
		}
	}
}

func (r *Registry) initV6Pools(profiles map[string]*ip.IPv6Profile) {
	if profiles == nil {
		return
	}

	for profileName, profile := range profiles {
		ianaNames := make([]string, len(profile.IANAPools))
		for i, pool := range profile.IANAPools {
			key := profileName + "/" + pool.Name
			ianaNames[i] = key

			if _, exists := r.ianaAllocators[key]; exists {
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
			if gw, err := netip.ParseAddr(pool.Gateway); err == nil {
				excludeAddrs = append(excludeAddrs, gw)
			}

			r.ianaAllocators[key] = NewPoolAllocator(rs, re, excludeAddrs)
		}
		r.profileIANAPools[profileName] = ianaNames

		pdNames := make([]string, len(profile.PDPools))
		for i, pool := range profile.PDPools {
			key := profileName + "/" + pool.Name
			pdNames[i] = key

			if _, exists := r.pdAllocators[key]; exists {
				continue
			}

			prefix, err := netip.ParsePrefix(pool.Network)
			if err != nil {
				continue
			}

			alloc := NewPrefixAllocator(prefix, int(pool.PrefixLength))
			if alloc == nil {
				continue
			}

			r.pdAllocators[key] = alloc
		}
		r.profilePDPools[profileName] = pdNames
	}
}

func (r *Registry) AllocateFromProfile(profileName, poolOverride, sessionID string) (net.IP, string, error) {
	if r == nil {
		return nil, "", ErrPoolExhausted
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if poolOverride != "" {
		key := profileName + "/" + poolOverride
		if alloc, ok := r.allocators[key]; ok {
			allocated, err := alloc.Allocate(sessionID)
			if err == nil {
				return allocated, key, nil
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

func (r *Registry) AllocateIANAFromProfile(profileName, poolOverride, sessionID string) (net.IP, string, error) {
	if r == nil {
		return nil, "", ErrPoolExhausted
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if poolOverride != "" {
		key := profileName + "/" + poolOverride
		if alloc, ok := r.ianaAllocators[key]; ok {
			allocated, err := alloc.Allocate(sessionID)
			if err == nil {
				return allocated, key, nil
			}
		}
	}

	for _, poolName := range r.profileIANAPools[profileName] {
		alloc, ok := r.ianaAllocators[poolName]
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

func (r *Registry) AllocatePDFromProfile(profileName, poolOverride, sessionID string) (*net.IPNet, string, error) {
	if r == nil {
		return nil, "", ErrPoolExhausted
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if poolOverride != "" {
		key := profileName + "/" + poolOverride
		if alloc, ok := r.pdAllocators[key]; ok {
			allocated, err := alloc.Allocate(sessionID)
			if err == nil {
				return allocated, key, nil
			}
		}
	}

	for _, poolName := range r.profilePDPools[profileName] {
		alloc, ok := r.pdAllocators[poolName]
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

func (r *Registry) ReleaseIANA(poolName string, ip net.IP) {
	if r == nil {
		return
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if alloc, ok := r.ianaAllocators[poolName]; ok {
		alloc.Release(ip)
	}
}

func (r *Registry) ReleasePD(poolName string, prefix *net.IPNet) {
	if r == nil {
		return
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if alloc, ok := r.pdAllocators[poolName]; ok {
		alloc.Release(prefix)
	}
}

func (r *Registry) ReserveIP(ip net.IP, sessionID string) {
	if r == nil {
		return
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, alloc := range r.allocators {
		if alloc.Contains(ip) {
			alloc.Reserve(ip, sessionID)
			return
		}
	}
}

func (r *Registry) ReserveIANA(ip net.IP, sessionID string) {
	if r == nil {
		return
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, alloc := range r.ianaAllocators {
		if alloc.Contains(ip) {
			alloc.Reserve(ip, sessionID)
			return
		}
	}
}

func (r *Registry) ReservePD(prefix *net.IPNet, sessionID string) {
	if r == nil {
		return
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, alloc := range r.pdAllocators {
		if alloc.Contains(prefix) {
			alloc.Reserve(prefix, sessionID)
			return
		}
	}
}

func (r *Registry) ReleaseIANAByIP(ip net.IP) {
	if r == nil {
		return
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, alloc := range r.ianaAllocators {
		alloc.Release(ip)
	}
}

func (r *Registry) ReleasePDByPrefix(prefix *net.IPNet) {
	if r == nil {
		return
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, alloc := range r.pdAllocators {
		if alloc.Contains(prefix) {
			alloc.Release(prefix)
			return
		}
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
