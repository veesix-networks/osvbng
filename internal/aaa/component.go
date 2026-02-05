package aaa

import (
	"context"
	"fmt"
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

	if err := c.eventBus.Subscribe(events.TopicAAARequest, c.handleAAARequest); err != nil {
		return fmt.Errorf("subscribe to aaa requests: %w", err)
	}

	if err := c.eventBus.Subscribe(events.TopicSessionLifecycle, c.handleSessionLifecycle); err != nil {
		return fmt.Errorf("subscribe to session lifecycle: %w", err)
	}

	c.BuildAccountingBuckets()

	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.logger.Info("Stopping AAA component")
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

func (c *Component) handleAAARequest(event models.Event) error {
	var req models.AAARequest
	if err := event.GetPayload(&req); err != nil {
		return fmt.Errorf("failed to decode AAA request: %w", err)
	}

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
		AccessType:    string(event.AccessType),
		PolicyName:    req.PolicyName,
		Attributes:    attrs,
	}

	authResp, err := c.authProvider.Authenticate(c.Ctx, authReq)
	if err != nil {
		c.logger.Error("Authentication failed",
			"mac", req.MAC,
			"acct_session_id", req.AcctSessionID,
			"error", err)
		return c.publishResponse(req.RequestID, event.SessionID, event.AccessType, false, nil, err)
	}

	c.logger.Debug("Authentication response",
		"allowed", authResp.Allowed,
		"mac", req.MAC,
		"acct_session_id", req.AcctSessionID)

	respAttrs := make(map[string]interface{})
	for k, v := range authResp.Attributes {
		respAttrs[k] = v
	}

	return c.publishResponse(req.RequestID, event.SessionID, event.AccessType, authResp.Allowed, respAttrs, nil)
}

func (c *Component) publishResponse(requestID, sessionID string, accessType models.AccessType, allowed bool, attributes map[string]interface{}, authErr error) error {
	resp := &models.AAAResponse{
		RequestID:  requestID,
		Allowed:    allowed,
		Attributes: attributes,
	}

	if authErr != nil {
		resp.Error = authErr.Error()
	}

	responseEvent := models.Event{
		Type:       models.EventTypeAAAResponse,
		AccessType: accessType,
		SessionID:  sessionID,
	}

	if err := responseEvent.SetPayload(resp); err != nil {
		return fmt.Errorf("failed to set payload: %w", err)
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

	return c.eventBus.Publish(topic, responseEvent)
}

func calculateBucket(authTime time.Time, bucketSize time.Duration) int {
	second := authTime.Second()
	nanos := authTime.Nanosecond()
	totalNanos := int64(second)*1e9 + int64(nanos)
	return int(totalNanos / bucketSize.Nanoseconds())
}

func (c *Component) handleSessionLifecycle(event models.Event) error {
	sessionId := event.SessionID

	var username, mac, ipv4Address, acctSessionID string
	var sessionState models.SessionState
	attributes := make(map[string]string)

	switch event.AccessType {
	case models.AccessTypeIPoE:
		var sess models.IPoESession
		if err := event.GetPayload(&sess); err == nil {
			sessionState = sess.State
			mac = sess.MAC.String()
			if sess.IPv4Address != nil {
				ipv4Address = sess.IPv4Address.String()
				attributes["ipv4_address"] = ipv4Address
			}
			if user, ok := sess.RADIUSAttributes["username"]; ok {
				username = user
			}
			acctSessionID = sess.RADIUSSessionID
		}
	case models.AccessTypePPPoE:
		var sess models.PPPSession
		if err := event.GetPayload(&sess); err == nil {
			sessionState = sess.State
			mac = sess.MAC.String()
			if sess.IPv4Address != nil {
				ipv4Address = sess.IPv4Address.String()
				attributes["ipv4_address"] = ipv4Address
			}
			if user, ok := sess.RADIUSAttributes["username"]; ok {
				username = user
			}
			acctSessionID = sess.RADIUSSessionID
		}
	}

	if sessionState == models.SessionStateReleased {
		return c.handleSessionRelease(sessionId, username, mac, acctSessionID, attributes)
	}

	bucketId := calculateBucket(event.Timestamp, defaultBucketSize)

	c.bucketMu.Lock()
	if sessions, exists := c.buckets[bucketId]; exists {
		for _, sid := range sessions {
			if sid == sessionId {
				c.bucketMu.Unlock()
				return nil
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
		accessType:    event.AccessType,
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

	return nil
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
