// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package radius

import "sync"

type CoAStats struct {
	mu            sync.Mutex
	clients       map[string]*CoAClientStats
	UnknownClient uint64
}

type CoAClientStats struct {
	Address            string `json:"address"`
	CoARequests        uint64 `json:"coa_requests"`
	CoAACKs            uint64 `json:"coa_acks"`
	CoANAKs            uint64 `json:"coa_naks"`
	DisconnectRequests uint64 `json:"disconnect_requests"`
	DisconnectACKs     uint64 `json:"disconnect_acks"`
	DisconnectNAKs     uint64 `json:"disconnect_naks"`
	Overflow           uint64 `json:"coa_overflow"`
	InvalidAuth        uint64 `json:"coa_invalid_auth"`
	SessionNotFound    uint64 `json:"coa_session_not_found"`
}

func NewCoAStats() *CoAStats {
	return &CoAStats{
		clients: make(map[string]*CoAClientStats),
	}
}

func (s *CoAStats) getOrCreate(addr string) *CoAClientStats {
	if _, exists := s.clients[addr]; !exists {
		s.clients[addr] = &CoAClientStats{Address: addr}
	}
	return s.clients[addr]
}

func (s *CoAStats) IncrCoARequest(addr string)       { s.mu.Lock(); s.getOrCreate(addr).CoARequests++; s.mu.Unlock() }
func (s *CoAStats) IncrCoAACK(addr string)            { s.mu.Lock(); s.getOrCreate(addr).CoAACKs++; s.mu.Unlock() }
func (s *CoAStats) IncrCoANAK(addr string)            { s.mu.Lock(); s.getOrCreate(addr).CoANAKs++; s.mu.Unlock() }
func (s *CoAStats) IncrDisconnectRequest(addr string) { s.mu.Lock(); s.getOrCreate(addr).DisconnectRequests++; s.mu.Unlock() }
func (s *CoAStats) IncrDisconnectACK(addr string)     { s.mu.Lock(); s.getOrCreate(addr).DisconnectACKs++; s.mu.Unlock() }
func (s *CoAStats) IncrDisconnectNAK(addr string)     { s.mu.Lock(); s.getOrCreate(addr).DisconnectNAKs++; s.mu.Unlock() }
func (s *CoAStats) IncrOverflow(addr string)          { s.mu.Lock(); s.getOrCreate(addr).Overflow++; s.mu.Unlock() }
func (s *CoAStats) IncrInvalidAuth(addr string)       { s.mu.Lock(); s.getOrCreate(addr).InvalidAuth++; s.mu.Unlock() }
func (s *CoAStats) IncrSessionNotFound(addr string)   { s.mu.Lock(); s.getOrCreate(addr).SessionNotFound++; s.mu.Unlock() }
func (s *CoAStats) IncrUnknownClient()                { s.mu.Lock(); s.UnknownClient++; s.mu.Unlock() }

func (s *CoAStats) GetAllStatsSnapshot() []*CoAClientStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*CoAClientStats, 0, len(s.clients))
	for _, stats := range s.clients {
		cp := *stats
		result = append(result, &cp)
	}
	return result
}
