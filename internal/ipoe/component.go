// Copyright 2025 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ipoe

import (
	"context"
	"sync"

	"github.com/veesix-networks/osvbng/internal/ra"
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/dhcp4"
	"github.com/veesix-networks/osvbng/pkg/dhcp6"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/ha"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/opdb"
	"github.com/veesix-networks/osvbng/pkg/session"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/svcgroup"
	"github.com/veesix-networks/osvbng/pkg/vrfmgr"
)

type Component struct {
	*component.Base

	logger           *logger.Logger
	eventBus         events.Bus
	srgMgr           ha.SRGProvider
	ifMgr            *ifmgr.Manager
	cfgMgr           component.ConfigManager
	accessResolver   subscriber.AccessResolver
	vpp              southbound.Southbound
	vrfMgr           *vrfmgr.Manager
	svcGroupResolver *svcgroup.Resolver
	cache            cache.Cache
	opdb             opdb.Store
	exclusivity      session.ExclusivityRegistry
	dhcp4Providers   map[string]dhcp4.DHCPProvider
	dhcp6Providers   map[string]dhcp6.DHCPProvider
	sessions         sync.Map
	xidIndex         sync.Map
	xid6Index        sync.Map
	sessionIndex     sync.Map
	acctSessionIndex sync.Map
	usernameIndex    sync.Map
	ipv4Index        sync.Map
	ipv6Index        sync.Map

	raBuckets     map[int][]string
	raBucketMu    sync.RWMutex
	raBucketCount int
	raEngine      *ra.Engine

	dhcpChan   <-chan *dataplane.ParsedPacket
	dhcp6Chan  <-chan *dataplane.ParsedPacket
	ipv6NDChan <-chan *dataplane.ParsedPacket

	aaaRespSub   events.Subscription
	haStateSub   events.Subscription
	mutationSub  events.Subscription
	terminateSub events.Subscription

	// currentRestoreCause is set by restoreSessions before iterating opdb
	// entries and read by setupSessionRestore to populate the
	// SessionRestoredEvent. Resets to empty after the loop completes;
	// only valid while restoreSessions is in flight.
	currentRestoreCause events.RestoreCause
}

func New(deps component.Dependencies, srgMgr ha.SRGProvider, ifMgr *ifmgr.Manager, dhcp4Providers map[string]dhcp4.DHCPProvider, dhcp6Providers map[string]dhcp6.DHCPProvider) (*Component, error) {
	log := logger.Get(logger.IPoE)

	c := &Component{
		Base:             component.NewBase("ipoe"),
		logger:           log,
		eventBus:         deps.EventBus,
		srgMgr:           srgMgr,
		ifMgr:            ifMgr,
		vrfMgr:           deps.VRFManager,
		svcGroupResolver: deps.SvcGroupResolver,
		cfgMgr:           deps.ConfigManager,
		accessResolver:   deps.AccessResolver,
		vpp:              deps.Southbound,
		cache:            deps.Cache,
		opdb:             deps.OpDB,
		exclusivity:      deps.Exclusivity,
		dhcp4Providers:   dhcp4Providers,
		dhcp6Providers:   dhcp6Providers,
		dhcpChan:         deps.DHCPChan,
		dhcp6Chan:        deps.DHCPv6Chan,
		ipv6NDChan:       deps.IPv6NDChan,
		raBuckets:        make(map[int][]string),
		raEngine:         ra.NewEngine(true, log),
	}

	return c, nil
}

func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)
	c.logger.Info("Starting IPoE component")

	if cfg, err := c.cfgMgr.GetRunning(); err == nil && cfg != nil {
		c.raBucketCount = c.computeRABucketCount(cfg)
	} else {
		c.raBucketCount = raMaxBucketCount
	}

	c.SetReadyState(component.StateRestoring)

	if err := c.restoreSessions(ctx); err != nil {
		c.logger.Warn("Failed to restore sessions from OpDB", "error", err)
	}

	c.aaaRespSub = c.eventBus.Subscribe(events.TopicAAAResponseIPoE, c.handleAAAResponse)
	c.haStateSub = c.eventBus.Subscribe(events.TopicHAStateChange, c.handleHAStateChange)
	c.mutationSub = c.eventBus.Subscribe(events.TopicSubscriberMutation, c.handleSubscriberMutation)
	c.terminateSub = c.eventBus.Subscribe(events.TopicSubscriberTerminate, c.handleSubscriberTerminate)

	c.Go(c.cleanupSessions)
	c.Go(c.periodicRAEmitter)
	c.Go(c.consumeDHCPPackets)
	c.Go(c.consumeDHCPv6Packets)
	c.Go(c.consumeIPv6NDPackets)

	c.SetReadyState(component.StateReady)
	c.eventBus.Publish(events.TopicComponentReady, events.Event{
		Source: c.Name(),
		Data:   &events.ComponentReadyEvent{Component: c.Name(), State: c.ReadyState().String()},
	})

	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.logger.Info("Stopping IPoE component")

	c.SetReadyState(component.StateDraining)

	c.aaaRespSub.Unsubscribe()
	c.haStateSub.Unsubscribe()
	c.mutationSub.Unsubscribe()
	c.terminateSub.Unsubscribe()

	c.StopContext()

	return nil
}
