package aaa

import (
	"net"
	"sync"
	"time"
)

type RADIUSStats struct {
	mu sync.Mutex

	servers map[string]*ServerStats
}

type ServerStats struct {
	Address       string    `json:"address"`
	AuthRequests  uint64    `json:"auth_requests"`
	AuthAccepts   uint64    `json:"auth_accepts"`
	AuthRejects   uint64    `json:"auth_rejects"`
	AuthTimeouts  uint64    `json:"auth_timeouts"`
	AuthErrors    uint64    `json:"auth_errors"`
	AcctRequests  uint64    `json:"acct_requests"`
	AcctResponses uint64    `json:"acct_responses"`
	AcctTimeouts  uint64    `json:"acct_timeouts"`
	AcctErrors    uint64    `json:"acct_errors"`
	LastError     string    `json:"last_error"`
	LastErrorTime time.Time `json:"last_error_time"`
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
	if err != nil {
		stats.LastError = err.Error()
	}
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
	if err != nil {
		stats.LastError = err.Error()
	}
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

func (s *RADIUSStats) GetAllStatsSnapshot() []*ServerStats {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := make([]*ServerStats, 0, len(s.servers))
	for addr, stats := range s.servers {
		snapshot := &ServerStats{
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
			LastError:     stats.LastError,
			LastErrorTime: stats.LastErrorTime,
		}
		result = append(result, snapshot)
	}
	return result
}
