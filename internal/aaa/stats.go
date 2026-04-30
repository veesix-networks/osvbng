// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package aaa

import (
	"net"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

// ServerStats is the per-RADIUS-server view used by the show command and
// the telemetry registry. Counter fields are populated at snapshot time
// from the SDK; LastError and LastErrorTime are guarded by metaMu in
// RADIUSStats.
type ServerStats struct {
	Address       string    `json:"address"             metric:"label"`
	AuthRequests  uint64    `json:"auth_requests"       metric:"name=auth.requests,type=counter,help=RADIUS Access-Request packets sent."`
	AuthAccepts   uint64    `json:"auth_accepts"        metric:"name=auth.accepts,type=counter,help=RADIUS Access-Accept responses."`
	AuthRejects   uint64    `json:"auth_rejects"        metric:"name=auth.rejects,type=counter,help=RADIUS Access-Reject responses."`
	AuthTimeouts  uint64    `json:"auth_timeouts"       metric:"name=auth.timeouts,type=counter,help=RADIUS auth request timeouts."`
	AuthErrors    uint64    `json:"auth_errors"         metric:"name=auth.errors,type=counter,help=RADIUS auth errors (transport, decode, unexpected code)."`
	AcctRequests  uint64    `json:"acct_requests"       metric:"name=acct.requests,type=counter,help=RADIUS accounting requests sent."`
	AcctResponses uint64    `json:"acct_responses"      metric:"name=acct.responses,type=counter,help=RADIUS accounting responses received."`
	AcctTimeouts  uint64    `json:"acct_timeouts"       metric:"name=acct.timeouts,type=counter,help=RADIUS accounting request timeouts."`
	AcctErrors    uint64    `json:"acct_errors"         metric:"name=acct.errors,type=counter,help=RADIUS accounting errors."`
	LastError     string    `json:"last_error"`
	LastErrorTime time.Time `json:"last_error_time"`
}

var radiusMetrics = telemetry.MustRegisterStruct[ServerStats](telemetry.RegisterOpts{
	Path: "aaa.radius",
})

type RADIUSStats struct {
	metrics *telemetry.StructMetrics[ServerStats]
	handles sync.Map // host string -> *telemetry.StructHandles

	metaMu sync.Mutex
	meta   map[string]*serverMetadata
}

type serverMetadata struct {
	LastError     string
	LastErrorTime time.Time
}

func NewRADIUSStats() *RADIUSStats {
	return &RADIUSStats{
		metrics: radiusMetrics,
		meta:    make(map[string]*serverMetadata),
	}
}

func NewRADIUSStatsWithRegistry(reg *telemetry.Registry) *RADIUSStats {
	return &RADIUSStats{
		metrics: telemetry.MustRegisterStructWith[ServerStats](reg, telemetry.RegisterOpts{Path: "aaa.radius"}),
		meta:    make(map[string]*serverMetadata),
	}
}

func extractHost(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

func (s *RADIUSStats) resolve(addr string) *telemetry.StructHandles {
	host := extractHost(addr)
	if v, ok := s.handles.Load(host); ok {
		return v.(*telemetry.StructHandles)
	}
	candidate := s.metrics.WithLabelValues(host)
	actual, _ := s.handles.LoadOrStore(host, candidate)
	return actual.(*telemetry.StructHandles)
}

func (s *RADIUSStats) IncrAuthRequest(addr string) { s.resolve(addr).Inc("AuthRequests") }
func (s *RADIUSStats) IncrAuthAccept(addr string)  { s.resolve(addr).Inc("AuthAccepts") }
func (s *RADIUSStats) IncrAuthReject(addr string)  { s.resolve(addr).Inc("AuthRejects") }
func (s *RADIUSStats) IncrAuthTimeout(addr string) { s.resolve(addr).Inc("AuthTimeouts") }
func (s *RADIUSStats) IncrAcctRequest(addr string) { s.resolve(addr).Inc("AcctRequests") }
func (s *RADIUSStats) IncrAcctResponse(addr string) {
	s.resolve(addr).Inc("AcctResponses")
}
func (s *RADIUSStats) IncrAcctTimeout(addr string) { s.resolve(addr).Inc("AcctTimeouts") }

func (s *RADIUSStats) IncrAuthError(addr string, err error) {
	s.resolve(addr).Inc("AuthErrors")
	if err != nil {
		s.recordError(extractHost(addr), err)
	}
}

func (s *RADIUSStats) IncrAcctError(addr string, err error) {
	s.resolve(addr).Inc("AcctErrors")
	if err != nil {
		s.recordError(extractHost(addr), err)
	}
}

func (s *RADIUSStats) recordError(host string, err error) {
	s.metaMu.Lock()
	m, ok := s.meta[host]
	if !ok {
		m = &serverMetadata{}
		s.meta[host] = m
	}
	m.LastError = err.Error()
	m.LastErrorTime = time.Now()
	s.metaMu.Unlock()
}

func (s *RADIUSStats) GetServerStats(addr string) *ServerStats {
	host := extractHost(addr)
	v, ok := s.handles.Load(host)
	if !ok {
		return nil
	}
	return s.snapshot(host, v.(*telemetry.StructHandles))
}

func (s *RADIUSStats) GetAllStats() map[string]*ServerStats {
	out := make(map[string]*ServerStats)
	s.handles.Range(func(k, v any) bool {
		host := k.(string)
		out[host] = s.snapshot(host, v.(*telemetry.StructHandles))
		return true
	})
	return out
}

func (s *RADIUSStats) GetAllStatsSnapshot() []*ServerStats {
	var out []*ServerStats
	s.handles.Range(func(k, v any) bool {
		out = append(out, s.snapshot(k.(string), v.(*telemetry.StructHandles)))
		return true
	})
	return out
}

func (s *RADIUSStats) snapshot(host string, h *telemetry.StructHandles) *ServerStats {
	v := &ServerStats{Address: host}
	s.metrics.FillSnapshot(h, v)
	s.metaMu.Lock()
	if m, ok := s.meta[host]; ok {
		v.LastError = m.LastError
		v.LastErrorTime = m.LastErrorTime
	}
	s.metaMu.Unlock()
	return v
}
