// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"fmt"
	"net"
	"sync"

	"github.com/veesix-networks/osvbng/pkg/config/cgnat"
	"github.com/veesix-networks/osvbng/pkg/models"
)

type subscriberKey struct {
	InsideVRF uint32
	InsideIP  [4]byte
}

type blockAllocation struct {
	OutsideIP      net.IP
	PortBlockStart uint16
	PortBlockEnd   uint16
}

type subscriberAllocation struct {
	PoolName  string
	PoolID    uint32
	SwIfIndex uint32
	Blocks    []blockAllocation
}

type outsideAddressState struct {
	IP             net.IP
	TotalBlocks    uint32
	AllocatedBits  []uint64
	Excluded       bool
}

type PoolManager struct {
	mu    sync.RWMutex
	pools map[string]*poolState
}

type poolState struct {
	Name   string
	ID     uint32
	Config *cgnat.Pool

	OutsideAddresses []*outsideAddressState
	Subscribers      map[subscriberKey]*subscriberAllocation

	nextPoolID uint32
}

func NewPoolManager() *PoolManager {
	return &PoolManager{
		pools: make(map[string]*poolState),
	}
}

func (pm *PoolManager) ConfigurePool(name string, id uint32, cfg *cgnat.Pool) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	blockSize := cfg.GetBlockSize()
	portStart := cfg.GetPortRangeStart()
	portEnd := cfg.GetPortRangeEnd()
	usablePorts := uint32(portEnd) - uint32(portStart) + 1
	blocksPerAddr := usablePorts / uint32(blockSize)

	var addresses []*outsideAddressState
	for _, addrStr := range cfg.OutsideAddresses {
		ips, err := expandCIDR(addrStr)
		if err != nil {
			return fmt.Errorf("pool %s: invalid outside address %s: %w", name, addrStr, err)
		}
		for _, ip := range ips {
			bitmapWords := (blocksPerAddr + 63) / 64
			addresses = append(addresses, &outsideAddressState{
				IP:            ip,
				TotalBlocks:   blocksPerAddr,
				AllocatedBits: make([]uint64, bitmapWords),
			})
		}
	}

	excluded := make(map[string]bool)
	for _, ex := range cfg.ExcludedAddresses {
		excluded[ex] = true
	}
	for _, addr := range addresses {
		if excluded[addr.IP.String()] {
			addr.Excluded = true
		}
	}

	pm.pools[name] = &poolState{
		Name:             name,
		ID:               id,
		Config:           cfg,
		OutsideAddresses: addresses,
		Subscribers:      make(map[subscriberKey]*subscriberAllocation),
	}

	return nil
}

func (pm *PoolManager) RemovePool(name string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.pools, name)
}

func (pm *PoolManager) AllocateBlock(poolName string, insideIP net.IP, insideVRF uint32, swIfIndex uint32) (*models.CGNATMapping, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	ps, ok := pm.pools[poolName]
	if !ok {
		return nil, fmt.Errorf("pool %s not found", poolName)
	}

	key := makeSubscriberKey(insideVRF, insideIP)
	sub := ps.Subscribers[key]

	maxBlocks := int(ps.Config.GetMaxBlocksPerSubscriber())
	if sub != nil && len(sub.Blocks) >= maxBlocks {
		return nil, fmt.Errorf("subscriber %s vrf %d: max blocks (%d) reached", insideIP, insideVRF, maxBlocks)
	}

	blockSize := ps.Config.GetBlockSize()
	portStart := ps.Config.GetPortRangeStart()

	var targetAddr *outsideAddressState
	if sub != nil && len(sub.Blocks) > 0 && ps.Config.GetAddressPooling() == "paired" {
		existingIP := sub.Blocks[0].OutsideIP
		for _, addr := range ps.OutsideAddresses {
			if addr.IP.Equal(existingIP) && !addr.Excluded {
				targetAddr = addr
				break
			}
		}
	}

	if targetAddr == nil {
		for _, addr := range ps.OutsideAddresses {
			if addr.Excluded {
				continue
			}
			if hasFreeBlock(addr) {
				targetAddr = addr
				break
			}
		}
	}

	if targetAddr == nil {
		return nil, fmt.Errorf("pool %s: no free blocks available", poolName)
	}

	blockIdx := allocateBlock(targetAddr)
	if blockIdx < 0 {
		return nil, fmt.Errorf("pool %s: block allocation failed on %s", poolName, targetAddr.IP)
	}

	portBlockStart := portStart + uint16(blockIdx)*blockSize
	portBlockEnd := portBlockStart + blockSize - 1

	block := blockAllocation{
		OutsideIP:      make(net.IP, 4),
		PortBlockStart: portBlockStart,
		PortBlockEnd:   portBlockEnd,
	}
	copy(block.OutsideIP, targetAddr.IP.To4())

	if sub == nil {
		sub = &subscriberAllocation{
			PoolName:  poolName,
			PoolID:    ps.ID,
			SwIfIndex: swIfIndex,
		}
		ps.Subscribers[key] = sub
	}
	sub.Blocks = append(sub.Blocks, block)

	return &models.CGNATMapping{
		PoolName:       poolName,
		PoolID:         ps.ID,
		InsideIP:       insideIP.To4(),
		InsideVRFID:    insideVRF,
		OutsideIP:      block.OutsideIP,
		PortBlockStart: portBlockStart,
		PortBlockEnd:   portBlockEnd,
		SwIfIndex:      swIfIndex,
	}, nil
}

func (pm *PoolManager) ReleaseBlocks(poolName string, insideIP net.IP, insideVRF uint32) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	ps, ok := pm.pools[poolName]
	if !ok {
		return fmt.Errorf("pool %s not found", poolName)
	}

	key := makeSubscriberKey(insideVRF, insideIP)
	sub, ok := ps.Subscribers[key]
	if !ok {
		return nil
	}

	blockSize := ps.Config.GetBlockSize()
	portStart := ps.Config.GetPortRangeStart()

	for _, block := range sub.Blocks {
		for _, addr := range ps.OutsideAddresses {
			if addr.IP.Equal(block.OutsideIP) {
				blockIdx := int(block.PortBlockStart-portStart) / int(blockSize)
				freeBlock(addr, blockIdx)
				break
			}
		}
	}

	delete(ps.Subscribers, key)
	return nil
}

func (pm *PoolManager) GetMappings(poolName string, insideIP net.IP, insideVRF uint32) []models.CGNATMapping {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	key := makeSubscriberKey(insideVRF, insideIP)

	ps, ok := pm.pools[poolName]
	if !ok {
		return nil
	}

	sub, ok := ps.Subscribers[key]
	if !ok {
		return nil
	}

	var mappings []models.CGNATMapping
	for _, block := range sub.Blocks {
		mappings = append(mappings, models.CGNATMapping{
			PoolName:       poolName,
			PoolID:         ps.ID,
			InsideIP:       insideIP.To4(),
			InsideVRFID:    insideVRF,
			OutsideIP:      block.OutsideIP,
			PortBlockStart: block.PortBlockStart,
			PortBlockEnd:   block.PortBlockEnd,
			SwIfIndex:      sub.SwIfIndex,
		})
	}
	return mappings
}

func (pm *PoolManager) GetAllMappings() []models.CGNATMapping {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var mappings []models.CGNATMapping
	for _, ps := range pm.pools {
		for key, sub := range ps.Subscribers {
			insideIP := net.IP(key.InsideIP[:])
			for _, block := range sub.Blocks {
				mappings = append(mappings, models.CGNATMapping{
					PoolName:       ps.Name,
					PoolID:         ps.ID,
					InsideIP:       insideIP,
					InsideVRFID:    key.InsideVRF,
					OutsideIP:      block.OutsideIP,
					PortBlockStart: block.PortBlockStart,
					PortBlockEnd:   block.PortBlockEnd,
					SwIfIndex:      sub.SwIfIndex,
				})
			}
		}
	}
	return mappings
}

func (pm *PoolManager) GetPoolStats(poolName string) *models.CGNATPoolStats {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	ps, ok := pm.pools[poolName]
	if !ok {
		return nil
	}

	var totalBlocks, allocatedBlocks, excludedAddrs uint32
	for _, addr := range ps.OutsideAddresses {
		if addr.Excluded {
			excludedAddrs++
			continue
		}
		totalBlocks += addr.TotalBlocks
		for _, word := range addr.AllocatedBits {
			allocatedBlocks += uint32(popcount(word))
		}
	}

	var freeBlocks uint32
	if totalBlocks > allocatedBlocks {
		freeBlocks = totalBlocks - allocatedBlocks
	}

	var utilization float64
	if totalBlocks > 0 {
		utilization = float64(allocatedBlocks) / float64(totalBlocks)
	}

	return &models.CGNATPoolStats{
		Name:               poolName,
		Mode:               ps.Config.GetMode(),
		TotalAddresses:     uint32(len(ps.OutsideAddresses)),
		AllocatedAddresses: allocatedBlocks,
		FreeBlocks:         freeBlocks,
		TotalBlocks:        totalBlocks,
		ExcludedAddresses:  excludedAddrs,
		SubscriberCount:    uint32(len(ps.Subscribers)),
		Utilization:        utilization,
	}
}

func (pm *PoolManager) FindPoolForIP(insideIP net.IP, insideVRF string) string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	ip4 := insideIP.To4()
	if ip4 == nil {
		return ""
	}

	for name, ps := range pm.pools {
		for _, prefix := range ps.Config.InsidePrefixes {
			if prefix.VRF != "" && prefix.VRF != insideVRF {
				continue
			}
			_, ipNet, err := net.ParseCIDR(prefix.Prefix)
			if err != nil {
				continue
			}
			if ipNet.Contains(ip4) {
				return name
			}
		}
	}
	return ""
}

func (pm *PoolManager) RestoreMapping(mapping *models.CGNATMapping) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	ps, ok := pm.pools[mapping.PoolName]
	if !ok {
		return fmt.Errorf("pool %s not found", mapping.PoolName)
	}

	blockSize := ps.Config.GetBlockSize()
	portStart := ps.Config.GetPortRangeStart()

	for _, addr := range ps.OutsideAddresses {
		if addr.IP.Equal(mapping.OutsideIP) {
			blockIdx := int(mapping.PortBlockStart-portStart) / int(blockSize)
			word := blockIdx / 64
			bit := uint(blockIdx % 64)
			if word < len(addr.AllocatedBits) {
				addr.AllocatedBits[word] |= 1 << bit
			}
			break
		}
	}

	key := makeSubscriberKey(mapping.InsideVRFID, mapping.InsideIP)
	sub, ok := ps.Subscribers[key]
	if !ok {
		sub = &subscriberAllocation{
			PoolName:  mapping.PoolName,
			PoolID:    ps.ID,
			SwIfIndex: mapping.SwIfIndex,
		}
		ps.Subscribers[key] = sub
	}

	sub.Blocks = append(sub.Blocks, blockAllocation{
		OutsideIP:      mapping.OutsideIP,
		PortBlockStart: mapping.PortBlockStart,
		PortBlockEnd:   mapping.PortBlockEnd,
	})

	return nil
}

func makeSubscriberKey(vrf uint32, ip net.IP) subscriberKey {
	var key subscriberKey
	key.InsideVRF = vrf
	ip4 := ip.To4()
	if ip4 != nil {
		copy(key.InsideIP[:], ip4)
	}
	return key
}

func expandCIDR(cidr string) ([]net.IP, error) {
	ip := net.ParseIP(cidr)
	if ip != nil {
		return []net.IP{ip.To4()}, nil
	}

	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	var ips []net.IP
	mask := ipNet.Mask
	ones, bits := mask.Size()
	if bits != 32 {
		return nil, fmt.Errorf("only IPv4 supported")
	}

	count := uint32(1) << uint(bits-ones)
	base := ipToU32(ipNet.IP.To4())

	for i := uint32(0); i < count; i++ {
		ip := u32ToIP(base + i)
		ips = append(ips, ip)
	}

	return ips, nil
}

func ipToU32(ip net.IP) uint32 {
	ip4 := ip.To4()
	return uint32(ip4[0])<<24 | uint32(ip4[1])<<16 | uint32(ip4[2])<<8 | uint32(ip4[3])
}

func u32ToIP(v uint32) net.IP {
	return net.IPv4(byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

func hasFreeBlock(addr *outsideAddressState) bool {
	allocated := uint32(0)
	for _, word := range addr.AllocatedBits {
		allocated += uint32(popcount(word))
	}
	return allocated < addr.TotalBlocks
}

func allocateBlock(addr *outsideAddressState) int {
	for i, word := range addr.AllocatedBits {
		if word == ^uint64(0) {
			continue
		}
		for bit := uint(0); bit < 64; bit++ {
			blockIdx := i*64 + int(bit)
			if uint32(blockIdx) >= addr.TotalBlocks {
				return -1
			}
			if word&(1<<bit) == 0 {
				addr.AllocatedBits[i] |= 1 << bit
				return blockIdx
			}
		}
	}
	return -1
}

func freeBlock(addr *outsideAddressState, blockIdx int) {
	word := blockIdx / 64
	bit := uint(blockIdx % 64)
	if word < len(addr.AllocatedBits) {
		addr.AllocatedBits[word] &^= 1 << bit
	}
}

func popcount(x uint64) int {
	x = x - ((x >> 1) & 0x5555555555555555)
	x = (x & 0x3333333333333333) + ((x >> 2) & 0x3333333333333333)
	x = (x + (x >> 4)) & 0x0f0f0f0f0f0f0f0f
	return int((x * 0x0101010101010101) >> 56)
}
