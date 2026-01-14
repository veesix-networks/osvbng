package subscriber

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/models/subscribers"
	"github.com/veesix-networks/osvbng/pkg/session"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/srg"
)

type Component struct {
	*component.Base

	logger    *slog.Logger
	eventBus  events.Bus
	srgMgr    *srg.Manager
	vpp       *southbound.VPP
	expiryMgr *session.ExpiryManager
	cfg       *config.Config
	cache     cache.Cache
}

func New(deps component.Dependencies, srgMgr *srg.Manager) (component.Component, error) {
	log := logger.Component(logger.ComponentSubscriber)

	c := &Component{
		Base:     component.NewBase("subscriber"),
		logger:   log,
		eventBus: deps.EventBus,
		srgMgr:   srgMgr,
		vpp:      deps.VPP,
		cfg:      deps.Config,
		cache:    deps.Cache,
	}

	c.expiryMgr = session.NewExpiryManager(c.handleSessionExpiry)

	return c, nil
}

func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)
	c.logger.Info("Starting subscriber management component")

	if c.srgMgr != nil {
		c.srgMgr.Start()
	}
	c.expiryMgr.Start()

	if err := c.eventBus.Subscribe(events.TopicSessionLifecycle, c.handleSessionLifecycle); err != nil {
		return fmt.Errorf("subscribe to session lifecycle: %w", err)
	}

	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.logger.Info("Stopping subscriber management component")

	if c.srgMgr != nil {
		c.srgMgr.Stop()
	}
	c.expiryMgr.Stop()

	c.eventBus.Unsubscribe(events.TopicSessionLifecycle, c.handleSessionLifecycle)

	c.StopContext()

	return nil
}

func (c *Component) GetSessionStats(ctx context.Context) ([]subscribers.Statistics, error) {
	return c.vpp.GetSubscriberStats(ctx)
}

func (c *Component) GetSessionsForAPI(ctx context.Context, accessType, protocol string, svlan uint32) ([]*models.DHCPv4Session, error) {
	sessions, err := c.GetSessions(ctx, accessType, protocol, svlan)
	if err != nil {
		return nil, err
	}

	result := make([]*models.DHCPv4Session, 0, len(sessions))
	for _, s := range sessions {
		result = append(result, s)
	}
	return result, nil
}

func (c *Component) GetSessions(ctx context.Context, accessType, protocol string, svlan uint32) ([]*models.DHCPv4Session, error) {
	pattern := "osvbng:sessions:*"

	var cursor uint64
	var sessions []*models.DHCPv4Session

	for {
		keys, nextCursor, err := c.cache.Scan(ctx, cursor, pattern, 100)
		if err != nil {
			return nil, fmt.Errorf("scan sessions: %w", err)
		}

		for _, key := range keys {
			data, err := c.cache.Get(ctx, key)
			if err != nil {
				c.logger.Debug("Failed to get key", "key", key, "error", err)
				continue
			}

			var sess models.DHCPv4Session
			if err := json.Unmarshal(data, &sess); err != nil{
				c.logger.Debug("Failed to unmarshal", "key", key, "error", err)
				continue
			}

			if sess.SessionID == "" {
				c.logger.Debug("Empty session_id", "key", key)
				continue
			}

			if accessType != "" && sess.AccessType != accessType {
				continue
			}

			if protocol != "" && sess.Protocol != protocol {
				continue
			}

			if svlan > 0 && uint32(sess.OuterVLAN) != svlan {
				continue
			}

			c.logger.Debug("Found session", "session_id", sess.SessionID)
			sessions = append(sessions, &sess)
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return sessions, nil
}

func (c *Component) GetSession(ctx context.Context, sessionID string) (*models.DHCPv4Session, error) {
	key := fmt.Sprintf("osvbng:sessions:%s", sessionID)

	data, err := c.cache.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	dataStr := string(data)
	if dataStr == "" {
		dataBytes := data
		if dataStr == "" {
			return nil, fmt.Errorf("invalid session data type")
		}
		dataStr = string(dataBytes)
	}

	var sess models.DHCPv4Session
	if err := json.Unmarshal([]byte(dataStr), &sess); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}

	return &sess, nil
}

func (c *Component) GetStats(ctx context.Context) (map[string]uint32, error) {
	pattern := "osvbng:sessions:*"
	var cursor uint64

	stats := map[string]uint32{
		"total":    0,
		"ipoe_v4":  0,
		"ipoe_v6":  0,
		"ppp":      0,
		"active":   0,
		"released": 0,
	}

	for {
		keys, nextCursor, err := c.cache.Scan(ctx, cursor, pattern, 100)
		if err != nil {
			return nil, fmt.Errorf("scan sessions: %w", err)
		}

		for _, key := range keys {
			data, err := c.cache.Get(ctx, key)
			if err != nil {
				continue
			}

			dataStr := string(data)
			if dataStr == "" {
				dataBytes := data
				if dataStr == "" {
					continue
				}
				dataStr = string(dataBytes)
			}

			var sessionData map[string]interface{}
			if err := json.Unmarshal([]byte(dataStr), &sessionData); err != nil {
				continue
			}

			stats["total"]++

			sessionID, _ := sessionData["session_id"].(string)
			state, _ := sessionData["state"].(string)

			if sessionID != "" {
				if len(sessionID) >= 7 && sessionID[:7] == "ipoe-v4" {
					stats["ipoe_v4"]++
				} else if len(sessionID) >= 7 && sessionID[:7] == "ipoe-v6" {
					stats["ipoe_v6"]++
				} else if len(sessionID) >= 3 && sessionID[:3] == "ppp" {
					stats["ppp"]++
				}
			}

			if state == "active" {
				stats["active"]++
			} else if state == "released" {
				stats["released"]++
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return stats, nil
}

func (c *Component) TerminateSession(ctx context.Context, sessionID string) error {
	sess, err := c.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}

	sess.State = models.SessionStateReleased

	var accessType models.AccessType
	var protocol models.Protocol

	if len(sessionID) >= 7 && sessionID[:7] == "ipoe-v4" {
		accessType = models.AccessTypeIPoE
		protocol = models.ProtocolDHCPv4
	} else if len(sessionID) >= 7 && sessionID[:7] == "ipoe-v6" {
		accessType = models.AccessTypeIPoE
		protocol = models.ProtocolDHCPv6
	} else if len(sessionID) >= 3 && sessionID[:3] == "ppp" {
		accessType = models.AccessTypePPPoE
		protocol = models.ProtocolPPP
	}

	releaseEvent := models.Event{
		Type:       models.EventTypeSessionLifecycle,
		AccessType: accessType,
		Protocol:   protocol,
		SessionID:  sessionID,
	}
	releaseEvent.SetPayload(sess)

	if err := c.eventBus.Publish(events.TopicSessionLifecycle, releaseEvent); err != nil {
		return fmt.Errorf("publish release event: %w", err)
	}

	return nil
}

func (c *Component) handleSessionLifecycle(event models.Event) error {
	var sessionID, macStr string
	var outerVLAN, innerVLAN uint16
	var state models.SessionState
	var leaseTime uint32
	var sess *models.DHCPv4Session

	switch event.AccessType {
	case models.AccessTypeIPoE:
		var dhcpSess models.DHCPv4Session
		if err := event.GetPayload(&dhcpSess); err != nil {
			return fmt.Errorf("failed to decode DHCPv4 session: %w", err)
		}
		sess = &dhcpSess
		sess.AccessType = string(event.AccessType)
		sess.Protocol = string(event.Protocol)
		sessionID = sess.SessionID
		macStr = sess.MAC.String()
		outerVLAN = sess.OuterVLAN
		innerVLAN = sess.InnerVLAN
		state = sess.State
		leaseTime = sess.LeaseTime
	case models.AccessTypePPPoE:
		var pppSess models.PPPSession
		if err := event.GetPayload(&pppSess); err != nil {
			return fmt.Errorf("failed to decode PPP session: %w", err)
		}
		sessionID = pppSess.SessionID
		macStr = pppSess.MAC.String()
		outerVLAN = pppSess.OuterVLAN
		innerVLAN = pppSess.InnerVLAN
		state = pppSess.State
	default:
		return fmt.Errorf("unsupported access type: %s", event.AccessType)
	}

	if outerVLAN == 0 {
		return fmt.Errorf("session rejected: S-VLAN required (untagged not supported)")
	}

	isDF := true
	if c.srgMgr != nil {
		isDF = c.srgMgr.IsDF(outerVLAN, macStr, innerVLAN)
	}

	if !isDF {
		c.logger.Info("[NOT-DF] Skipping southbound ops",
			"session_id", sessionID,
			"group", c.srgMgr.GetGroupForSVLAN(outerVLAN))
		c.persistSession(sess)
		return nil
	}

	c.logger.Info("[DF] Processing session", "session_id", sessionID, "state", state)

	if err := c.persistSession(sess); err != nil {
		return fmt.Errorf("persist session: %w", err)
	}

	if state == models.SessionStateActive {
		if err := c.activateSession(sess); err != nil {
			c.logger.Error("Error activating session", "error", err)
		}

		if leaseTime > 0 {
			expiryTime := time.Now().Add(time.Duration(leaseTime) * time.Second)
			c.expiryMgr.Set(sessionID, expiryTime)
			c.logger.Info("Set session expiry", "session_id", sessionID, "expiry_time", expiryTime.Format(time.RFC3339))
		}
	} else if state == models.SessionStateReleased {
		c.expiryMgr.Remove(sessionID)

		if err := c.releaseSession(sess); err != nil {
			c.logger.Error("Error releasing session", "error", err)
		}
	}

	return nil
}

func (c *Component) activateSession(sess *models.DHCPv4Session) error {
	ifaceName := c.determineInterface(sess)

	if err := c.vpp.ApplyQoS(ifaceName, 10, 10); err != nil {
		c.logger.Warn("Failed to apply QoS", "error", err, "interface", ifaceName)
	} else {
		c.logger.Info("Applied QoS", "interface", ifaceName)
	}

	return nil
}

func (c *Component) releaseSession(sess *models.DHCPv4Session) error {
	return nil
}

func (c *Component) determineInterface(sess *models.DHCPv4Session) string {
	if sess.InterfaceName != "" {
		return sess.InterfaceName
	}

	parentName := c.vpp.GetParentInterface()

	if sess.VLANCount == 1 || sess.InnerVLAN == 0 {
		return fmt.Sprintf("%s.%d", parentName, sess.OuterVLAN)
	}
	return fmt.Sprintf("%s.%d.%d", parentName, sess.OuterVLAN, sess.InnerVLAN)
}

func (c *Component) interfaceExists(name string) bool {
	_, err := c.vpp.GetInterfaceIndex(name)
	return err == nil
}

func (c *Component) countActiveSessions(svlan, cvlan uint16) int {
	pattern := fmt.Sprintf("osvbng:sessions:*:*:%d", cvlan)
	if cvlan == 0 {
		pattern = fmt.Sprintf("osvbng:sessions:*")
	}

	var cursor uint64
	count := 0

	for {
		keys, nextCursor, err := c.cache.Scan(c.Ctx, cursor, pattern, 100)
		if err != nil {
			c.logger.Error("Error scanning sessions", "error", err)
			break
		}

		for _, key := range keys {
			data, err := c.cache.Get(c.Ctx, key)
			if err != nil {
				continue
			}

			dataStr := string(data)
			if dataStr == "" {
				dataBytes := data
				if dataStr == "" {
					continue
				}
				dataStr = string(dataBytes)
			}

			var session map[string]interface{}
			if err := json.Unmarshal([]byte(dataStr), &session); err != nil {
				continue
			}

			if state, ok := session["state"].(string); ok && state == "active" {
				count++
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return count
}

func (c *Component) persistSession(sess *models.DHCPv4Session) error {
	key := fmt.Sprintf("osvbng:sessions:%s", sess.SessionID)

	if sess.State == models.SessionStateReleased {
		if err := c.cache.Delete(c.Ctx, key); err != nil {
			return fmt.Errorf("delete session: %w", err)
		}

		lookupKey := c.buildLookupKey(sess)
		if lookupKey != "" {
			if err := c.cache.Delete(c.Ctx, lookupKey); err != nil {
				c.logger.Warn("Failed to delete lookup key", "key", lookupKey, "error", err)
			}
		}

		arpLookupKey := c.buildARPLookupKey(sess)
		if arpLookupKey != "" {
			if err := c.cache.Delete(c.Ctx, arpLookupKey); err != nil {
				c.logger.Warn("Failed to delete ARP lookup key", "key", arpLookupKey, "error", err)
			}
		}

		c.logger.Info("Deleted session from cache", "session_id", sess.SessionID)
		return nil
	}

	data, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	c.logger.Info("Persisting session to cache", "session_id", sess.SessionID, "key", key, "data_len", len(data), "json", string(data[:min(200, len(data))]))

	ttl := time.Duration(0)
	if sess.LeaseTime > 0 {
		ttl = time.Duration(sess.LeaseTime) * time.Second
	}

	if err := c.cache.Set(c.Ctx, key, data, ttl); err != nil {
		return fmt.Errorf("set session: %w", err)
	}

	c.logger.Info("Session persisted to cache", "session_id", sess.SessionID)

	lookupKey := c.buildLookupKey(sess)
	if lookupKey != "" {
		if err := c.cache.Set(c.Ctx, lookupKey, []byte(sess.SessionID), ttl); err != nil {
			return fmt.Errorf("set lookup key: %w", err)
		}
	}

	arpLookupKey := c.buildARPLookupKey(sess)
	if arpLookupKey != "" {
		c.logger.Info("Creating ARP lookup index", "key", arpLookupKey, "session_id", sess.SessionID)
		if err := c.cache.Set(c.Ctx, arpLookupKey, []byte(sess.SessionID), ttl); err != nil {
			return fmt.Errorf("set arp lookup key: %w", err)
		}
	} else {
		c.logger.Debug("Skipping ARP lookup index creation", "session_id", sess.SessionID, "reason", "empty key")
	}

	return nil
}

func (c *Component) buildLookupKey(sess *models.DHCPv4Session) string {
	if sess.MAC == nil {
		return ""
	}

	return fmt.Sprintf("osvbng:lookup:ipoe-v4:%s:%d:%d", sess.MAC.String(), sess.OuterVLAN, sess.InnerVLAN)
}

func (c *Component) buildARPLookupKey(sess *models.DHCPv4Session) string {
	if sess.IPv4Address == nil {
		c.logger.Debug("buildARPLookupKey: no IPv4Address in session")
		return ""
	}

	if sess.IfIndex == 0 {
		c.logger.Debug("buildARPLookupKey: IfIndex is 0")
		return ""
	}

	key := fmt.Sprintf("osvbng:lookup:arp:%d:%s", sess.IfIndex, sess.IPv4Address.String())
	c.logger.Debug("buildARPLookupKey result", "key", key, "sw_if_index", sess.IfIndex, "ipv4", sess.IPv4Address.String())
	return key
}

func (c *Component) handleSessionExpiry(sessionID string, expiryTime time.Time) {
	c.logger.Info("Session expired", "session_id", sessionID, "expiry_time", expiryTime.Format(time.RFC3339))

	key := fmt.Sprintf("osvbng:sessions:%s", sessionID)
	data, err := c.cache.Get(c.Ctx, key)
	if err != nil {
		c.logger.Warn("Failed to get session", "session_id", sessionID, "error", err)
		return
	}

	dataStr := string(data)
	if dataStr == "" {
		dataBytes := data
		if dataStr == "" {
			c.logger.Warn("Invalid data type from cache", "session_id", sessionID)
			return
		}
		dataStr = string(dataBytes)
	}

	var sess models.DHCPv4Session
	if err := json.Unmarshal([]byte(dataStr), &sess); err != nil {
		c.logger.Warn("Failed to unmarshal session", "session_id", sessionID, "error", err)
		return
	}

	sess.State = models.SessionStateReleased

	if err := c.cache.Delete(c.Ctx, key); err != nil {
		c.logger.Warn("Failed to delete expired session", "session_id", sessionID, "error", err)
	}

	releaseEvent := models.Event{
		Type:       models.EventTypeSessionLifecycle,
		AccessType: models.AccessTypeIPoE,
		Protocol:   models.ProtocolDHCPv4,
		SessionID:  sessionID,
	}
	releaseEvent.SetPayload(&sess)

	if err := c.eventBus.Publish(events.TopicSessionLifecycle, releaseEvent); err != nil {
		c.logger.Warn("Failed to publish expiry event", "session_id", sessionID, "error", err)
	}
}
