// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ipoe

import (
	"time"

	"github.com/veesix-networks/osvbng/pkg/allocator"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/models"
)

func (c *Component) publishSessionProgrammed(sess *SessionState, swIfIndex uint32) {
	sess.mu.Lock()
	ipoeSess := &models.IPoESession{
		SessionID:    sess.SessionID,
		State:        models.SessionStateActive,
		AccessType:   string(models.AccessTypeIPoE),
		Protocol:     string(models.ProtocolDHCPv4),
		MAC:          sess.MAC,
		OuterVLAN:    sess.OuterVLAN,
		InnerVLAN:    sess.InnerVLAN,
		VLANCount:    c.getVLANCount(sess.OuterVLAN, sess.InnerVLAN),
		IfIndex:      swIfIndex,
		VRF:          sess.VRF,
		ServiceGroup: sess.ServiceGroup.Name,
		SRGName:      sess.SRGName,
		IPv4Address:  sess.IPv4,
		Username:     sess.Username,
		AAASessionID: sess.AcctSessionID,
	}
	sess.mu.Unlock()

	c.eventBus.Publish(events.TopicSessionProgrammed, events.Event{
		Source: c.Name(),
		Data: &events.SessionLifecycleEvent{
			AccessType: ipoeSess.GetAccessType(),
			Protocol:   ipoeSess.GetProtocol(),
			SessionID:  ipoeSess.GetSessionID(),
			State:      ipoeSess.GetState(),
			Session:    ipoeSess,
		},
	})
}

func (c *Component) publishSessionLifecycle(payload models.SubscriberSession) error {
	c.eventBus.Publish(events.TopicSessionLifecycle, events.Event{
		Source: c.Name(),
		Data: &events.SessionLifecycleEvent{
			AccessType: payload.GetAccessType(),
			Protocol:   payload.GetProtocol(),
			SessionID:  payload.GetSessionID(),
			State:      payload.GetState(),
			Session:    payload,
		},
	})
	return nil
}

func (c *Component) handleSubscriberMutation(ev events.Event) {
	data, ok := ev.Data.(*events.SubscriberMutationEvent)
	if !ok {
		return
	}

	sess := c.resolveTargetFromEvent(data)
	if sess == nil {
		return
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	if sess.Closing || sess.State == string(models.SessionStateReleased) {
		c.publishMutationResult(data.RequestID, data.SessionID, false, "session released during mutation", 503, nil)
		return
	}

	if sess.Attributes == nil {
		sess.Attributes = make(map[string]string)
	}
	for k, v := range data.AttributeDelta {
		sess.Attributes[k] = v
	}

	if err := c.checkpointSessionSync(sess); err != nil {
		c.publishMutationResult(data.RequestID, data.SessionID, false, err.Error(), 506, nil)
		return
	}

	snapshot := c.buildModelSnapshot(sess)
	c.publishMutationResult(data.RequestID, data.SessionID, true, "", 0, snapshot)
}

func (c *Component) handleSubscriberTerminate(ev events.Event) {
	data, ok := ev.Data.(*events.SubscriberTerminateEvent)
	if !ok {
		return
	}

	sess := c.resolveTerminateTarget(data)
	if sess == nil {
		return
	}

	sess.mu.Lock()
	if sess.Closing {
		sess.mu.Unlock()
		return
	}
	sess.Closing = true

	mac := sess.MAC
	ipv4 := sess.IPv4
	ipv6Addr := sess.IPv6Address
	ipv6Prefix := sess.IPv6Prefix
	encapIfIndex := sess.EncapIfIndex
	ipoeSwIfIndex := sess.IPoESwIfIndex
	innerVLAN := sess.InnerVLAN
	acctSessionID := sess.AcctSessionID
	username := sess.Username
	vrf := sess.VRF
	srgName := sess.SRGName
	outerVLAN := sess.OuterVLAN
	sess.mu.Unlock()

	if registry := allocator.GetGlobalRegistry(); registry != nil {
		if ipv4 != nil {
			registry.ReleaseIP(ipv4)
		}
		if ipv6Addr != nil {
			registry.ReleaseIANAByIP(ipv6Addr)
		}
		if ipv6Prefix != nil {
			registry.ReleasePDByPrefix(ipv6Prefix)
		}
	}

	if c.vpp != nil && ipoeSwIfIndex != 0 {
		c.vpp.DeleteIPoESessionAsync(mac, encapIfIndex, innerVLAN, func(err error) {
			if err != nil {
				c.logger.Warn("Failed to delete IPoE session on terminate", "session_id", data.SessionID, "error", err)
			}
		})
	}

	lookupKey := c.makeSessionKeyV4(mac, outerVLAN, innerVLAN)
	c.sessions.Delete(lookupKey)
	lookupKeyV6 := c.makeSessionKeyV6(mac, outerVLAN, innerVLAN)
	c.sessions.Delete(lookupKeyV6)
	c.sessionIndex.Delete(sess.SessionID)
	c.removeSessionFromIndexes(sess)
	c.releaseTuple(sess)
	c.deleteSessionCheckpoint(sess.SessionID)

	c.publishSessionLifecycle(&models.IPoESession{
		SessionID:    sess.SessionID,
		State:        models.SessionStateReleased,
		AccessType:   string(models.AccessTypeIPoE),
		Protocol:     string(models.ProtocolDHCPv4),
		AAASessionID: acctSessionID,
		MAC:          mac,
		OuterVLAN:    outerVLAN,
		InnerVLAN:    innerVLAN,
		VRF:          vrf,
		SRGName:      srgName,
		Username:     username,
		IPv4Address:  ipv4,
		IfIndex:      ipoeSwIfIndex,
	})

	c.logger.Debug("Session terminated by external request",
		"session_id", sess.SessionID,
		"reason", data.Reason)
}

func (c *Component) publishMutationResult(requestID, sessionID string, ok bool, errMsg string, errCause int, session models.SubscriberSession) {
	c.eventBus.Publish(events.TopicSubscriberMutationResult, events.Event{
		Source:    c.Name(),
		Timestamp: time.Now(),
		Data: &events.SubscriberMutationResultEvent{
			RequestID:  requestID,
			SessionID:  sessionID,
			Ok:         ok,
			Error:      errMsg,
			ErrorCause: errCause,
			Session:    session,
		},
	})
}
