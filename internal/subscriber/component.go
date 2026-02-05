package subscriber

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
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
	cfgMgr    component.ConfigManager
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
		cfgMgr:   deps.ConfigManager,
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

func (c *Component) GetSessionsForAPI(ctx context.Context, accessType, protocol string, svlan uint32) ([]models.SubscriberSession, error) {
	return c.GetSessions(ctx, accessType, protocol, svlan)
}

func (c *Component) GetSessions(ctx context.Context, accessType, protocol string, svlan uint32) ([]models.SubscriberSession, error) {
	pattern := "osvbng:sessions:*"

	var cursor uint64
	var sessions []models.SubscriberSession

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

			var meta struct {
				SessionID  string `json:"SessionID"`
				AccessType string `json:"AccessType"`
				Protocol   string `json:"Protocol"`
				OuterVLAN  uint16 `json:"OuterVLAN"`
			}
			if err := json.Unmarshal(data, &meta); err != nil {
				c.logger.Debug("Failed to unmarshal metadata", "key", key, "error", err)
				continue
			}

			if meta.SessionID == "" {
				c.logger.Debug("Empty session_id", "key", key)
				continue
			}

			if accessType != "" && meta.AccessType != accessType {
				continue
			}

			if protocol != "" && meta.Protocol != protocol {
				continue
			}

			if svlan > 0 && uint32(meta.OuterVLAN) != svlan {
				continue
			}

			var sess models.SubscriberSession
			switch meta.AccessType {
			case string(models.AccessTypePPPoE):
				var pppSess models.PPPSession
				if err := json.Unmarshal(data, &pppSess); err != nil {
					c.logger.Debug("Failed to unmarshal PPP session", "key", key, "error", err)
					continue
				}
				sess = &pppSess
			case string(models.AccessTypeIPoE):
				switch meta.Protocol {
				case string(models.ProtocolDHCPv6):
					var dhcp6Sess models.IPoESession
					if err := json.Unmarshal(data, &dhcp6Sess); err != nil {
						c.logger.Debug("Failed to unmarshal DHCPv6 session", "key", key, "error", err)
						continue
					}
					sess = &dhcp6Sess
				default:
					var dhcp4Sess models.IPoESession
					if err := json.Unmarshal(data, &dhcp4Sess); err != nil {
						c.logger.Debug("Failed to unmarshal DHCPv4 session", "key", key, "error", err)
						continue
					}
					sess = &dhcp4Sess
				}
			default:
				var dhcp4Sess models.IPoESession
				if err := json.Unmarshal(data, &dhcp4Sess); err != nil {
					c.logger.Debug("Failed to unmarshal DHCP session", "key", key, "error", err)
					continue
				}
				sess = &dhcp4Sess
			}

			c.logger.Debug("Found session", "session_id", meta.SessionID, "access_type", meta.AccessType)
			sessions = append(sessions, sess)
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return sessions, nil
}

func (c *Component) GetSession(ctx context.Context, sessionID string) (*models.IPoESession, error) {
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

	var sess models.IPoESession
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
	var sess models.SubscriberSession
	var leaseTime uint32

	switch event.AccessType {
	case models.AccessTypeIPoE:
		switch event.Protocol {
		case models.ProtocolDHCPv6:
			var dhcp6Sess models.IPoESession
			if err := event.GetPayload(&dhcp6Sess); err != nil {
				return fmt.Errorf("failed to decode DHCPv6 session: %w", err)
			}
			sess = &dhcp6Sess
		default:
			var dhcp4Sess models.IPoESession
			if err := event.GetPayload(&dhcp4Sess); err != nil {
				return fmt.Errorf("failed to decode DHCPv4 session: %w", err)
			}
			leaseTime = dhcp4Sess.LeaseTime
			sess = &dhcp4Sess
		}
	case models.AccessTypePPPoE:
		var pppSess models.PPPSession
		if err := event.GetPayload(&pppSess); err != nil {
			return fmt.Errorf("failed to decode PPP session: %w", err)
		}
		sess = &pppSess
	default:
		return fmt.Errorf("unsupported access type: %s", event.AccessType)
	}

	sessionID := sess.GetSessionID()
	macStr := sess.GetMAC().String()
	outerVLAN := sess.GetOuterVLAN()
	innerVLAN := sess.GetInnerVLAN()
	state := sess.GetState()

	if outerVLAN == 0 {
		return fmt.Errorf("session rejected: S-VLAN required (untagged not supported)")
	}

	isDF := true
	if c.srgMgr != nil {
		isDF = c.srgMgr.IsDF(outerVLAN, macStr, innerVLAN)
	}

	if !isDF {
		c.logger.Debug("[NOT-DF] Skipping southbound ops",
			"session_id", sessionID,
			"group", c.srgMgr.GetGroupForSVLAN(outerVLAN))
		c.persistSession(sess)
		return nil
	}

	c.logger.Debug("[DF] Processing session", "session_id", sessionID, "state", state)

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
			c.logger.Debug("Set session expiry", "session_id", sessionID, "expiry_time", expiryTime.Format(time.RFC3339))
		}
	} else if state == models.SessionStateReleased {
		c.expiryMgr.Remove(sessionID)

		if err := c.releaseSession(sess); err != nil {
			c.logger.Error("Error releasing session", "error", err)
		}
	}

	return nil
}

func (c *Component) activateSession(sess models.SubscriberSession) error {
	swIfIndex := sess.GetIfIndex()
	if swIfIndex == 0 {
		return nil
	}

	if err := c.vpp.ApplyQoS(swIfIndex, 10, 10); err != nil {
		c.logger.Warn("Failed to apply QoS", "error", err, "sw_if_index", swIfIndex)
	} else {
		c.logger.Debug("Applied QoS", "sw_if_index", swIfIndex)
	}

	return nil
}

func (c *Component) releaseSession(sess models.SubscriberSession) error {
	return nil
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

func (c *Component) persistSession(sess models.SubscriberSession) error {
	sessionID := sess.GetSessionID()
	key := fmt.Sprintf("osvbng:sessions:%s", sessionID)

	if sess.GetState() == models.SessionStateReleased {
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

		c.logger.Debug("Deleted session from cache", "session_id", sessionID)
		return nil
	}

	data, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	c.logger.Debug("Persisting session to cache", "session_id", sessionID, "key", key, "data_len", len(data), "json", string(data[:min(200, len(data))]))

	ttl := time.Duration(0)
	if dhcpSess, ok := sess.(*models.IPoESession); ok && dhcpSess.LeaseTime > 0 {
		ttl = time.Duration(dhcpSess.LeaseTime) * time.Second
	}

	if err := c.cache.Set(c.Ctx, key, data, ttl); err != nil {
		return fmt.Errorf("set session: %w", err)
	}

	c.logger.Debug("Session persisted to cache", "session_id", sessionID)

	lookupKey := c.buildLookupKey(sess)
	if lookupKey != "" {
		if err := c.cache.Set(c.Ctx, lookupKey, []byte(sessionID), ttl); err != nil {
			return fmt.Errorf("set lookup key: %w", err)
		}
	}

	arpLookupKey := c.buildARPLookupKey(sess)
	if arpLookupKey != "" {
		c.logger.Debug("Creating ARP lookup index", "key", arpLookupKey, "session_id", sessionID)
		if err := c.cache.Set(c.Ctx, arpLookupKey, []byte(sessionID), ttl); err != nil {
			return fmt.Errorf("set arp lookup key: %w", err)
		}
	} else {
		c.logger.Debug("Skipping ARP lookup index creation", "session_id", sessionID, "reason", "empty key")
	}

	return nil
}

func (c *Component) buildLookupKey(sess models.SubscriberSession) string {
	mac := sess.GetMAC()
	if mac == nil {
		return ""
	}

	return fmt.Sprintf("osvbng:lookup:%s:%s:%d:%d", sess.GetAccessType(), mac.String(), sess.GetOuterVLAN(), sess.GetInnerVLAN())
}

func (c *Component) buildARPLookupKey(sess models.SubscriberSession) string {
	ipv4 := sess.GetIPv4Address()
	if ipv4 == nil {
		return ""
	}

	ifIndex := sess.GetIfIndex()
	if ifIndex == 0 {
		return ""
	}

	return fmt.Sprintf("osvbng:lookup:arp:%d:%s", ifIndex, ipv4.String())
}

func (c *Component) handleSessionExpiry(sessionID string, expiryTime time.Time) {
	c.logger.Debug("Session expired", "session_id", sessionID, "expiry_time", expiryTime.Format(time.RFC3339))

	key := fmt.Sprintf("osvbng:sessions:%s", sessionID)
	data, err := c.cache.Get(c.Ctx, key)
	if err != nil {
		c.logger.Warn("Failed to get session", "session_id", sessionID, "error", err)
		return
	}

	if len(data) == 0 {
		c.logger.Warn("Empty session data from cache", "session_id", sessionID)
		return
	}

	var meta struct {
		AccessType string `json:"AccessType"`
		Protocol   string `json:"Protocol"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		c.logger.Warn("Failed to unmarshal session metadata", "session_id", sessionID, "error", err)
		return
	}

	if err := c.cache.Delete(c.Ctx, key); err != nil {
		c.logger.Warn("Failed to delete expired session", "session_id", sessionID, "error", err)
	}

	accessType := models.AccessType(meta.AccessType)
	protocol := models.Protocol(meta.Protocol)

	if accessType == "" {
		accessType = models.AccessTypeIPoE
	}
	if protocol == "" {
		protocol = models.ProtocolDHCPv4
	}

	var payload interface{}
	switch accessType {
	case models.AccessTypePPPoE:
		var pppSess models.PPPSession
		if err := json.Unmarshal(data, &pppSess); err != nil {
			c.logger.Warn("Failed to unmarshal PPP session", "session_id", sessionID, "error", err)
			return
		}
		pppSess.State = models.SessionStateReleased
		payload = &pppSess
	case models.AccessTypeIPoE:
		switch protocol {
		case models.ProtocolDHCPv6:
			var dhcp6Sess models.IPoESession
			if err := json.Unmarshal(data, &dhcp6Sess); err != nil {
				c.logger.Warn("Failed to unmarshal DHCPv6 session", "session_id", sessionID, "error", err)
				return
			}
			dhcp6Sess.State = models.SessionStateReleased
			payload = &dhcp6Sess
		default:
			var dhcp4Sess models.IPoESession
			if err := json.Unmarshal(data, &dhcp4Sess); err != nil {
				c.logger.Warn("Failed to unmarshal DHCPv4 session", "session_id", sessionID, "error", err)
				return
			}
			dhcp4Sess.State = models.SessionStateReleased
			payload = &dhcp4Sess
		}
	default:
		var dhcp4Sess models.IPoESession
		if err := json.Unmarshal(data, &dhcp4Sess); err != nil {
			c.logger.Warn("Failed to unmarshal session", "session_id", sessionID, "error", err)
			return
		}
		dhcp4Sess.State = models.SessionStateReleased
		payload = &dhcp4Sess
	}

	releaseEvent := models.Event{
		Type:       models.EventTypeSessionLifecycle,
		AccessType: accessType,
		Protocol:   protocol,
		SessionID:  sessionID,
	}
	releaseEvent.SetPayload(payload)

	if err := c.eventBus.Publish(events.TopicSessionLifecycle, releaseEvent); err != nil {
		c.logger.Warn("Failed to publish expiry event", "session_id", sessionID, "error", err)
	}
}
