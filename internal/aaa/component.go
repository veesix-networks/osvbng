package aaa

import (
	"context"
	"hash/fnv"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/auth"
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/opdb"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

const (
	defaultBucketSize = 5 * time.Second
)

type AccountingSession struct {
	// mu guards the counter fields (baseline / prior-delta / last-reported)
	// against concurrent writes from the bucket processor, session-restore
	// handler, and release handler.
	mu sync.Mutex

	sessionID         string
	acctSessionID     string
	accessType        models.AccessType
	authDate          time.Time
	lastInterimUpdate time.Time
	username          string
	mac               string
	ipv4Address       string
	svlan             uint16
	cvlan             uint16
	accessIfIndex     uint32
	accessInterface   string
	attributes        map[string]string

	// swIfIndex is the dataplane session interface AAA reads VPP
	// counters from when computing the next Acct-Interim cumulative.
	// Re-resolved post-restore by handleSessionRestored to track VPP
	// re-numbering across cold-restart.
	swIfIndex uint32

	// pendingSessionConfirm flags entries that loadAcctSessions pulled
	// from opdb at Start() but that haven't yet been confirmed live by
	// a TopicSessionRestored emission from PPPoE / IPoE. Cleared by
	// handleSessionRestored. Stale entries are pruned by
	// pruneOrphanedAcctEntries past pendingConfirmDeadline.
	pendingSessionConfirm  bool
	pendingConfirmDeadline time.Time

	// Persisted accounting baseline — fully owned and mutated by AAA.
	// See AccountingCheckpoint in accounting.go for semantics.
	lastReportedInOctets   uint64
	lastReportedOutOctets  uint64
	lastReportedInPackets  uint64
	lastReportedOutPackets uint64

	currentBaselineInBytes    uint64
	currentBaselineOutBytes   uint64
	currentBaselineInPackets  uint64
	currentBaselineOutPackets uint64

	priorDeltaInBytes    uint64
	priorDeltaOutBytes   uint64
	priorDeltaInPackets  uint64
	priorDeltaOutPackets uint64
}

type Component struct {
	*component.Base

	logger       *logger.Logger
	authProvider auth.AuthProvider
	eventBus     events.Bus
	cache        cache.Cache
	opdb         opdb.Store
	showSource   component.ShowSource

	aaaReqSub    events.Subscription
	lifecycleSub events.Subscription
	restoredSub  events.Subscription

	buckets  map[int][]string
	bucketMu sync.RWMutex

	acctCache   map[string]*AccountingSession
	acctCacheMu sync.RWMutex
}

func New(deps component.Dependencies, authProvider auth.AuthProvider) (*Component, error) {
	log := logger.Get(logger.AAA)

	c := &Component{
		Base:         component.NewBase("aaa"),
		logger:       log,
		authProvider: authProvider,
		eventBus:     deps.EventBus,
		cache:        deps.Cache,
		opdb:         deps.OpDB,
		showSource:   deps.ShowSource,
		buckets:      make(map[int][]string),
		acctCache:    make(map[string]*AccountingSession),
	}

	return c, nil
}

func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)
	c.logger.Info("Starting AAA component")

	// Load persisted accounting state from AAA's own opdb namespace
	// BEFORE subscribing to session-restored events. Entries land with
	// pendingSessionConfirm=true; handleSessionRestored confirms them
	// as the corresponding sessions are replayed by PPPoE / IPoE.
	// Entries that go unconfirmed past their deadline are pruned by
	// the background goroutine started below.
	if loaded, err := c.loadAcctSessions(c.Ctx); err != nil {
		c.logger.Warn("Failed to load accounting state from opdb", "error", err)
	} else if loaded > 0 {
		c.logger.Info("Loaded persisted accounting state",
			"sessions", loaded)
	}

	c.aaaReqSub = c.eventBus.Subscribe(events.TopicAAARequest, c.handleAAARequest)
	// TopicSessionLifecycle is the ONE path that emits Accounting-Start
	// (handleSessionLifecycle Active branch). Restored sessions take the
	// separate TopicSessionRestored path which rebuilds the in-memory
	// acctCache entry from the opdb-persisted baseline WITHOUT emitting
	// Start, preserving the one-Start-per-session invariant required by
	// RFC 2866.
	c.lifecycleSub = c.eventBus.Subscribe(events.TopicSessionLifecycle, c.handleSessionLifecycle)
	c.restoredSub = c.eventBus.Subscribe(events.TopicSessionRestored, c.handleSessionRestored)

	c.BuildAccountingBuckets()
	c.Go(c.orphanPruneLoop)

	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.logger.Info("Stopping AAA component")
	c.aaaReqSub.Unsubscribe()
	c.lifecycleSub.Unsubscribe()
	if c.restoredSub != nil {
		c.restoredSub.Unsubscribe()
	}
	c.StopContext()
	return nil
}

// orphanPruneLoop periodically drops acctCache entries that were loaded
// from opdb at Start() but never confirmed by a TopicSessionRestored
// emission. Runs at half the pruneAcctOrphansAfter cadence so an entry
// always reaches the deadline before its next pruning pass evaluates it.
func (c *Component) orphanPruneLoop() {
	t := time.NewTicker(pruneAcctOrphansAfter / 2)
	defer t.Stop()
	for {
		select {
		case <-c.Ctx.Done():
			return
		case now := <-t.C:
			if pruned := c.pruneOrphanedAcctEntries(now); pruned > 0 {
				c.logger.Info("Pruned acct cache entries with no live session",
					"count", pruned)
			}
		}
	}
}

func (c *Component) BuildAccountingBuckets() {
	numBuckets := int(time.Minute / defaultBucketSize)

	for i := 0; i < numBuckets; i++ {
		bucketId := i
		c.Go(func() {
			now := time.Now()
			next := time.Date(
				now.Year(), now.Month(), now.Day(),
				now.Hour(), now.Minute(), bucketId,
				0, now.Location(),
			)

			if next.Before(now) {
				next = next.Add(1 * time.Minute)
			}

			c.logger.Debug("Bucket sleeping", "id", bucketId, "until", next, "duration", time.Until(next))
			time.Sleep(time.Until(next))

			ticker := time.NewTicker(1 * time.Minute)
			defer ticker.Stop()

			c.ProcessAccountingBucket(bucketId)

			for {
				select {
				case <-ticker.C:
					c.ProcessAccountingBucket(bucketId)
				case <-c.Ctx.Done():
					return
				}
			}
		})
	}
}

func (c *Component) ProcessAccountingBucket(bucketId int) {
	c.bucketMu.RLock()
	sessionIds := c.buckets[bucketId]
	c.bucketMu.RUnlock()

	statsByIdx := c.fetchInterfaceStats()

	c.acctCacheMu.Lock()
	defer c.acctCacheMu.Unlock()

	for _, sessionId := range sessionIds {
		session, exists := c.acctCache[sessionId]
		if !exists {
			continue
		}

		go c.sendAccountingUpdate(session, statsByIdx)
	}
}

// fetchInterfaceStats reads the cached SystemDataplaneInterfaces snapshot
// once per bucket tick and indexes by sw_if_index for per-session lookup.
// A nil map (no registry wired, snapshot error, unexpected shape) causes
// sendAccountingUpdate to fall back to the last-reported cumulative.
func (c *Component) fetchInterfaceStats() map[uint32]*southbound.InterfaceStats {
	if c.showSource == nil {
		return nil
	}
	snap, err := c.showSource.Snapshot(c.Ctx, paths.SystemDataplaneInterfaces.String())
	if err != nil {
		c.logger.Debug("Interface stats snapshot unavailable",
			"path", paths.SystemDataplaneInterfaces.String(), "error", err)
		return nil
	}
	stats, ok := snap.([]southbound.InterfaceStats)
	if !ok {
		c.logger.Debug("Interface stats snapshot unexpected shape",
			"path", paths.SystemDataplaneInterfaces.String())
		return nil
	}
	by := make(map[uint32]*southbound.InterfaceStats, len(stats))
	for i := range stats {
		by[stats[i].Index] = &stats[i]
	}
	return by
}

func (c *Component) sendAccountingUpdate(acctSession *AccountingSession, statsByIdx map[uint32]*southbound.InterfaceStats) {
	acctSession.mu.Lock()
	rxBytes, txBytes, rxPackets, txPackets := acctSession.lastReportedInOctets,
		acctSession.lastReportedOutOctets,
		acctSession.lastReportedInPackets,
		acctSession.lastReportedOutPackets
	if stats, ok := statsByIdx[acctSession.swIfIndex]; ok {
		rxBytes, txBytes, rxPackets, txPackets = acctSession.applyVPPCounters(stats)
	}
	acctSession.mu.Unlock()

	session := &auth.Session{
		SessionID:         acctSession.sessionID,
		AcctSessionID:     acctSession.acctSessionID,
		Username:          acctSession.username,
		MAC:               acctSession.mac,
		AccessType:        string(acctSession.accessType),
		AccessInterface:   acctSession.accessInterface,
		SVLAN:             acctSession.svlan,
		CVLAN:             acctSession.cvlan,
		AccessIfIndex:     acctSession.accessIfIndex,
		SubscriberIfIndex: acctSession.swIfIndex,
		RxBytes:           rxBytes,
		TxBytes:           txBytes,
		RxPackets:         rxPackets,
		TxPackets:         txPackets,
		SessionDuration:   uint32(time.Since(acctSession.authDate).Seconds()),
		Attributes:        acctSession.attributes,
	}

	if err := c.authProvider.UpdateAccounting(c.Ctx, session); err != nil {
		c.logger.Debug("Accounting update failed", "session_id", acctSession.sessionID, "error", err)
		return
	}

	acctSession.mu.Lock()
	acctSession.advanceLastReported(rxBytes, txBytes, rxPackets, txPackets)
	acctSession.mu.Unlock()
	c.checkpointAcctSession(acctSession)
}

func (c *Component) handleAAARequest(event events.Event) {
	data, ok := event.Data.(*events.AAARequestEvent)
	if !ok {
		c.logger.Error("Invalid event data for AAA request")
		return
	}

	req := data.Request
	attrs := req.Attributes
	if attrs == nil {
		attrs = make(map[string]string)
	}

	authReq := &auth.AuthRequest{
		Username:         req.Username,
		MAC:              req.MAC,
		AcctSessionID:    req.AcctSessionID,
		SVLAN:            req.SVLAN,
		CVLAN:            req.CVLAN,
		Interface:        req.Interface,
		AccessIfIndex:    req.AccessIfIndex,
		AccessInterface:  req.AccessInterface,
		AccessType:       string(data.AccessType),
		PolicyName:       req.PolicyName,
		UsernameFallback: req.UsernameFallback,
		Attributes:       attrs,
	}

	authResp, err := c.authProvider.Authenticate(c.Ctx, authReq)
	if err != nil {
		c.logger.Error("Authentication failed",
			"mac", req.MAC,
			"acct_session_id", req.AcctSessionID,
			"error", err)
		c.publishResponse(req.RequestID, data.SessionID, data.AccessType, false, nil, err)
		return
	}

	c.logger.Debug("Authentication response",
		"allowed", authResp.Allowed,
		"mac", req.MAC,
		"acct_session_id", req.AcctSessionID,
		"attributes", authResp.Attributes)

	respAttrs := make(map[string]interface{})
	for k, v := range authResp.Attributes {
		respAttrs[k] = v
	}

	c.publishResponse(req.RequestID, data.SessionID, data.AccessType, authResp.Allowed, respAttrs, nil)
}

func (c *Component) publishResponse(requestID, sessionID string, accessType models.AccessType, allowed bool, attributes map[string]interface{}, authErr error) {
	resp := models.AAAResponse{
		RequestID:  requestID,
		Allowed:    allowed,
		Attributes: attributes,
	}

	if authErr != nil {
		resp.Error = authErr.Error()
	}

	var topic string
	switch accessType {
	case models.AccessTypeIPoE:
		topic = events.TopicAAAResponseIPoE
	case models.AccessTypePPPoE:
		topic = events.TopicAAAResponsePPPoE
	case models.AccessTypeL2TP:
		topic = events.TopicAAAResponseL2TP
	default:
		topic = events.TopicAAAResponse
	}

	c.eventBus.Publish(topic, events.Event{
		Source: c.Name(),
		Data: &events.AAAResponseEvent{
			AccessType: accessType,
			SessionID:  sessionID,
			Response:   resp,
		},
	})
}

func numBuckets() int {
	return int(time.Minute / defaultBucketSize)
}

// bucketForSession returns the deterministic accounting bucket for a session
// ID. Stable across restarts and across multiple lifecycle / restore events
// for the same session, so a session fires exactly one interim per minute
// regardless of how often its lifecycle is republished.
func bucketForSession(sessionID string) int {
	h := fnv.New32a()
	h.Write([]byte(sessionID))
	return int(h.Sum32() % uint32(numBuckets()))
}

// placeSessionInBucket slots sessionID into its deterministic bucket and
// sweeps stale copies out of any other bucket where it may exist. Returns
// the assigned bucket and whether the session was already present there.
// Takes bucketMu.
func (c *Component) placeSessionInBucket(sessionID string) (bucketId int, alreadyPresent bool) {
	target := bucketForSession(sessionID)

	c.bucketMu.Lock()
	defer c.bucketMu.Unlock()

	for bid, sessions := range c.buckets {
		if bid == target {
			continue
		}
		for i, sid := range sessions {
			if sid == sessionID {
				c.buckets[bid] = append(sessions[:i], sessions[i+1:]...)
				break
			}
		}
	}

	for _, sid := range c.buckets[target] {
		if sid == sessionID {
			return target, true
		}
	}
	c.buckets[target] = append(c.buckets[target], sessionID)
	return target, false
}

func (c *Component) handleSessionLifecycle(event events.Event) {
	data, ok := event.Data.(*events.SessionLifecycleEvent)
	if !ok {
		c.logger.Error("Invalid event data for session lifecycle")
		return
	}

	sessionId := data.SessionID

	var username, mac, ipv4Address, acctSessionID, accessInterface string
	var sessionState models.SessionState
	var swIfIndex, accessIfIndex uint32
	var svlan, cvlan uint16
	attributes := make(map[string]string)

	switch data.AccessType {
	case models.AccessTypeIPoE:
		if sess, ok := data.Session.(*models.IPoESession); ok {
			sessionState = sess.State
			mac = sess.MAC.String()
			if sess.IPv4Address != nil {
				ipv4Address = sess.IPv4Address.String()
			}
			username = sess.Username
			acctSessionID = sess.AAASessionID
			swIfIndex = sess.IfIndex
			accessIfIndex = sess.AccessIfIndex
			accessInterface = sess.AccessInterface
			svlan = sess.OuterVLAN
			cvlan = sess.InnerVLAN
			for k, v := range sess.Attributes {
				attributes[k] = v
			}
		}
	case models.AccessTypePPPoE:
		if sess, ok := data.Session.(*models.PPPSession); ok {
			sessionState = sess.State
			mac = sess.MAC.String()
			if sess.IPv4Address != nil {
				ipv4Address = sess.IPv4Address.String()
			}
			username = sess.Username
			acctSessionID = sess.AAASessionID
			swIfIndex = sess.IfIndex
			accessIfIndex = sess.AccessIfIndex
			accessInterface = sess.AccessInterface
			svlan = sess.OuterVLAN
			cvlan = sess.InnerVLAN
			for k, v := range sess.Attributes {
				attributes[k] = v
			}
		}
	}
	if ipv4Address != "" {
		attributes["ipv4_address"] = ipv4Address
	}

	if sessionState == models.SessionStateReleased {
		if err := c.handleSessionRelease(sessionId, username, mac, acctSessionID, attributes); err != nil {
			c.logger.Error("Failed to handle session release", "session_id", sessionId, "error", err)
		}
		return
	}

	bucketId, alreadyPresent := c.placeSessionInBucket(sessionId)
	if alreadyPresent {
		return
	}

	c.logger.Debug("Added session to bucket for accounting", "sessionId", sessionId, "bucketId", bucketId)

	c.acctCacheMu.Lock()
	acctSession := &AccountingSession{
		sessionID:       sessionId,
		acctSessionID:   acctSessionID,
		accessType:      data.AccessType,
		authDate:        event.Timestamp,
		username:        username,
		mac:             mac,
		ipv4Address:     ipv4Address,
		svlan:           svlan,
		cvlan:           cvlan,
		accessIfIndex:   accessIfIndex,
		accessInterface: accessInterface,
		attributes:      attributes,
		swIfIndex:       swIfIndex,
	}
	c.acctCache[sessionId] = acctSession
	c.acctCacheMu.Unlock()
	c.checkpointAcctSession(acctSession)

	session := &auth.Session{
		SessionID:         sessionId,
		AcctSessionID:     acctSessionID,
		Username:          username,
		MAC:               mac,
		AccessType:        string(data.AccessType),
		AccessInterface:   accessInterface,
		SVLAN:             svlan,
		CVLAN:             cvlan,
		AccessIfIndex:     accessIfIndex,
		SubscriberIfIndex: swIfIndex,
		SessionDuration:   0,
		Attributes:        attributes,
	}

	go c.authProvider.StartAccounting(c.Ctx, session)
}

// handleSessionRestored confirms an acctCache entry loaded from opdb by
// loadAcctSessions, OR creates a fresh entry if the session arrives
// without a persisted accounting checkpoint. Critically does NOT emit
// Accounting-Start — that is the Lifecycle(Active) path's job, and
// emitting Start on every restore would violate the RFC 2866
// one-Start-per-session invariant and confuse RADIUS billing.
//
// Also re-resolves the dataplane sw_if_index from the event payload so
// the next Acct-Interim reads VPP counters at the correct (possibly
// re-numbered post-restart) interface.
func (c *Component) handleSessionRestored(event events.Event) {
	data, ok := event.Data.(*events.SessionRestoredEvent)
	if !ok || data == nil || data.Session == nil {
		return
	}
	sess := data.Session

	// Re-slot the restored session into its deterministic bucket. Hashing
	// the session ID (not the event timestamp) keeps a session in exactly
	// one bucket across restarts and across any subsequent lifecycle
	// re-publishes — RFC 2866 wants one interim cadence per session.
	c.placeSessionInBucket(data.SessionID)

	swIfIndex := sess.GetIfIndex()
	username := sess.GetUsername()
	acctSessionID := sess.GetAAASessionID()
	macStr := ""
	if mac := sess.GetMAC(); mac != nil {
		macStr = mac.String()
	}
	ipv4 := ""
	if ip := sess.GetIPv4Address(); ip != nil {
		ipv4 = ip.String()
	}

	c.acctCacheMu.Lock()
	defer c.acctCacheMu.Unlock()

	if existing, ok := c.acctCache[data.SessionID]; ok {
		// Persisted entry from loadAcctSessions — confirm it now that
		// the session is back. Refresh the volatile fields (sw_if_index,
		// IP address, attributes) from the live restore payload; the
		// persisted counter baseline stays as-is.
		existing.pendingSessionConfirm = false
		existing.swIfIndex = swIfIndex
		existing.ipv4Address = ipv4
		if existing.acctSessionID == "" {
			existing.acctSessionID = acctSessionID
		}
		c.logger.Debug("Confirmed restored acct cache entry",
			"session_id", data.SessionID,
			"acct_session_id", existing.acctSessionID,
			"restore_cause", string(data.RestoreCause))
		return
	}

	// No persisted entry — restored session predates AAA accounting
	// state (or the entry was pruned). Seed a fresh cache row from
	// the session payload; counters start at zero.
	c.acctCache[data.SessionID] = &AccountingSession{
		sessionID:     data.SessionID,
		acctSessionID: acctSessionID,
		accessType:    data.AccessType,
		authDate:      event.Timestamp,
		username:      username,
		mac:           macStr,
		ipv4Address:   ipv4,
		svlan:         sess.GetOuterVLAN(),
		cvlan:         sess.GetInnerVLAN(),
		swIfIndex:     swIfIndex,
		attributes:    map[string]string{"ipv4_address": ipv4},
	}
	c.logger.Info("Seeded acct cache from restored session with no prior checkpoint",
		"session_id", data.SessionID,
		"acct_session_id", acctSessionID,
		"restore_cause", string(data.RestoreCause))
}

func (c *Component) handleSessionRelease(sessionId, username, mac, acctSessionID string, attributes map[string]string) error {
	c.logger.Debug("Session released, sending stop accounting", "sessionId", sessionId)

	c.acctCacheMu.Lock()
	acctSession, exists := c.acctCache[sessionId]
	if exists {
		delete(c.acctCache, sessionId)
	}
	c.acctCacheMu.Unlock()
	c.deleteAcctCheckpoint(sessionId)

	c.bucketMu.Lock()
	for bucketId, sessions := range c.buckets {
		for i, sid := range sessions {
			if sid == sessionId {
				c.buckets[bucketId] = append(sessions[:i], sessions[i+1:]...)
				break
			}
		}
	}
	c.bucketMu.Unlock()

	var sessionDuration uint32
	var accessType, accessInterface string
	var svlan, cvlan uint16
	var accessIfIndex, subscriberIfIndex uint32
	var rxBytes, txBytes, rxPackets, txPackets uint64
	if exists && acctSession != nil {
		sessionDuration = uint32(time.Since(acctSession.authDate).Seconds())
		accessType = string(acctSession.accessType)
		accessInterface = acctSession.accessInterface
		svlan = acctSession.svlan
		cvlan = acctSession.cvlan
		accessIfIndex = acctSession.accessIfIndex
		subscriberIfIndex = acctSession.swIfIndex

		statsByIdx := c.fetchInterfaceStats()
		acctSession.mu.Lock()
		rxBytes, txBytes, rxPackets, txPackets = acctSession.lastReportedInOctets,
			acctSession.lastReportedOutOctets,
			acctSession.lastReportedInPackets,
			acctSession.lastReportedOutPackets
		if stats, ok := statsByIdx[acctSession.swIfIndex]; ok {
			rxBytes, txBytes, rxPackets, txPackets = acctSession.applyVPPCounters(stats)
		}
		acctSession.mu.Unlock()
	}

	session := &auth.Session{
		SessionID:         sessionId,
		AcctSessionID:     acctSessionID,
		Username:          username,
		MAC:               mac,
		AccessType:        accessType,
		AccessInterface:   accessInterface,
		SVLAN:             svlan,
		CVLAN:             cvlan,
		AccessIfIndex:     accessIfIndex,
		SubscriberIfIndex: subscriberIfIndex,
		RxBytes:           rxBytes,
		TxBytes:           txBytes,
		RxPackets:         rxPackets,
		TxPackets:         txPackets,
		SessionDuration:   sessionDuration,
		Attributes:        attributes,
	}

	go c.authProvider.StopAccounting(c.Ctx, session)

	return nil
}

func (c *Component) GetStatsSnapshot() []*ServerStats {
	return []*ServerStats{}
}
