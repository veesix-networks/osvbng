package aaa

import (
	"net"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/models/aaa"
)

type RADIUSStats struct {
	mu sync.Mutex

	servers map[string]*ServerStats
}

type ServerStats struct {
	AuthRequests uint64
	AuthAccepts  uint64
	AuthRejects  uint64
	AuthTimeouts uint64
	AuthErrors   uint64

	AcctRequests  uint64
	AcctResponses uint64
	AcctTimeouts  uint64
	AcctErrors    uint64

	LastError     error
	LastErrorTime time.Time
}

func NewRADIUSStats() *RADIUSStats {
	return &RADIUSStats{
		servers: make(map[string]*ServerStats),
	}
}

func (s *RADIUSStats) getOrCreateServerLocked(addr string) *ServerStats {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}

	if _, exists := s.servers[host]; !exists {
		s.servers[host] = &ServerStats{}
	}
	return s.servers[host]
}

func (s *RADIUSStats) IncrAuthRequest(addr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getOrCreateServerLocked(addr).AuthRequests++
}

func (s *RADIUSStats) IncrAuthAccept(addr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getOrCreateServerLocked(addr).AuthAccepts++
}

func (s *RADIUSStats) IncrAuthReject(addr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getOrCreateServerLocked(addr).AuthRejects++
}

func (s *RADIUSStats) IncrAuthTimeout(addr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getOrCreateServerLocked(addr).AuthTimeouts++
}

func (s *RADIUSStats) IncrAuthError(addr string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stats := s.getOrCreateServerLocked(addr)
	stats.AuthErrors++
	stats.LastError = err
	stats.LastErrorTime = time.Now()
}

func (s *RADIUSStats) IncrAcctRequest(addr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getOrCreateServerLocked(addr).AcctRequests++
}

func (s *RADIUSStats) IncrAcctResponse(addr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getOrCreateServerLocked(addr).AcctResponses++
}

func (s *RADIUSStats) IncrAcctTimeout(addr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getOrCreateServerLocked(addr).AcctTimeouts++
}

func (s *RADIUSStats) IncrAcctError(addr string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stats := s.getOrCreateServerLocked(addr)
	stats.AcctErrors++
	stats.LastError = err
	stats.LastErrorTime = time.Now()
}

func (s *RADIUSStats) GetServerStats(addr string) *ServerStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.servers[addr]
}

func (s *RADIUSStats) GetAllStats() map[string]*ServerStats {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make(map[string]*ServerStats)
	for addr, stats := range s.servers {
		statsCopy := *stats
		result[addr] = &statsCopy
	}
	return result
}

func (s *RADIUSStats) GetAllStatsSnapshot() []*aaa.ServerStats {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]*aaa.ServerStats, 0, len(s.servers))
	for addr, stats := range s.servers {
		snapshot := &aaa.ServerStats{
			Address:       addr,
			AuthRequests:  stats.AuthRequests,
			AuthAccepts:   stats.AuthAccepts,
			AuthRejects:   stats.AuthRejects,
			AuthTimeouts:  stats.AuthTimeouts,
			AuthErrors:    stats.AuthErrors,
			AcctRequests:  stats.AcctRequests,
			AcctResponses: stats.AcctResponses,
			AcctTimeouts:  stats.AcctTimeouts,
			AcctErrors:    stats.AcctErrors,
			LastErrorTime: stats.LastErrorTime,
		}
		if stats.LastError != nil {
			snapshot.LastError = stats.LastError.Error()
		}
		result = append(result, snapshot)
	}
	return result
}
