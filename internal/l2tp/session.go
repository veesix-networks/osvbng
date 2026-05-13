// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"fmt"
	"net"
	"sync"
	"time"

	pppdisp "github.com/veesix-networks/osvbng/internal/ppp"
	"github.com/veesix-networks/osvbng/pkg/allocator"
	l2tppkt "github.com/veesix-networks/osvbng/pkg/l2tp"
	"github.com/veesix-networks/osvbng/pkg/ppp"
	"github.com/veesix-networks/osvbng/pkg/svcgroup"
)

// Session is the control-plane state for one L2TPv2 session. LAC and
// LNS roles share the struct; `Role` discriminates.
type Session struct {
	mu sync.Mutex

	// SessionID is the BNG-wide session identifier published in
	// lifecycle / AAA / opdb events. Distinct from the L2TP wire IDs
	// (LocalID / PeerID) which are tunnel-scoped 16-bit values.
	SessionID     string
	AcctSessionID string

	Tunnel  *Tunnel
	LocalID uint16
	PeerID  uint16

	Role l2tppkt.SessionRole
	FSM  *l2tppkt.SessionFSM

	// LNS-only: the PPP stack terminating on us. `lcp/ipcp/ipv6cp/pap/chap`
	// are owned by the session and wired into the dispatcher in the
	// LNS bring-up path.
	LCP           *ppp.LCP
	IPCP          *ppp.IPCP
	IPv6CP        *ppp.IPv6CP
	PAP           *ppp.PAPHandler
	CHAP          *ppp.CHAPHandler
	PPPDispatcher *pppdisp.Dispatcher
	Phase         ppp.Phase

	// Auth state (LNS).
	pendingAuthRequestID string
	pendingAuthType      string
	pendingPAPID         uint8
	pendingCHAPID        uint8
	chapID               uint8
	chapChallenge        []byte
	chapRetryTimer       *time.Timer
	chapRetryCount       int

	// Attribute bag populated from AAA response (LNS).
	Attributes map[string]string

	// Resolved subscriber-context (LNS).
	VRF          string
	ServiceGroup svcgroup.ServiceGroup
	SRGName      string
	AllocCtx     *allocator.Context

	// Subscriber identity once auth and NCP complete (LNS).
	Username    string
	IPv4Address net.IP
	IPv6Address net.IP
	IPv6Prefix  *net.IPNet

	// Pools the session was allocated from (LNS). Empty if the address
	// came from AAA Framed-IP-* rather than a local pool.
	allocatedPool     string
	allocatedIANAPool string
	allocatedPDPool   string

	// NCP convergence flags (LNS).
	ipcpOpen        bool
	ipv6cpOpen      bool
	programmedInVPP bool
	LCPMagic        uint32

	// VPP-side state (LNS).
	SwIfIndex    uint32
	DecapFIBIdx  uint32
	EncapIfIndex uint32

	// LAC-only: which PPPoE session we are bridged to.
	PPPoESessionID uint16
	PPPoESwIfIndex uint32

	ActivatedAt time.Time
	BoundAt     time.Time
}

// makeSessionID builds the BNG-wide session ID used in lifecycle and
// AAA events. Tunnel + local session ID is unique across the BNG since
// tunnel IDs are scoped to (peer-ip, local-tunnel-id).
func makeSessionID(peerIP net.IP, localTunnelID, localSessionID uint16) string {
	return fmt.Sprintf("l2tp:%s:%d:%d", peerIP.String(), localTunnelID, localSessionID)
}
