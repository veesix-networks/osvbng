// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"context"
	"errors"
	"net"
	"sync"

	"github.com/veesix-networks/osvbng/pkg/allocator"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/svcgroup"
	"github.com/veesix-networks/osvbng/pkg/vrfmgr"
)

// Component is the L2TPv2 control-plane orchestrator. It owns the per-
// peer tunnel pool and the per-tunnel session pool, drives the
// reliable control channel for each tunnel, dispatches inbound control
// messages to the right FSM, and wires AAA / HA / opdb integration.
//
// LAC and LNS bring-up flows are mounted as sub-handlers (lac.go,
// lns.go) once Phase 5.5b / 5.7 lands; this file holds only the
// shared lifecycle and lookup machinery.
type Component struct {
	log *logger.Logger

	mu sync.RWMutex

	// Tunnels indexed by (peer_ip, local_tunnel_id) — incoming-packet
	// lookup key. Lookup is on the hot path; reads are RLocked.
	tunnels map[tunnelKey]*Tunnel

	// Per-tunnel runners (control-channel tick + Hello scheduling).
	runners map[tunnelKey]*tunnelRunner

	// Tunnel-ID allocator keyed by peer_ip (RFC 2661 §3.1 scope).
	tunnelIDs map[string]*IDAllocator

	// resolveLNSConfig maps an inbound SCCRQ's Host Name AVP to the
	// LNSConfig for that peer. Injected by the cmd-level wiring so the
	// component does not depend on pkg/config/l2tp directly.
	resolveLNSConfig func(hostname string) (LNSConfig, bool)

	// send transmits a control packet. Injected because the right TX
	// path depends on topology (kernel socket, VPP punt-egress L3, TAP).
	send SendControlFn

	// puntCh is the inbound control-packet channel fed by the
	// dataplane component.
	puntCh <-chan *dataplane.ParsedPacket

	// Optional dependencies wired by the cmd layer. Each is nil-checked
	// at every use site; an L2TP component without AAA / allocator /
	// southbound still serves tunnel-only smoke tests.
	eventBus         events.Bus
	cfgMgr           component.ConfigManager
	registry         *allocator.Registry
	vpp              southbound.Southbound
	vrfMgr           *vrfmgr.Manager
	svcGroupResolver *svcgroup.Resolver
	localHostname    string

	aaaRespSub events.Subscription

	// LAC-side state. lacPending holds the in-flight bring-up requests
	// keyed by tunnel identity; SCCRP and ICRP arrival looks up the
	// originating LACBringUpRequest to forward proxy-auth AVPs into
	// ICCN and to address the final TopicL2TPLACDecision event.
	lacMu      sync.Mutex
	lacPending map[tunnelKey]*LACBringUpRequest

	// denylist tracks LNS peers the LAC has temporarily given up on.
	// Allocated lazily so test setups that never exercise the LAC path
	// don't pay the map cost.
	denylist *PeerDenylist

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// SetSendControlFn installs the outbound-control-packet transmitter.
func (c *Component) SetSendControlFn(fn SendControlFn) { c.send = fn }

// SetPuntChannel installs the inbound L2TP punt channel. Called by the
// cmd-level wiring after both the dataplane and l2tp components have
// been created.
func (c *Component) SetPuntChannel(ch <-chan *dataplane.ParsedPacket) {
	c.puntCh = ch
}

// Name implements component.Component.
func (c *Component) Name() string { return "l2tp" }

// SetLNSConfigResolver installs the callback used to authorize and
// configure inbound LAC peers.
func (c *Component) SetLNSConfigResolver(fn func(hostname string) (LNSConfig, bool)) {
	c.resolveLNSConfig = fn
}

// SetEventBus installs the event bus used for AAA request/response and
// session-lifecycle traffic. Required for LNS auth and IP allocation.
func (c *Component) SetEventBus(bus events.Bus) { c.eventBus = bus }

// SetConfigManager installs the running-config provider used to resolve
// AAA policies and subscriber-group bindings on the LNS path.
func (c *Component) SetConfigManager(m component.ConfigManager) { c.cfgMgr = m }

// SetAllocator installs the IP allocator registry. LNS uses it to draw
// IPv4 / IPv6 IANA / IPv6 PD from the configured pools on NCP up.
func (c *Component) SetAllocator(r *allocator.Registry) { c.registry = r }

// SetSouthbound installs the dataplane interface used to program
// per-session vnet interfaces in VPP.
func (c *Component) SetSouthbound(s southbound.Southbound) { c.vpp = s }

// SetVRFManager installs the VRF table-ID resolver. Required to map a
// subscriber-group VRF name to the FIB index passed to AddL2TPSessionIP.
func (c *Component) SetVRFManager(m *vrfmgr.Manager) { c.vrfMgr = m }

// SetServiceGroupResolver installs the service-group resolver used to
// merge AAA / subscriber-group / config-default service tags on the
// LNS path.
func (c *Component) SetServiceGroupResolver(r *svcgroup.Resolver) { c.svcGroupResolver = r }

// SetLocalHostname records the LNS hostname used as the CHAP challenge
// name and as the default Host Name in outbound SCCRQ.
func (c *Component) SetLocalHostname(h string) { c.localHostname = h }

type tunnelKey struct {
	peerIP [16]byte
	id     uint16
}

func makeTunnelKey(peerIP net.IP, id uint16) tunnelKey {
	k := tunnelKey{id: id}
	v4 := peerIP.To4()
	if v4 != nil {
		copy(k.peerIP[12:], v4)
		k.peerIP[10] = 0xff
		k.peerIP[11] = 0xff
	} else {
		copy(k.peerIP[:], peerIP.To16())
	}
	return k
}

func New(log *logger.Logger) *Component {
	return &Component{
		log:       log,
		tunnels:   make(map[tunnelKey]*Tunnel),
		tunnelIDs: make(map[string]*IDAllocator),
		denylist:  NewPeerDenylist(),
	}
}

// Denylist returns the LAC peer denylist. Exposed so external callers
// (operational CLI, HA restore) can inspect or seed entries.
func (c *Component) Denylist() *PeerDenylist { return c.denylist }

// Start brings the component up. Launches the punt-channel consumer
// if a channel has been installed via SetPuntChannel and subscribes to
// AAA response traffic if an event bus is wired.
func (c *Component) Start(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	if c.puntCh != nil {
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			c.puntConsumer(runCtx, c.puntCh)
		}()
	}
	if c.eventBus != nil {
		c.aaaRespSub = c.eventBus.Subscribe(events.TopicAAAResponseL2TP, c.handleAAAResponse)
	}
	return nil
}

// Stop tears the component down. Stops all per-tunnel runners and the
// punt consumer.
func (c *Component) Stop(ctx context.Context) error {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()

	if c.aaaRespSub != nil {
		c.aaaRespSub.Unsubscribe()
		c.aaaRespSub = nil
	}

	c.mu.Lock()
	runners := c.runners
	c.runners = nil
	for k := range c.tunnels {
		delete(c.tunnels, k)
	}
	c.mu.Unlock()
	for _, r := range runners {
		r.Stop()
	}
	return nil
}

// LookupTunnel returns the tunnel for an incoming packet, or nil if
// no match. Lock-free hot path.
func (c *Component) LookupTunnel(peerIP net.IP, localTunnelID uint16) *Tunnel {
	k := makeTunnelKey(peerIP, localTunnelID)
	c.mu.RLock()
	t := c.tunnels[k]
	c.mu.RUnlock()
	return t
}

// LookupSession returns the session for an incoming data packet, or
// nil if no match.
func (c *Component) LookupSession(peerIP net.IP, localTunnelID, localSessionID uint16) *Session {
	t := c.LookupTunnel(peerIP, localTunnelID)
	if t == nil {
		return nil
	}
	t.mu.Lock()
	s := t.Sessions[localSessionID]
	t.mu.Unlock()
	return s
}

// findSessionByAuthRequestID returns the LNS session whose outstanding
// AAA request matches `requestID`, or nil if none. Linear scan of the
// per-tunnel session maps; AAA round-trip latency keeps the number of
// pending sessions small.
func (c *Component) findSessionByAuthRequestID(requestID string) *Session {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, t := range c.tunnels {
		for _, s := range t.snapshotSessions() {
			s.mu.Lock()
			match := s.pendingAuthRequestID == requestID
			s.mu.Unlock()
			if match {
				return s
			}
		}
	}
	return nil
}

var (
	ErrTunnelExists  = errors.New("l2tp: tunnel already exists for (peer, local-id)")
	ErrTunnelMissing = errors.New("l2tp: tunnel not found")
)

// registerTunnel publishes a freshly-built tunnel into the lookup map.
// Caller is responsible for the tunnel ID allocation and FSM wiring.
func (c *Component) registerTunnel(t *Tunnel) error {
	k := makeTunnelKey(t.PeerIP, t.LocalID)
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.tunnels[k]; exists {
		return ErrTunnelExists
	}
	c.tunnels[k] = t
	return nil
}

func (c *Component) unregisterTunnel(peerIP net.IP, localID uint16) *Tunnel {
	k := makeTunnelKey(peerIP, localID)
	c.mu.Lock()
	t := c.tunnels[k]
	delete(c.tunnels, k)
	c.mu.Unlock()
	return t
}

// allocateTunnelID returns a fresh local Tunnel-ID scoped to the given
// peer. Per RFC 2661 §3.1 IDs must be unique within (local_addr, peer)
// rather than globally; we approximate "local_addr is one BNG" today.
func (c *Component) allocateTunnelID(peerIP net.IP) (uint16, error) {
	key := peerIP.String()
	c.mu.Lock()
	alloc, ok := c.tunnelIDs[key]
	if !ok {
		alloc = NewIDAllocator()
		c.tunnelIDs[key] = alloc
	}
	c.mu.Unlock()
	return alloc.Allocate()
}

func (c *Component) releaseTunnelID(peerIP net.IP, id uint16) {
	c.mu.RLock()
	alloc := c.tunnelIDs[peerIP.String()]
	c.mu.RUnlock()
	if alloc != nil {
		alloc.Release(id)
	}
}

// installTunnelVPP registers a freshly Established tunnel in the VPP
// L2TPv2 plugin so subsequent AddL2TPSession{IP,Raw} calls can resolve
// it. Idempotent at the call sites: both LAC (handleSCCRP) and LNS
// (HandleSCCCN) drive a tunnel into Established exactly once.
func (c *Component) installTunnelVPP(t *Tunnel) error {
	if c.vpp == nil {
		return nil
	}
	t.mu.Lock()
	installed := t.installedInVPP
	t.mu.Unlock()
	if installed {
		return nil
	}
	if _, err := c.vpp.AddL2TPTunnel(
		t.LocalIP, t.PeerIP, t.LocalID, t.PeerID,
		t.LocalPort, t.PeerPort, false,
	); err != nil {
		return err
	}
	t.mu.Lock()
	t.installedInVPP = true
	t.mu.Unlock()
	return nil
}

// uninstallTunnelVPP removes a tunnel from the VPP L2TPv2 plugin. Must
// be called only after every bound session has been removed (the plugin
// returns INSTANCE_IN_USE otherwise). Idempotent.
func (c *Component) uninstallTunnelVPP(t *Tunnel) {
	if c.vpp == nil {
		return
	}
	t.mu.Lock()
	installed := t.installedInVPP
	t.installedInVPP = false
	t.mu.Unlock()
	if !installed {
		return
	}
	if err := c.vpp.DeleteL2TPTunnel(t.LocalIP, t.PeerIP, t.LocalID); err != nil {
		c.log.Warn("DeleteL2TPTunnel failed",
			"peer_ip", t.PeerIP.String(),
			"local_tunnel_id", t.LocalID,
			"error", err)
	}
}
