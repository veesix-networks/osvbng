// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"net"
	"sync"

	"github.com/veesix-networks/osvbng/pkg/models"
)

type reverseKey struct {
	OutsideIP  [4]byte
	PortStart  uint16
}

type ReverseIndex struct {
	mu      sync.RWMutex
	byBlock map[reverseKey]*models.CGNATMapping
	byIP    map[[4]byte][]*models.CGNATMapping
}

func NewReverseIndex() *ReverseIndex {
	return &ReverseIndex{
		byBlock: make(map[reverseKey]*models.CGNATMapping),
		byIP:    make(map[[4]byte][]*models.CGNATMapping),
	}
}

func (ri *ReverseIndex) Add(mapping *models.CGNATMapping) {
	ri.mu.Lock()
	defer ri.mu.Unlock()

	key := makeReverseKey(mapping.OutsideIP, mapping.PortBlockStart)
	ri.byBlock[key] = mapping

	var ipKey [4]byte
	copy(ipKey[:], mapping.OutsideIP.To4())
	ri.byIP[ipKey] = append(ri.byIP[ipKey], mapping)
}

func (ri *ReverseIndex) Remove(outsideIP net.IP, portStart uint16) {
	ri.mu.Lock()
	defer ri.mu.Unlock()

	key := makeReverseKey(outsideIP, portStart)
	mapping, ok := ri.byBlock[key]
	if !ok {
		return
	}
	delete(ri.byBlock, key)

	var ipKey [4]byte
	copy(ipKey[:], outsideIP.To4())
	mappings := ri.byIP[ipKey]
	for i, m := range mappings {
		if m == mapping {
			ri.byIP[ipKey] = append(mappings[:i], mappings[i+1:]...)
			break
		}
	}
	if len(ri.byIP[ipKey]) == 0 {
		delete(ri.byIP, ipKey)
	}
}

func (ri *ReverseIndex) Lookup(outsideIP net.IP, port uint16) *models.CGNATMapping {
	ri.mu.RLock()
	defer ri.mu.RUnlock()

	var ipKey [4]byte
	copy(ipKey[:], outsideIP.To4())

	mappings, ok := ri.byIP[ipKey]
	if !ok {
		return nil
	}

	for _, m := range mappings {
		if port >= m.PortBlockStart && port <= m.PortBlockEnd {
			return m
		}
	}
	return nil
}

func (ri *ReverseIndex) LookupByIP(outsideIP net.IP) []*models.CGNATMapping {
	ri.mu.RLock()
	defer ri.mu.RUnlock()

	var ipKey [4]byte
	copy(ipKey[:], outsideIP.To4())

	src := ri.byIP[ipKey]
	if len(src) == 0 {
		return nil
	}

	result := make([]*models.CGNATMapping, len(src))
	copy(result, src)
	return result
}

func makeReverseKey(ip net.IP, portStart uint16) reverseKey {
	var key reverseKey
	ip4 := ip.To4()
	if ip4 != nil {
		copy(key.OutsideIP[:], ip4)
	}
	key.PortStart = portStart
	return key
}
