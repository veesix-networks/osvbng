package aaa

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/auth"
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
)

const (
	defaultBucketSize = 5 * time.Second
)

type AccountingSession struct {
	sessionID         string
	acctSessionID     string
	accessType        models.AccessType
	authDate          time.Time
	lastInterimUpdate time.Time
	username          string
	mac               string
	ipv4Address       string
	attributes        map[string]string
}

type Component struct {
	*component.Base

	logger       *slog.Logger
	authProvider auth.AuthProvider
	eventBus     events.Bus
	cache        cache.Cache

	aaaReqSub    events.Subscription
	lifecycleSub events.Subscription

	buckets  map[int][]string
	bucketMu sync.RWMutex

	acctCache   map[string]*AccountingSession
	acctCacheMu sync.RWMutex
}

func New(deps component.Dependencies, authProvider auth.AuthProvider) (component.Component, error) {
	log := logger.Get(logger.AAA)

	c := &Component{
		Base:         component.NewBase("aaa"),
		logger:       log,
		authProvider: authProvider,
		eventBus:     deps.EventBus,
		cache:        deps.Cache,
		buckets:      make(map[int][]string),
		acctCache:    make(map[string]*AccountingSession),
	}

	return c, nil
}

func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)
	c.logger.Info("Starting AAA component")

	c.aaaReqSub = c.eventBus.Subscribe(events.TopicAAARequest, c.handleAAARequest)
	c.lifecycleSub = c.eventBus.Subscribe(events.TopicSessionLifecycle, c.handleSessionLifecycle)

	c.BuildAccountingBuckets()

	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.logger.Info("Stopping AAA component")
	c.aaaReqSub.Unsubscribe()
	c.lifecycleSub.Unsubscribe()
	c.StopContext()
	return nil
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

	c.acctCacheMu.Lock()
	defer c.acctCacheMu.Unlock()

	for _, sessionId := range sessionIds {
		session, exists := c.acctCache[sessionId]
		if !exists {
			continue
		}

		go c.sendAccountingUpdate(session)
	}
}

func (c *Component) sendAccountingUpdate(acctSession *AccountingSession) {
	session := &auth.Session{
		SessionID:       acctSession.sessionID,
		AcctSessionID:   acctSession.acctSessionID,
		Username:        acctSession.username,
		MAC:             acctSession.mac,
		SessionDuration: uint32(time.Since(acctSession.authDate).Seconds()),
		Attributes:      acctSession.attributes,
	}

	if err := c.authProvider.UpdateAccounting(c.Ctx, session); err != nil {
		c.logger.Debug("Accounting update failed", "session_id", acctSession.sessionID, "error", err)
	}
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
		Username:      req.Username,
		MAC:           req.MAC,
		AcctSessionID: req.AcctSessionID,
		SVLAN:         req.SVLAN,
		CVLAN:         req.CVLAN,
		Interface:     req.Interface,
		AccessType:    string(data.AccessType),
		PolicyName:    req.PolicyName,
		Attributes:    attrs,
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

	c.logger.Info("Authentication response",
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

func calculateBucket(authTime time.Time, bucketSize time.Duration) int {
	second := authTime.Second()
	nanos := authTime.Nanosecond()
	totalNanos := int64(second)*1e9 + int64(nanos)
	return int(totalNanos / bucketSize.Nanoseconds())
}

func (c *Component) handleSessionLifecycle(event events.Event) {
	data, ok := event.Data.(*events.SessionLifecycleEvent)
	if !ok {
		c.logger.Error("Invalid event data for session lifecycle")
		return
	}

	sessionId := data.SessionID

	var username, mac, ipv4Address, acctSessionID string
	var sessionState models.SessionState
	attributes := make(map[string]string)

	switch data.AccessType {
	case models.AccessTypeIPoE:
		if sess, ok := data.Session.(*models.IPoESession); ok {
			sessionState = sess.State
			mac = sess.MAC.String()
			if sess.IPv4Address != nil {
				ipv4Address = sess.IPv4Address.String()
				attributes["ipv4_address"] = ipv4Address
			}
			username = sess.Username
			acctSessionID = sess.AAASessionID
		}
	case models.AccessTypePPPoE:
		if sess, ok := data.Session.(*models.PPPSession); ok {
			sessionState = sess.State
			mac = sess.MAC.String()
			if sess.IPv4Address != nil {
				ipv4Address = sess.IPv4Address.String()
				attributes["ipv4_address"] = ipv4Address
			}
			username = sess.Username
			acctSessionID = sess.AAASessionID
		}
	}

	if sessionState == models.SessionStateReleased {
		if err := c.handleSessionRelease(sessionId, username, mac, acctSessionID, attributes); err != nil {
			c.logger.Error("Failed to handle session release", "session_id", sessionId, "error", err)
		}
		return
	}

	bucketId := calculateBucket(event.Timestamp, defaultBucketSize)

	c.bucketMu.Lock()
	if sessions, exists := c.buckets[bucketId]; exists {
		for _, sid := range sessions {
			if sid == sessionId {
				c.bucketMu.Unlock()
				return
			}
		}
	}
	c.buckets[bucketId] = append(c.buckets[bucketId], sessionId)
	c.bucketMu.Unlock()

	c.logger.Debug("Added session to bucket for accounting", "sessionId", sessionId, "bucketId", bucketId)

	c.acctCacheMu.Lock()
	acctSession := &AccountingSession{
		sessionID:     sessionId,
		acctSessionID: acctSessionID,
		accessType:    data.AccessType,
		authDate:      event.Timestamp,
		username:      username,
		mac:           mac,
		ipv4Address:   ipv4Address,
		attributes:    attributes,
	}
	c.acctCache[sessionId] = acctSession
	c.acctCacheMu.Unlock()

	session := &auth.Session{
		SessionID:       sessionId,
		AcctSessionID:   acctSessionID,
		Username:        username,
		MAC:             mac,
		SessionDuration: 0,
		Attributes:      attributes,
	}

	go c.authProvider.StartAccounting(c.Ctx, session)
}

func (c *Component) handleSessionRelease(sessionId, username, mac, acctSessionID string, attributes map[string]string) error {
	c.logger.Debug("Session released, sending stop accounting", "sessionId", sessionId)

	c.acctCacheMu.Lock()
	acctSession, exists := c.acctCache[sessionId]
	if exists {
		delete(c.acctCache, sessionId)
	}
	c.acctCacheMu.Unlock()

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
	if exists && acctSession != nil {
		sessionDuration = uint32(time.Since(acctSession.authDate).Seconds())
	}

	session := &auth.Session{
		SessionID:       sessionId,
		AcctSessionID:   acctSessionID,
		Username:        username,
		MAC:             mac,
		SessionDuration: sessionDuration,
		Attributes:      attributes,
	}

	go c.authProvider.StopAccounting(c.Ctx, session)

	return nil
}

func (c *Component) GetStatsSnapshot() []*ServerStats {
	return []*ServerStats{}
}
