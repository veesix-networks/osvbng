// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"

	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/cgnat"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/opdb"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/vrfmgr"
)

const (
	// eventQueueBound caps the number of lifecycle events buffered while
	// the restore loop is running. Exceeding the bound marks the component
	// degraded rather than silently dropping the event.
	eventQueueBound = 4096
)

const opdbNamespace = "cgnat_mappings"

type Component struct {
	*component.Base

	logger    *logger.Logger
	eventBus  events.Bus
	dataplane southbound.CGNATDataplane
	opdb      opdb.Store
	cfgMgr    component.ConfigManager
	ifMgr     *ifmgr.Manager
	vrfMgr    *vrfmgr.Manager

	pools     *PoolManager
	reverse   *ReverseIndex
	bypass    *BypassManager
	blacklist *BlacklistManager

	poolIDMap      map[string]uint32
	sessionPoolMap map[string]string

	lifecycleSub  events.Subscription
	programmedSub events.Subscription
	restoredSub   events.Subscription

	sessionProvider SessionProvider

	// actMu serializes the activation-state guard. It protects both
	// sessionPoolMap and activations so the "look up existing mapping then
	// allocate" pair is atomic, removing the race where two concurrent
	// lifecycle events for the same session each allocate a fresh block.
	actMu       sync.Mutex
	activations map[string]struct{}

	// Event queue: subscribers attach BEFORE the restore loop runs and
	// queue events into pendingEvents; once restore completes drainQueue
	// processes them and sets queueDrained, after which subsequent events
	// are dispatched directly.
	queueMu       sync.Mutex
	pendingEvents []queuedEvent
	queueDrained  bool
	queueDropped  int

	restoreMu        sync.Mutex
	restoreDegraded  bool
	restoreFailedIDs []string
}

type queuedEventKind int

const (
	qLifecycle queuedEventKind = iota
	qProgrammed
	qRestored
)

type queuedEvent struct {
	kind queuedEventKind
	ev   events.Event
}

func NewComponent(deps component.Dependencies, ifMgr *ifmgr.Manager, vrfMgr *vrfmgr.Manager, sessionProvider SessionProvider) (*Component, error) {
	c := &Component{
		Base:            component.NewBase("cgnat"),
		logger:          logger.Get(logger.CGNAT),
		eventBus:        deps.EventBus,
		dataplane:       deps.Southbound,
		opdb:            deps.OpDB,
		cfgMgr:          deps.ConfigManager,
		ifMgr:           ifMgr,
		vrfMgr:          vrfMgr,
		pools:           NewPoolManager(),
		reverse:         NewReverseIndex(),
		bypass:          NewBypassManager(),
		blacklist:       NewBlacklistManager(),
		poolIDMap:       make(map[string]uint32),
		sessionPoolMap:  make(map[string]string),
		sessionProvider: sessionProvider,
		activations:     make(map[string]struct{}),
	}

	return c, nil
}

func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)
	c.logger.Info("Starting CGNAT component")

	cfg, err := c.cfgMgr.GetRunning()
	if err != nil {
		return fmt.Errorf("get running config: %w", err)
	}

	if cfg.CGNAT == nil {
		c.logger.Info("No CGNAT configuration, component idle")
		return nil
	}

	c.SetReadyState(component.StateRestoring)

	if err := c.reconcile(ctx, cfg); err != nil {
		return fmt.Errorf("reconcile: %w", err)
	}

	if err := c.setupOutsideInterfaces(cfg); err != nil {
		c.logger.Warn("Failed to setup outside interfaces", "error", err)
	}

	// Subscribe BEFORE the restore loop so live activation events that
	// arrive during restore are queued, not dropped by the no-replay bus.
	c.lifecycleSub = c.eventBus.Subscribe(events.TopicSessionLifecycle, c.handleSessionLifecycle)
	c.programmedSub = c.eventBus.Subscribe(events.TopicSessionProgrammed, c.handleSessionProgrammed)
	c.restoredSub = c.eventBus.Subscribe(events.TopicSessionRestored, c.handleSessionRestored)

	if err := c.restoreFromOpDB(ctx); err != nil {
		c.logger.Warn("Failed to restore CGNAT state from OpDB", "error", err)
	}

	c.drainQueue()

	if c.restoreDegraded {
		c.logger.Error("CGNAT entered degraded restore state",
			"failed_session_ids", c.snapshotFailedIDs(),
			"queue_dropped", c.queueDropped)
	}
	c.SetReadyState(component.StateReady)
	c.eventBus.Publish(events.TopicComponentReady, events.Event{
		Source: c.Name(),
		Data:   &events.ComponentReadyEvent{Component: c.Name(), State: c.ReadyState().String()},
	})

	c.logger.Info("CGNAT component started", "pools", len(cfg.CGNAT.Pools))
	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.logger.Info("Stopping CGNAT component")
	c.SetReadyState(component.StateDraining)
	if c.lifecycleSub != nil {
		c.lifecycleSub.Unsubscribe()
	}
	if c.programmedSub != nil {
		c.programmedSub.Unsubscribe()
	}
	if c.restoredSub != nil {
		c.restoredSub.Unsubscribe()
	}
	c.StopContext()
	return nil
}

func (c *Component) setupOutsideInterfaces(cfg *config.Config) error {
	if cfg.CGNAT == nil || len(cfg.CGNAT.Pools) == 0 {
		return nil
	}

	for poolName, pool := range cfg.CGNAT.Pools {
		if pool == nil {
			continue
		}

		vrfTableID, err := c.resolveOutsideVRF(poolName, pool.OutsideInterfaces)
		if err != nil {
			return err
		}

		if err := c.rejectOutsideSubscriberOverlap(cfg, poolName, pool.OutsideInterfaces); err != nil {
			return err
		}

		if err := c.rejectLocalAddressOverlap(poolName, pool); err != nil {
			return err
		}

		poolID, ok := c.poolIDMap[poolName]
		if !ok {
			return fmt.Errorf("cgnat: pool %q: not registered in poolIDMap (configurePools must run first)", poolName)
		}
		if err := c.dataplane.CGNATSetOutsideVRF(poolID, vrfTableID); err != nil {
			return fmt.Errorf("cgnat: pool %q: set outside VRF: %w", poolName, err)
		}
		c.logger.Info("Outside VRF configured",
			"pool", poolName,
			"interfaces", pool.OutsideInterfaces,
			"vrf_table_id", vrfTableID)
	}

	return nil
}

func (c *Component) resolveOutsideVRF(poolName string, names []string) (uint32, error) {
	var (
		vrfTableID uint32
		anchorName string
		first      = true
	)
	for _, name := range names {
		swIfIndex, ok := c.ifMgr.GetSwIfIndex(name)
		if !ok {
			return 0, fmt.Errorf("cgnat: pool %q: outside interface %q not found in dataplane", poolName, name)
		}
		iface := c.ifMgr.Get(swIfIndex)
		var tableID uint32
		if iface != nil {
			tableID = iface.FIBTableID
		}
		if first {
			vrfTableID = tableID
			anchorName = name
			first = false
			continue
		}
		if tableID != vrfTableID {
			return 0, fmt.Errorf("cgnat: pool %q: outside_interfaces span multiple VRFs (got table %d for %q, table %d for %q); a pool's outside interfaces must share one VRF for ECMP",
				poolName, vrfTableID, anchorName, tableID, name)
		}
	}
	return vrfTableID, nil
}

func (c *Component) rejectOutsideSubscriberOverlap(cfg *config.Config, poolName string, outsideNames []string) error {
	if cfg.SubscriberGroups == nil {
		return nil
	}
	outsideSet := make(map[string]struct{}, len(outsideNames))
	for _, n := range outsideNames {
		outsideSet[n] = struct{}{}
	}
	for groupName, group := range cfg.SubscriberGroups.Groups {
		if group == nil {
			continue
		}
		for _, vr := range group.VLANs {
			if vr.ParentInterface == "" {
				continue
			}
			if _, hit := outsideSet[vr.ParentInterface]; hit {
				return fmt.Errorf("cgnat: pool %q: outside interface %q is also a subscriber access interface (parent of subscriber-group %q); subscriber and outside roles must not overlap",
					poolName, vr.ParentInterface, groupName)
			}
		}
	}
	return nil
}

func (c *Component) rejectLocalAddressOverlap(poolName string, pool *cgnat.Pool) error {
	var prefixes []*net.IPNet
	for _, addrStr := range pool.OutsideAddresses {
		_, prefix, err := net.ParseCIDR(addrStr)
		if err != nil {
			ip := net.ParseIP(addrStr)
			if ip == nil {
				continue
			}
			prefix = &net.IPNet{IP: ip.To4(), Mask: net.CIDRMask(32, 32)}
		}
		prefixes = append(prefixes, prefix)
	}
	if len(prefixes) == 0 {
		return nil
	}
	for _, iface := range c.ifMgr.List() {
		if iface == nil {
			continue
		}
		for _, addr := range iface.IPv4Addresses {
			if addr == nil {
				continue
			}
			v4 := addr.To4()
			if v4 == nil {
				continue
			}
			for _, prefix := range prefixes {
				if prefix.Contains(v4) {
					return fmt.Errorf("cgnat: pool %q: outside prefix %s overlaps with locally-owned address %s on %s; the DPO redirect would catch the BNG's own control-plane reply traffic and break it",
						poolName, prefix.String(), v4.String(), iface.Name)
				}
			}
		}
	}
	return nil
}

func (c *Component) handleSessionLifecycle(event events.Event) {
	if c.maybeEnqueue(qLifecycle, event) {
		return
	}
	c.dispatchLifecycle(event)
}

func (c *Component) dispatchLifecycle(event events.Event) {
	data, ok := event.Data.(*events.SessionLifecycleEvent)
	if !ok {
		return
	}

	switch data.State {
	case models.SessionStateActive:
		if data.AccessType == models.AccessTypePPPoE || data.AccessType == models.AccessTypeIPoE {
			return
		}
		if data.Protocol == models.ProtocolDHCPv6 {
			return
		}
		c.handleSessionActivate(data)
	case models.SessionStateReleased:
		c.handleSessionRelease(data)
	}
}

func (c *Component) handleSessionProgrammed(event events.Event) {
	if c.maybeEnqueue(qProgrammed, event) {
		return
	}
	c.dispatchProgrammed(event)
}

func (c *Component) dispatchProgrammed(event events.Event) {
	data, ok := event.Data.(*events.SessionLifecycleEvent)
	if !ok {
		return
	}
	c.handleSessionActivate(data)
}

// handleSessionRestored installs / re-installs the CGNAT mapping for a
// session whose state was replayed from opdb by setupSession on the
// IPoE / PPPoE side. The CGNAT plugin add_mapping API is idempotent under
// the three-state contract, so the same code
// path that handles fresh TopicSessionProgrammed handles restore safely
// here. Splitting into a dedicated handler keeps the option of branching
// on RestoreCause later (e.g. distinct counters for opdb-restore vs
// VPP-recovery) without changing the wiring.
func (c *Component) handleSessionRestored(event events.Event) {
	if c.maybeEnqueue(qRestored, event) {
		return
	}
	c.dispatchRestored(event)
}

func (c *Component) dispatchRestored(event events.Event) {
	data, ok := event.Data.(*events.SessionRestoredEvent)
	if !ok {
		return
	}
	c.handleSessionActivate(&events.SessionLifecycleEvent{
		AccessType: data.AccessType,
		Protocol:   data.Protocol,
		SessionID:  data.SessionID,
		State:      models.SessionStateActive,
		Session:    data.Session,
	})
}

func (c *Component) handleSessionActivate(data *events.SessionLifecycleEvent) {
	proceed, done := c.beginActivation(data.SessionID)
	if !proceed {
		return
	}
	// done() is called from the leaf paths (sync) or from the PBA async
	// callback (success and failure both).
	c.handleSessionActivateLocked(data, done)
}

func (c *Component) handleSessionActivateLocked(data *events.SessionLifecycleEvent, done func()) {
	var insideIP net.IP
	var vrfName string
	var swIfIndex uint32
	var serviceGroup string
	var srgName string

	switch data.AccessType {
	case models.AccessTypeIPoE:
		sess, ok := data.Session.(*models.IPoESession)
		if !ok {
			done()
			return
		}
		insideIP = sess.IPv4Address
		vrfName = sess.VRF
		swIfIndex = sess.IfIndex
		serviceGroup = sess.ServiceGroup
		srgName = sess.SRGName
	case models.AccessTypePPPoE:
		sess, ok := data.Session.(*models.PPPSession)
		if !ok {
			done()
			return
		}
		insideIP = sess.IPv4Address
		vrfName = sess.VRF
		swIfIndex = sess.IfIndex
		serviceGroup = sess.ServiceGroup
		srgName = sess.SRGName
	default:
		done()
		return
	}

	if insideIP == nil || insideIP.To4() == nil {
		done()
		return
	}

	cls := c.classifySession(serviceGroup, insideIP, vrfName)
	switch cls.kind {
	case classBypass:
		c.handleBypass(insideIP, vrfName)
		done()
	case classDeterministic:
		c.handleDetActivate(cls.poolName, swIfIndex)
		done()
	case classPBA:
		c.handlePBAActivate(cls.poolName, insideIP, vrfName, swIfIndex, data.SessionID, srgName, done)
	default:
		done()
	}
}

type classificationKind int

const (
	classNone classificationKind = iota
	classBypass
	classDeterministic
	classPBA
)

type classification struct {
	kind     classificationKind
	poolName string
}

// classifySession encapsulates the PBA / deterministic / bypass decision used
// by both the live event path and the post-restore non-PBA scan. Keeping this
// in one place means recovery and steady-state agree on what to install.
func (c *Component) classifySession(serviceGroup string, insideIP net.IP, vrfName string) classification {
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil || cfg.CGNAT == nil {
		return classification{kind: classNone}
	}

	if serviceGroup != "" && cfg.ServiceGroups != nil {
		if sg, ok := cfg.ServiceGroups[serviceGroup]; ok && sg.CGNAT != nil {
			if sg.CGNAT.Bypass {
				return classification{kind: classBypass}
			}
			if sg.CGNAT.Policy != "" {
				if _, ok := cfg.CGNAT.Pools[sg.CGNAT.Policy]; ok {
					return classification{kind: classPBA, poolName: sg.CGNAT.Policy}
				}
			}
		}
	}

	poolName := c.pools.FindPoolForIP(insideIP, vrfName)
	if poolName == "" {
		return classification{kind: classNone}
	}
	pool := cfg.CGNAT.Pools[poolName]
	if pool == nil {
		return classification{kind: classNone}
	}
	if pool.GetMode() == "deterministic" {
		return classification{kind: classDeterministic, poolName: poolName}
	}
	return classification{kind: classPBA, poolName: poolName}
}

func (c *Component) handlePBAActivate(poolName string, insideIP net.IP, vrfName string, swIfIndex uint32, sessionID string, srgName string, done func()) {
	if synced := c.tryRestoreSyncedMapping(sessionID, swIfIndex, poolName, srgName, done); synced {
		return
	}

	mapping, isNew, err := c.pools.GetOrAllocate(poolName, insideIP, 0, swIfIndex)
	if err != nil {
		c.logger.Error("CGNAT block allocation failed", "pool", poolName, "ip", insideIP, "error", err)
		done()
		return
	}

	mapping.SessionID = sessionID
	poolID := c.poolIDMap[poolName]

	if !isNew {
		c.commitMapping(sessionID, poolName, mapping, srgName, false)
		done()
		return
	}

	c.dataplane.CGNATAddDelSubscriberMappingAsync(poolID, swIfIndex, insideIP,
		0, mapping.OutsideIP, mapping.PortBlockStart, mapping.PortBlockEnd,
		true, true, func(err error) {
			if err != nil {
				c.logger.Error("subscriber mapping failed, rolling back", "error", err)
				c.pools.ReleaseBlocks(poolName, insideIP, 0)
				done()
				return
			}

			c.commitMapping(sessionID, poolName, mapping, srgName, true)
			done()

			c.logger.Debug("CGNAT PBA mapping created",
				"session", sessionID,
				"inside", insideIP,
				"outside", fmt.Sprintf("%s:%d-%d", mapping.OutsideIP, mapping.PortBlockStart, mapping.PortBlockEnd),
				"pool", poolName)
		})
}

// commitMapping serializes the post-success state updates that the old async
// callback used to do inline. Takes actMu so the activation guard sees a
// consistent sessionPoolMap. publishMappingEvent is left outside the lock
// because the bus dispatcher may take its own locks.
func (c *Component) commitMapping(sessionID, poolName string, mapping *models.CGNATMapping, srgName string, persist bool) {
	c.actMu.Lock()
	c.sessionPoolMap[sessionID] = poolName
	c.actMu.Unlock()

	c.reverse.Add(mapping)

	if persist && c.opdb != nil {
		if data, err := json.Marshal(mapping); err == nil {
			c.opdb.Put(context.Background(), opdbNamespace, sessionID, data)
		}
	}

	c.publishMappingEvent(srgName, mapping, true)
}

func (c *Component) tryRestoreSyncedMapping(sessionID string, swIfIndex uint32, poolName string, srgName string, done func()) bool {
	if c.opdb == nil {
		return false
	}

	var found []byte
	c.opdb.Load(context.Background(), opdb.NamespaceHASyncedCGNAT, func(key string, value []byte) error {
		if key == sessionID {
			found = make([]byte, len(value))
			copy(found, value)
		}
		return nil
	})

	if found == nil {
		return false
	}

	var mapping models.CGNATMapping
	if err := json.Unmarshal(found, &mapping); err != nil {
		return false
	}

	mapping.SwIfIndex = swIfIndex
	mapping.SessionID = sessionID

	if err := c.pools.RestoreMappingIfAbsent(&mapping); err != nil {
		c.logger.Warn("Failed to restore synced CGNAT mapping", "session", sessionID, "error", err)
		return false
	}

	poolID := c.poolIDMap[poolName]

	c.dataplane.CGNATAddDelSubscriberMappingAsync(poolID, swIfIndex, mapping.InsideIP,
		0, mapping.OutsideIP, mapping.PortBlockStart, mapping.PortBlockEnd,
		true, true, func(err error) {
			if err != nil {
				c.logger.Error("restore synced mapping failed", "session", sessionID, "error", err)
				c.pools.ReleaseBlocks(poolName, mapping.InsideIP, 0)
				done()
				return
			}

			c.commitMapping(sessionID, poolName, &mapping, srgName, true)

			if c.opdb != nil {
				c.opdb.Delete(context.Background(), opdb.NamespaceHASyncedCGNAT, sessionID)
			}

			done()

			c.logger.Info("CGNAT mapping restored from HA sync",
				"session", sessionID,
				"inside", mapping.InsideIP,
				"outside_ip", mapping.OutsideIP,
				"ports", fmt.Sprintf("%d-%d", mapping.PortBlockStart, mapping.PortBlockEnd))
		})

	return true
}

func (c *Component) handleDetActivate(poolName string, swIfIndex uint32) {
	poolID, ok := c.poolIDMap[poolName]
	if !ok {
		return
	}

	if err := c.dataplane.CGNATEnableOnSession(poolID, swIfIndex, true); err != nil {
		c.logger.Error("enable CGNAT on session failed", "pool", poolName, "sw_if", swIfIndex, "error", err)
	}
}

func (c *Component) handleBypass(insideIP net.IP, vrfName string) {
	prefix := net.IPNet{IP: insideIP.To4(), Mask: net.CIDRMask(32, 32)}
	if err := c.dataplane.CGNATAddDelBypass(prefix, 0, true); err != nil {
		c.logger.Error("bypass programming failed", "ip", insideIP, "error", err)
		return
	}
	c.bypass.AddIP(insideIP, 0)
}

func (c *Component) handleSessionRelease(data *events.SessionLifecycleEvent) {
	c.logger.Debug("CGNAT handleSessionRelease called",
		"session", data.SessionID,
		"access_type", data.AccessType,
		"protocol", data.Protocol)

	var insideIP net.IP
	var swIfIndex uint32
	var serviceGroup string
	var srgName string

	switch data.AccessType {
	case models.AccessTypeIPoE:
		sess, ok := data.Session.(*models.IPoESession)
		if !ok {
			return
		}
		insideIP = sess.IPv4Address
		swIfIndex = sess.IfIndex
		serviceGroup = sess.ServiceGroup
		srgName = sess.SRGName
	case models.AccessTypePPPoE:
		sess, ok := data.Session.(*models.PPPSession)
		if !ok {
			return
		}
		insideIP = sess.IPv4Address
		swIfIndex = sess.IfIndex
		serviceGroup = sess.ServiceGroup
		srgName = sess.SRGName
	default:
		return
	}

	if insideIP == nil || insideIP.To4() == nil {
		return
	}

	cfg, _ := c.cfgMgr.GetRunning()
	if cfg != nil && cfg.CGNAT != nil && serviceGroup != "" && cfg.ServiceGroups != nil {
		if sg, ok := cfg.ServiceGroups[serviceGroup]; ok && sg.CGNAT != nil && sg.CGNAT.Bypass {
			prefix := net.IPNet{IP: insideIP.To4(), Mask: net.CIDRMask(32, 32)}
			c.dataplane.CGNATAddDelBypass(prefix, 0, false)
			c.bypass.RemovePrefix(&prefix, 0)
			return
		}
	}

	c.actMu.Lock()
	poolName, ok := c.sessionPoolMap[data.SessionID]
	if !ok {
		mapSize := len(c.sessionPoolMap)
		c.actMu.Unlock()
		c.logger.Debug("CGNAT release: no pool mapping for session",
			"session", data.SessionID,
			"inside_ip", insideIP,
			"map_size", mapSize)
		if err := c.dataplane.CGNATEnableOnSession(0, swIfIndex, false); err != nil {
			c.logger.Debug("Disable CGNAT on session", "sw_if", swIfIndex, "error", err)
		}
		return
	}
	c.actMu.Unlock()

	mappings := c.pools.GetMappings(poolName, insideIP, 0)
	c.logger.Debug("CGNAT release: found mappings",
		"session", data.SessionID,
		"pool", poolName,
		"inside_ip", insideIP,
		"mapping_count", len(mappings))
	if len(mappings) == 0 {
		c.actMu.Lock()
		delete(c.sessionPoolMap, data.SessionID)
		c.actMu.Unlock()
		return
	}

	poolID := c.poolIDMap[poolName]

	for i := range mappings {
		mapping := &mappings[i]
		c.dataplane.CGNATAddDelSubscriberMappingAsync(poolID, swIfIndex, insideIP,
			0, mapping.OutsideIP, mapping.PortBlockStart, mapping.PortBlockEnd,
			false, false, func(err error) {
				if err != nil {
					c.logger.Error("remove mapping failed", "error", err)
					return
				}
				c.publishMappingEvent(srgName, mapping, false)
			})
		c.reverse.Remove(mapping.OutsideIP, mapping.PortBlockStart)
	}

	c.pools.ReleaseBlocks(poolName, insideIP, 0)
	c.actMu.Lock()
	delete(c.sessionPoolMap, data.SessionID)
	c.actMu.Unlock()

	if c.opdb != nil {
		c.opdb.Delete(context.Background(), opdbNamespace, data.SessionID)
	}

	c.logger.Debug("CGNAT mappings released", "session", data.SessionID, "inside", insideIP, "blocks", len(mappings))
}

// beginActivation atomically reserves the (sessionID) activation slot. Returns
// (false, no-op) when a committed mapping already exists for the session or
// another activation is in flight — both cases are correctly idempotent
// no-ops. Callers MUST invoke done() exactly once on the proceed path; for
// async southbound callbacks that means the callback owns done().
func (c *Component) beginActivation(sessionID string) (bool, func()) {
	c.actMu.Lock()
	defer c.actMu.Unlock()

	if _, ok := c.sessionPoolMap[sessionID]; ok {
		return false, func() {}
	}
	if _, ok := c.activations[sessionID]; ok {
		return false, func() {}
	}
	c.activations[sessionID] = struct{}{}

	return true, func() {
		c.actMu.Lock()
		delete(c.activations, sessionID)
		c.actMu.Unlock()
	}
}

// maybeEnqueue buffers an event during the restore window. Returns true when
// the event was queued (caller does not dispatch directly) or dropped past
// the bound (caller still does not dispatch — the dropped event flips the
// component into the degraded state). Returns false once the queue has been
// drained, at which point the caller dispatches the event itself.
func (c *Component) maybeEnqueue(kind queuedEventKind, ev events.Event) bool {
	c.queueMu.Lock()
	defer c.queueMu.Unlock()
	if c.queueDrained {
		return false
	}
	if len(c.pendingEvents) >= eventQueueBound {
		c.queueDropped++
		c.markRestoreDegradedLocked("event_queue_overflow")
		c.logger.Warn("CGNAT event queue full; dropping event",
			"dropped_total", c.queueDropped, "bound", eventQueueBound)
		return true
	}
	c.pendingEvents = append(c.pendingEvents, queuedEvent{kind: kind, ev: ev})
	return true
}

func (c *Component) drainQueue() {
	c.queueMu.Lock()
	queued := c.pendingEvents
	c.pendingEvents = nil
	c.queueDrained = true
	c.queueMu.Unlock()

	for _, q := range queued {
		switch q.kind {
		case qLifecycle:
			c.dispatchLifecycle(q.ev)
		case qProgrammed:
			c.dispatchProgrammed(q.ev)
		case qRestored:
			c.dispatchRestored(q.ev)
		}
	}
}

func (c *Component) markRestoreDegraded(sessionID string) {
	c.restoreMu.Lock()
	defer c.restoreMu.Unlock()
	c.restoreDegraded = true
	if sessionID != "" {
		c.restoreFailedIDs = append(c.restoreFailedIDs, sessionID)
	}
}

func (c *Component) markRestoreDegradedLocked(reason string) {
	// Caller holds queueMu; restoreMu is independent.
	c.restoreMu.Lock()
	defer c.restoreMu.Unlock()
	c.restoreDegraded = true
	if reason != "" {
		c.restoreFailedIDs = append(c.restoreFailedIDs, reason)
	}
}

func (c *Component) snapshotFailedIDs() []string {
	c.restoreMu.Lock()
	defer c.restoreMu.Unlock()
	out := make([]string, len(c.restoreFailedIDs))
	copy(out, c.restoreFailedIDs)
	return out
}

// RestoreDegraded reports whether the most recent restore (cold or
// watchdog) failed to reprogram at least one mapping into VPP or dropped
// at least one event past the bounded queue. Surfaced for ops alarming
// and tests; the component still reaches StateReady because session
// teardown / live-event handling continues to work.
func (c *Component) RestoreDegraded() bool {
	c.restoreMu.Lock()
	defer c.restoreMu.Unlock()
	return c.restoreDegraded
}

// restoreFromOpDB is the cold-restart recovery entry point. Three phases:
//
//  1. PBA loop — replay cgnat_mappings into VPP with the fresh sw_if_index.
//     Programs VPP first, commits local state only on success, preserves the
//     opdb entry for retry on failure. Marks orphans (session expired in the
//     access opdb) for deletion; preserves mappings whose access opdb entry
//     still exists (retained for retry).
//
//  2. Non-PBA scan — iterate the subscriber session cache and classify any
//     session not already in sessionPoolMap. This is the source of truth for
//     deterministic and bypass recovery (neither writes cgnat_mappings) and
//     also catches genuinely-new sessions that established during the
//     restore window (their queued event is drained later but the scan
//     covers the case where the event was dropped past the queue bound).
//
//  3. Orphan delete — for each PBA mapping whose access session is
//     authoritatively gone, delete cgnat_mappings and release the block.
//
// RecoverDataplane is invoked by the VPP watchdog after a successful
// reconnect + bootstrap. Local CGNAT state is intact (osvbngd never went
// down) but the dataplane lost the plugin's mapping table, so we reprogram
// every known mapping into VPP without rebuilding the PoolManager /
// reverse / sessionPoolMap state that is already correct.
//
// IPoE/PPPoE call their own RecoverSessions BEFORE this hook (see
// cmd/osvbngd/main.go OnRecover), so the subscriber cache holds the live
// sw_if_index for every session by the time we run.
func (c *Component) RecoverDataplane(ctx context.Context) error {
	if c.opdb == nil {
		return nil
	}
	if c.sessionProvider == nil {
		c.logger.Warn("CGNAT recover: no session provider wired; skipping dataplane reprogram")
		return nil
	}

	// Reset the degraded flag for this recovery cycle — a previous cold
	// restart's failures should not mask a clean watchdog recovery.
	c.restoreMu.Lock()
	c.restoreDegraded = false
	c.restoreFailedIDs = nil
	c.restoreMu.Unlock()

	var (
		toProgram = map[uint32][]southbound.CGNATMapping{}
		ctxByPool = map[uint32][]restoreCtx{}
		failed    []string
	)

	err := c.opdb.Load(ctx, opdbNamespace, func(key string, value []byte) error {
		var mapping models.CGNATMapping
		if err := json.Unmarshal(value, &mapping); err != nil {
			return nil
		}
		mapping.SessionID = key

		liveSwIfIndex, present := resolveIfIndex(ctx, c.sessionProvider, key)
		if !present {
			// Session is gone in this recovery cycle. The cold-restart
			// path owns orphan cleanup; here we just skip and let the
			// next restart pick it up.
			return nil
		}
		mapping.SwIfIndex = liveSwIfIndex

		poolID, ok := c.poolIDMap[mapping.PoolName]
		if !ok {
			return nil
		}

		toProgram[poolID] = append(toProgram[poolID], southbound.CGNATMapping{
			PoolID:         poolID,
			SwIfIndex:      liveSwIfIndex,
			InsideIP:       mapping.InsideIP,
			InsideVRFID:    mapping.InsideVRFID,
			OutsideIP:      mapping.OutsideIP,
			PortBlockStart: mapping.PortBlockStart,
			PortBlockEnd:   mapping.PortBlockEnd,
			EnableFeature:  true,
		})
		ctxByPool[poolID] = append(ctxByPool[poolID], restoreCtx{sessionID: key, mapping: mapping})
		return nil
	})
	if err != nil {
		return err
	}

	for poolID, batch := range toProgram {
		results, callErr := c.dataplane.CGNATAddSubscriberMappingBulk(poolID, batch)
		if callErr != nil {
			for _, rc := range ctxByPool[poolID] {
				c.markRestoreDegraded(rc.sessionID)
				failed = append(failed, rc.sessionID)
			}
			continue
		}
		for i, perErr := range results {
			rc := ctxByPool[poolID][i]
			if perErr != nil {
				c.markRestoreDegraded(rc.sessionID)
				failed = append(failed, rc.sessionID)
				continue
			}
			// Refresh sw_if_index in opdb only — local state is
			// authoritative and untouched.
			if data, err := json.Marshal(&rc.mapping); err == nil {
				c.opdb.Put(ctx, opdbNamespace, rc.sessionID, data)
			}
		}
	}

	// Deterministic and bypass need their feature re-enabled per session
	// since the plugin lost that state. The non-PBA scan handles it via
	// the same classification helper, idempotent against existing local
	// CGNAT state because handleDetActivate / handleBypass program by
	// (poolID, sw_if_index) / (insideIP) — VPP's add APIs treat the
	// repeat as a no-op.
	c.scanNonPBASessionsForRecover(ctx)

	c.logger.Info("CGNAT watchdog dataplane recovery complete",
		"pba_reprogrammed", countRestored(toProgram, failed),
		"failed", len(failed))

	return nil
}

func (c *Component) scanNonPBASessionsForRecover(ctx context.Context) {
	sessions, err := c.sessionProvider.GetSessions(ctx, "", "", 0)
	if err != nil {
		c.logger.Warn("CGNAT recover: session scan failed", "error", err)
		return
	}
	for _, sess := range sessions {
		insideIP := sess.GetIPv4Address()
		if insideIP == nil || insideIP.To4() == nil {
			continue
		}
		vrfName := ""
		switch s := sess.(type) {
		case *models.IPoESession:
			vrfName = s.VRF
		case *models.PPPSession:
			vrfName = s.VRF
		}
		cls := c.classifySession(sess.GetServiceGroup(), insideIP, vrfName)
		switch cls.kind {
		case classBypass:
			c.handleBypass(insideIP, vrfName)
		case classDeterministic:
			c.handleDetActivate(cls.poolName, sess.GetIfIndex())
		}
	}
}

func (c *Component) restoreFromOpDB(ctx context.Context) error {
	if c.opdb == nil {
		return nil
	}
	if c.sessionProvider == nil {
		c.logger.Warn("CGNAT restore: no session provider wired; skipping (deterministic / bypass / sw_if_index refresh will be missed)")
		return nil
	}

	ipoeKnown, pppoeKnown, err := c.loadAccessSessionKeys(ctx)
	if err != nil {
		c.logger.Warn("CGNAT restore: failed to preload access opdb namespaces; orphan detection will be conservative", "error", err)
	}

	var (
		toProgram     = map[uint32][]southbound.CGNATMapping{}
		ctxByPool     = map[uint32][]restoreCtx{}
		toOrphan      []restoreCtx
		alreadyKnown  = map[string]struct{}{}
		restoreErrors []string
	)

	err = c.opdb.Load(ctx, opdbNamespace, func(key string, value []byte) error {
		var mapping models.CGNATMapping
		if err := json.Unmarshal(value, &mapping); err != nil {
			c.logger.Warn("CGNAT restore: unmarshal mapping", "key", key, "error", err)
			return nil
		}
		mapping.SessionID = key

		liveSwIfIndex, present := resolveIfIndex(ctx, c.sessionProvider, key)
		if !present {
			if hasAccessRecord(key, ipoeKnown, pppoeKnown) {
				c.logger.Warn("CGNAT restore: subscriber cache miss but access opdb entry retained; preserving mapping for retry",
					"session", key, "inside", mapping.InsideIP)
				if rerr := c.pools.RestoreMappingIfAbsent(&mapping); rerr == nil {
					c.reverse.Add(&mapping)
				}
				c.markRestoreDegraded(key)
				restoreErrors = append(restoreErrors, key)
				return nil
			}
			c.logger.Info("CGNAT restore: session authoritatively gone; orphan cleanup",
				"session", key, "inside", mapping.InsideIP)
			toOrphan = append(toOrphan, restoreCtx{sessionID: key, mapping: mapping})
			return nil
		}

		mapping.SwIfIndex = liveSwIfIndex

		poolID, ok := c.poolIDMap[mapping.PoolName]
		if !ok {
			c.logger.Warn("CGNAT restore: pool no longer configured; orphan cleanup",
				"session", key, "pool", mapping.PoolName)
			toOrphan = append(toOrphan, restoreCtx{sessionID: key, mapping: mapping})
			return nil
		}

		toProgram[poolID] = append(toProgram[poolID], southbound.CGNATMapping{
			PoolID:         poolID,
			SwIfIndex:      liveSwIfIndex,
			InsideIP:       mapping.InsideIP,
			InsideVRFID:    mapping.InsideVRFID,
			OutsideIP:      mapping.OutsideIP,
			PortBlockStart: mapping.PortBlockStart,
			PortBlockEnd:   mapping.PortBlockEnd,
			EnableFeature:  true,
		})
		ctxByPool[poolID] = append(ctxByPool[poolID], restoreCtx{sessionID: key, mapping: mapping})
		return nil
	})
	if err != nil {
		return err
	}

	for poolID, batch := range toProgram {
		results, callErr := c.dataplane.CGNATAddSubscriberMappingBulk(poolID, batch)
		if callErr != nil {
			c.logger.Error("CGNAT restore: bulk binapi failed at transport level; preserving all entries for retry",
				"pool_id", poolID, "count", len(batch), "error", callErr)
			for _, rc := range ctxByPool[poolID] {
				c.markRestoreDegraded(rc.sessionID)
				restoreErrors = append(restoreErrors, rc.sessionID)
			}
			continue
		}
		for i, perErr := range results {
			rc := ctxByPool[poolID][i]
			if perErr != nil {
				c.logger.Error("CGNAT restore: mapping reprogram failed; preserving opdb entry for retry",
					"session", rc.sessionID, "inside", rc.mapping.InsideIP, "error", perErr)
				c.markRestoreDegraded(rc.sessionID)
				restoreErrors = append(restoreErrors, rc.sessionID)
				continue
			}
			c.commitRestoredPBA(ctx, &rc.mapping)
			alreadyKnown[rc.sessionID] = struct{}{}
		}
	}

	c.scanNonPBASessions(ctx, alreadyKnown)

	for _, rc := range toOrphan {
		c.deleteOrphan(ctx, &rc.mapping)
	}

	c.logger.Info("CGNAT restore complete",
		"pba_reprogrammed", countRestored(toProgram, restoreErrors),
		"failed", len(restoreErrors),
		"orphan_cleaned", len(toOrphan))

	return nil
}

type restoreCtx struct {
	sessionID string
	mapping   models.CGNATMapping
}

func resolveIfIndex(ctx context.Context, sp SessionProvider, sessionID string) (uint32, bool) {
	snap, ok := sp.SessionSnapshot(ctx, sessionID)
	if !ok {
		return 0, false
	}
	idx := snap.GetIfIndex()
	if idx == 0 {
		return 0, false
	}
	return idx, true
}

func hasAccessRecord(sessionID string, ipoeKnown, pppoeKnown map[string]struct{}) bool {
	if _, ok := ipoeKnown[sessionID]; ok {
		return true
	}
	_, ok := pppoeKnown[sessionID]
	return ok
}

func (c *Component) loadAccessSessionKeys(ctx context.Context) (map[string]struct{}, map[string]struct{}, error) {
	ipoe := map[string]struct{}{}
	pppoe := map[string]struct{}{}
	if err := c.opdb.Load(ctx, opdb.NamespaceIPoESessions, func(key string, _ []byte) error {
		ipoe[key] = struct{}{}
		return nil
	}); err != nil {
		return ipoe, pppoe, err
	}
	if err := c.opdb.Load(ctx, opdb.NamespacePPPoESessions, func(key string, _ []byte) error {
		pppoe[key] = struct{}{}
		return nil
	}); err != nil {
		return ipoe, pppoe, err
	}
	return ipoe, pppoe, nil
}

func (c *Component) commitRestoredPBA(ctx context.Context, mapping *models.CGNATMapping) {
	if err := c.pools.RestoreMappingIfAbsent(mapping); err != nil {
		c.logger.Warn("CGNAT restore: local pool restore", "session", mapping.SessionID, "error", err)
	}
	c.reverse.Add(mapping)

	c.actMu.Lock()
	c.sessionPoolMap[mapping.SessionID] = mapping.PoolName
	c.actMu.Unlock()

	// Refresh the persisted entry if the sw_if_index changed (VPP renumbered
	// after a cold restart). Same intent as IPoE/PPPoE's restore checkpoint.
	if data, err := json.Marshal(mapping); err == nil {
		c.opdb.Put(ctx, opdbNamespace, mapping.SessionID, data)
	}
}

func (c *Component) deleteOrphan(ctx context.Context, mapping *models.CGNATMapping) {
	if err := c.opdb.Delete(ctx, opdbNamespace, mapping.SessionID); err != nil {
		c.logger.Warn("CGNAT restore: orphan delete failed", "session", mapping.SessionID, "error", err)
	}
}

// scanNonPBASessions re-derives deterministic and bypass activations from the
// live subscriber cache. These modes write no per-subscriber opdb record, so
// the PBA loop above cannot recover them. Also catches genuinely-new sessions
// whose lifecycle event was dropped past the event-queue bound during
// restore.
func (c *Component) scanNonPBASessions(ctx context.Context, alreadyKnown map[string]struct{}) {
	sessions, err := c.sessionProvider.GetSessions(ctx, "", "", 0)
	if err != nil {
		c.logger.Warn("CGNAT restore: subscriber session scan failed; deterministic/bypass recovery may be incomplete",
			"error", err)
		return
	}
	for _, sess := range sessions {
		sid := sess.GetSessionID()
		if sid == "" {
			continue
		}
		if _, known := alreadyKnown[sid]; known {
			continue
		}
		c.actMu.Lock()
		_, committed := c.sessionPoolMap[sid]
		c.actMu.Unlock()
		if committed {
			continue
		}

		insideIP := sess.GetIPv4Address()
		if insideIP == nil || insideIP.To4() == nil {
			continue
		}
		vrfName := ""
		switch s := sess.(type) {
		case *models.IPoESession:
			vrfName = s.VRF
		case *models.PPPSession:
			vrfName = s.VRF
		}

		cls := c.classifySession(sess.GetServiceGroup(), insideIP, vrfName)
		switch cls.kind {
		case classBypass:
			proceed, done := c.beginActivation(sid)
			if !proceed {
				continue
			}
			c.handleBypass(insideIP, vrfName)
			done()
		case classDeterministic:
			proceed, done := c.beginActivation(sid)
			if !proceed {
				continue
			}
			c.handleDetActivate(cls.poolName, sess.GetIfIndex())
			done()
		case classPBA:
			// PBA session with no cgnat_mappings record — treat as a new
			// activation. The atomic GetOrAllocate prevents racing with a
			// queued event for the same session.
			proceed, done := c.beginActivation(sid)
			if !proceed {
				continue
			}
			c.handlePBAActivate(cls.poolName, insideIP, vrfName, sess.GetIfIndex(), sid, sess.GetSRGName(), done)
		}
	}
}

func countRestored(toProgram map[uint32][]southbound.CGNATMapping, errs []string) int {
	total := 0
	for _, b := range toProgram {
		total += len(b)
	}
	return total - len(errs)
}

func (c *Component) publishMappingEvent(srgName string, mapping *models.CGNATMapping, isAdd bool) {
	c.eventBus.Publish(events.TopicCGNATMapping, events.Event{
		Source: c.Name(),
		Data: &events.CGNATMappingEvent{
			SRGName:   srgName,
			SessionID: mapping.SessionID,
			Mapping:   mapping,
			IsAdd:     isAdd,
		},
	})
}

func (c *Component) GetRunningConfig() (*cgnat.Config, error) {
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil {
		return nil, err
	}
	return cfg.CGNAT, nil
}

func (c *Component) GetPoolManager() *PoolManager {
	return c.pools
}

func (c *Component) GetReverseIndex() *ReverseIndex {
	return c.reverse
}

func (c *Component) GetBypassManager() *BypassManager {
	return c.bypass
}

func (c *Component) GetBlacklistManager() *BlacklistManager {
	return c.blacklist
}
