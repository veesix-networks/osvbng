// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"net"
	"sync"
	"time"

	l2tppkt "github.com/veesix-networks/osvbng/pkg/l2tp"
)

// Tunnel is the control-plane state for one L2TPv2 control connection.
// All access to mutable fields is serialised through `mu`.
type Tunnel struct {
	mu sync.Mutex

	LocalIP  net.IP
	PeerIP   net.IP
	LocalID  uint16
	PeerID   uint16
	LocalPort uint16
	PeerPort  uint16

	Role     l2tppkt.TunnelRole
	FSM      *l2tppkt.TunnelFSM
	Channel  *l2tppkt.ControlChannel

	// Hostname exchanged in Host Name AVPs.
	LocalHostname string
	PeerHostname  string

	// Shared secret used for the Challenge AVP exchange. Empty disables
	// authentication for this tunnel.
	Secret []byte

	// outstandingChallenge holds the random nonce the LNS sent to the
	// LAC in SCCRP. The matching response arrives in SCCCN and is
	// verified against this value, then cleared. Empty when the LNS
	// chose not to challenge the peer (cfg.ChallengeRequired=false or
	// no Secret configured).
	outstandingChallenge []byte

	// Per-tunnel session map keyed by local session ID.
	Sessions map[uint16]*Session

	// Hello / liveness state.
	HelloInterval time.Duration

	// CreatedAt marks when the tunnel object was instantiated (not when
	// it reached Established).
	CreatedAt time.Time

	// ref_count of sessions currently bound. Used by teardown to defer
	// tunnel deletion until the last session is gone.
	refCount int
}

func (t *Tunnel) addSession(s *Session) {
	t.mu.Lock()
	if t.Sessions == nil {
		t.Sessions = make(map[uint16]*Session)
	}
	t.Sessions[s.LocalID] = s
	t.refCount++
	t.mu.Unlock()
}

func (t *Tunnel) removeSession(localID uint16) (empty bool) {
	t.mu.Lock()
	delete(t.Sessions, localID)
	t.refCount--
	empty = t.refCount <= 0
	t.mu.Unlock()
	return empty
}

func (t *Tunnel) snapshotSessions() []*Session {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]*Session, 0, len(t.Sessions))
	for _, s := range t.Sessions {
		out = append(out, s)
	}
	return out
}
