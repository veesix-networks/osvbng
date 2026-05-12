// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package relay

import (
	"net"
	"sync/atomic"
	"time"

	"github.com/veesix-networks/osvbng/pkg/netbind"
)

// Family discriminates the local socket port (67 for v4, 547 for v6).
type Family int

const (
	FamilyV4 Family = 4
	FamilyV6 Family = 6
)

func (f Family) localPort() int {
	switch f {
	case FamilyV6:
		return 547
	default:
		return 67
	}
}

func (f Family) network() string {
	switch f {
	case FamilyV6:
		return "udp6"
	default:
		return "udp4"
	}
}

// bindingKey identifies the local socket end of a relay flow. Two
// servers in different VRFs share neither cache entry nor socket,
// even if they happen to use the same upstream IP.
type bindingKey struct {
	Family    Family
	VRF       string
	SourceIP  string // canonical netip.Addr.String() ("" if unbound)
	LocalPort int
}

func makeBindingKey(family Family, b netbind.Binding) bindingKey {
	src := ""
	if b.SourceIP.IsValid() {
		src = b.SourceIP.String()
	}
	return bindingKey{
		Family:    family,
		VRF:       b.VRF,
		SourceIP:  src,
		LocalPort: family.localPort(),
	}
}

type Server struct {
	Addr     *net.UDPAddr
	Priority int
	Family   Family
	Binding  netbind.Binding

	failures  atomic.Int32
	dead      atomic.Bool
	deadSince atomic.Int64
	requests  atomic.Uint64
	timeouts  atomic.Uint64
}

func (s *Server) bindingKey() bindingKey {
	return makeBindingKey(s.Family, s.Binding)
}

type ServerStatus struct {
	Address  string `json:"address"        metric:"label"`
	VRF      string `json:"vrf,omitempty"  metric:"label"`
	Priority int    `json:"priority"`
	Dead     bool   `json:"dead"           metric:"name=dhcp.relay.server.dead,type=gauge,help=1 if this DHCP relay server is currently marked dead."`
	Failures int    `json:"failures"       metric:"name=dhcp.relay.server.failures,type=counter,help=Consecutive failures observed for this DHCP relay server."`
	Requests uint64 `json:"requests"       metric:"name=dhcp.relay.server.requests,type=counter,help=Requests sent to this DHCP relay server."`
	Timeouts uint64 `json:"timeouts"       metric:"name=dhcp.relay.server.timeouts,type=counter,help=Timeouts from this DHCP relay server."`
}

func (s *Server) GetStatus(deadTime time.Duration) ServerStatus {
	return ServerStatus{
		Address:  s.Addr.String(),
		VRF:      s.Binding.VRF,
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
