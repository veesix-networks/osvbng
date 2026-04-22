// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package relay

import (
	"net"
	"sync/atomic"
	"time"
)

type Server struct {
	Addr     *net.UDPAddr
	Priority int

	failures  atomic.Int32
	dead      atomic.Bool
	deadSince atomic.Int64
	requests  atomic.Uint64
	timeouts  atomic.Uint64
}

type ServerStatus struct {
	Address  string `json:"address" prometheus:"label"`
	Priority int    `json:"priority"`
	Dead     bool   `json:"dead"`
	Failures int    `json:"failures"`
	Requests uint64 `json:"requests" prometheus:"name=osvbng_dhcp_relay_server_requests,help=Requests sent to DHCP relay server,type=counter"`
	Timeouts uint64 `json:"timeouts" prometheus:"name=osvbng_dhcp_relay_server_timeouts,help=Timeouts from DHCP relay server,type=counter"`
}

func (s *Server) GetStatus(deadTime time.Duration) ServerStatus {
	return ServerStatus{
		Address:  s.Addr.String(),
		Priority: s.Priority,
		Dead:     s.IsDead(deadTime),
		Failures: int(s.failures.Load()),
		Requests: s.requests.Load(),
		Timeouts: s.timeouts.Load(),
	}
}

func (s *Server) IsDead(deadTime time.Duration) bool {
	if !s.dead.Load() {
		return false
	}
	since := time.Unix(s.deadSince.Load(), 0)
	if time.Since(since) >= deadTime {
		s.dead.Store(false)
		s.failures.Store(0)
		return false
	}
	return true
}

func (s *Server) RecordFailure(threshold int) {
	s.timeouts.Add(1)
	if int(s.failures.Add(1)) >= threshold {
		s.dead.Store(true)
		s.deadSince.Store(time.Now().Unix())
	}
}

func (s *Server) RecordSuccess() {
	s.failures.Store(0)
	s.dead.Store(false)
}
