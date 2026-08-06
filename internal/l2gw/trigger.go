// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2gw

import (
	"fmt"
	"strconv"
	"time"

	"github.com/google/gopacket/layers"
	"github.com/google/uuid"
	"github.com/veesix-networks/osvbng/pkg/aaa"
	aaacfg "github.com/veesix-networks/osvbng/pkg/config/aaa"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/dhcp6"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/session"
)

const (
	rejectBackoff  = 30 * time.Second
	pendingTimeout = 60 * time.Second
)

func (c *Component) consumeTriggers() {
	for {
		select {
		case <-c.Ctx.Done():
			return
		case pkt := <-c.triggerChan:
			if err := c.handleTrigger(pkt); err != nil {
				c.logger.Debug("l2gw trigger dropped", "error", err)
			}
		}
	}
}

// consumePacketTriggers drains the dedicated packet-mode channel: any
// protocol first-frame punts from ranges armed with trigger: packet.
func (c *Component) consumePacketTriggers() {
	for {
		select {
		case <-c.Ctx.Done():
			return
		case pkt := <-c.packetTriggerChan:
			if err := c.handleTrigger(pkt); err != nil {
				c.logger.Debug("l2gw packet trigger dropped", "error", err)
			}
		}
	}
}

// handleTrigger processes a punted trigger packet from an l2gw
// access-type subscriber group: a DHCP session-initiating message in
// dhcp mode, or the first frame of any protocol in packet mode.
// Everything else on an unestablished circuit is dropped (the client
// retransmits, and established circuits never punt).
func (c *Component) handleTrigger(pkt *dataplane.ParsedPacket) error {
	var proto models.Protocol
	switch {
	case pkt.Protocol == models.ProtocolL2:
		proto = models.ProtocolL2
	case pkt.DHCPv4 != nil:
		msgType := dhcpv4MessageType(pkt.DHCPv4.Options)
		if msgType != layers.DHCPMsgTypeDiscover && msgType != layers.DHCPMsgTypeRequest {
			return nil
		}
		proto = models.ProtocolDHCPv4
	case pkt.DHCPv6 != nil:
		msgType, _ := dhcpv6TriggerIdentity(pkt)
		if msgType != layers.DHCPv6MsgTypeSolicit &&
			msgType != layers.DHCPv6MsgTypeRequest {
			return nil
		}
		proto = models.ProtocolDHCPv6
	default:
		return nil
	}

	if pkt.OuterVLAN == 0 {
		return fmt.Errorf("untagged trigger not supported")
	}

	match, ok := c.cfgMgr.LookupSubscriberGroup(pkt.OuterVLAN, pkt.InnerVLAN)
	if !ok || !match.Group.HasAccessType(subscriber.AccessTypeL2GW) {
		return nil
	}

	if !c.IsReady() {
		return nil
	}

	portIdx, portName := c.resolvePort(pkt.SwIfIndex)
	key := circuitKey(portIdx, pkt.OuterVLAN, pkt.InnerVLAN)

	if val, exists := c.circuits.Load(key); exists {
		ct := val.(*Circuit)
		ct.mu.Lock()
		state := ct.State
		ct.mu.Unlock()
		switch state {
		case circuitStateInstalled:
			// Punt raced the install; the dataplane path takes over on
			// the next retransmit.
			return nil
		case circuitStateRejected:
			return nil
		default:
			// Auth in flight, hold the newest trigger for replay.
			ct.mu.Lock()
			ct.pendingTrigger = buildPendingTrigger(pkt, proto)
			ct.mu.Unlock()
			return nil
		}
	}

	var srgName string
	if c.srgMgr != nil {
		srgName = c.srgMgr.GetSRGForGroup(match.Name)
		if srgName != "" && !c.srgMgr.IsActive(srgName) {
			return nil
		}
	}

	sessID := session.GenerateID()
	ct := &Circuit{
		SessionID:       sessID,
		AcctSessionID:   session.ToAcctSessionID(sessID),
		MAC:             pkt.MAC.String(),
		AccessInterface: portName,
		AccessIfIndex:   portIdx,
		AccessSVLAN:     pkt.OuterVLAN,
		AccessCVLAN:     pkt.InnerVLAN,
		AccessTPID:      match.Group.GetOuterTPID(),
		SRGName:         srgName,
		State:           circuitStateAuthenticating,
		CreatedAt:       time.Now(),
		Protocol:        string(proto),
		pendingTrigger:  buildPendingTrigger(pkt, proto),
	}

	if _, loaded := c.circuits.LoadOrStore(key, ct); loaded {
		return nil
	}
	c.sessionIndex.Store(sessID, ct)

	c.logger.Info("l2gw circuit trigger",
		"session_id", sessID, "mac", ct.MAC, "svlan", ct.AccessSVLAN,
		"cvlan", ct.AccessCVLAN, "group", match.Name, "protocol", string(proto))

	return c.publishAAARequest(ct, pkt, match)
}

// buildPendingTrigger snapshots the trigger frame's L3 payload for
// replay out the handoff once the circuit installs. The client's own
// MAC is preserved so the retail BNG learns the subscriber, not us.
// Packet-mode triggers are never replayed: whatever protocol the frame
// was, it retransmits on its own once the circuit forwards.
func buildPendingTrigger(pkt *dataplane.ParsedPacket, proto models.Protocol) *pendingTrigger {
	if proto == models.ProtocolL2 {
		return nil
	}
	l3Offset := 14 + 4*len(pkt.Dot1Q)
	if len(pkt.RawPacket) <= l3Offset {
		return nil
	}
	pt := &pendingTrigger{
		protocol:  proto,
		srcMAC:    pkt.MAC,
		l3Payload: append([]byte(nil), pkt.RawPacket[l3Offset:]...),
	}
	if pkt.Ethernet != nil {
		pt.dstMAC = pkt.Ethernet.DstMAC
	}
	return pt
}

func (c *Component) publishAAARequest(ct *Circuit, pkt *dataplane.ParsedPacket, match subscriber.GroupMatch) error {
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil {
		return fmt.Errorf("no running config")
	}

	var circuitID, remoteID, hostname string
	if pkt.DHCPv4 != nil {
		cid, rid := parseOption82(dhcpOption(pkt.DHCPv4.Options, 82))
		circuitID, remoteID = string(cid), string(rid)
		hostname = string(dhcpOption(pkt.DHCPv4.Options, layers.DHCPOptHostname))
	} else if pkt.DHCPv6 != nil {
		if _, info := dhcpv6TriggerIdentity(pkt); info != nil {
			circuitID = string(info.InterfaceID)
			remoteID = string(info.RemoteID)
		}
	}

	// The line identity for wholesale is the group-qualified VLAN
	// tuple, not the client MAC: a CPE swap must not change the
	// subscriber. MAC is recorded on the circuit for show/accounting.
	username := fmt.Sprintf("%s.%d.%d", match.Name, ct.AccessSVLAN, ct.AccessCVLAN)
	policyName := match.Group.GetPolicyName(pkt.OuterVLAN)
	aaaAttrs := make(map[string]string)
	var usernameFallback bool

	if policyName != "" {
		if policy := cfg.AAA.GetPolicyByType(policyName, aaacfg.PolicyTypeDHCP); policy != nil {
			ctx := &aaacfg.PolicyContext{
				MACAddress: pkt.MAC,
				SVLAN:      pkt.OuterVLAN,
				CVLAN:      pkt.InnerVLAN,
				RemoteID:   remoteID,
				CircuitID:  circuitID,
				Hostname:   hostname,
				GroupName:  match.Name,
			}
			if expanded, ok := policy.ExpandFormatChecked(ctx); ok {
				username = expanded
			} else if policy.Format != "" {
				usernameFallback = true
				c.logger.WithGroup(logger.L2GW).Warn("AAA policy username unresolved; using group.svlan.cvlan fallback",
					"policy", policyName, "group", match.Name, "username", username)
			}
			if policy.Password != "" {
				aaaAttrs[aaa.AttrPassword] = policy.ExpandPassword(ctx)
			}
		}
	}

	ct.mu.Lock()
	ct.Username = username
	ct.mu.Unlock()

	if circuitID != "" {
		aaaAttrs[aaa.AttrCircuitID] = circuitID
	}
	if remoteID != "" {
		aaaAttrs[aaa.AttrRemoteID] = remoteID
	}
	if hostname != "" {
		aaaAttrs[aaa.AttrHostname] = hostname
	}

	c.eventBus.Publish(events.TopicAAARequest, events.Event{
		Source: c.Name(),
		Data: &events.AAARequestEvent{
			AccessType: models.AccessTypeL2GW,
			Protocol:   models.Protocol(ct.Protocol),
			SessionID:  ct.SessionID,
			Request: models.AAARequest{
				RequestID:        uuid.New().String(),
				Username:         username,
				MAC:              ct.MAC,
				AcctSessionID:    ct.AcctSessionID,
				SVLAN:            ct.AccessSVLAN,
				CVLAN:            ct.AccessCVLAN,
				Interface:        ct.AccessInterface,
				AccessIfIndex:    ct.AccessIfIndex,
				AccessInterface:  ct.AccessInterface,
				PolicyName:       policyName,
				UsernameFallback: usernameFallback,
				Attributes:       aaaAttrs,
			},
		},
	})
	return nil
}

func (c *Component) handleAAAResponse(event events.Event) {
	data, ok := event.Data.(*events.AAAResponseEvent)
	if !ok {
		c.logger.Error("Invalid event data for l2gw AAA response")
		return
	}

	val, ok := c.sessionIndex.Load(data.SessionID)
	if !ok {
		c.logger.Debug("l2gw AAA response for unknown session", "session_id", data.SessionID)
		return
	}
	ct := val.(*Circuit)

	if !data.Response.Allowed {
		c.logger.Info("l2gw circuit rejected by AAA",
			"session_id", ct.SessionID, "mac", ct.MAC,
			"svlan", ct.AccessSVLAN, "cvlan", ct.AccessCVLAN)
		ct.mu.Lock()
		ct.State = circuitStateRejected
		ct.rejectedAt = time.Now()
		ct.pendingTrigger = nil
		ct.mu.Unlock()
		return
	}

	attrs := make(map[string]string, len(data.Response.Attributes))
	for k, v := range data.Response.Attributes {
		attrs[k] = fmt.Sprintf("%v", v)
	}

	if err := c.resolveAndInstall(ct, attrs); err != nil {
		c.logger.Error("Failed to install l2gw circuit",
			"session_id", ct.SessionID, "error", err)
		ct.mu.Lock()
		ct.State = circuitStateRejected
		ct.rejectedAt = time.Now()
		ct.pendingTrigger = nil
		ct.mu.Unlock()
		return
	}
}

// resolveAndInstall turns an Access-Accept into a programmed circuit:
// handoff group (RADIUS label or group default), egress VLANs (RADIUS
// override or allocator), dataplane install, checkpoint, accounting
// start, trigger replay.
func (c *Component) resolveAndInstall(ct *Circuit, attrs map[string]string) error {
	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil || cfg.L2GW == nil {
		return fmt.Errorf("no l2gw configuration")
	}

	groupName := attrs[aaa.AttrL2GWHandoffGroup]
	if groupName == "" {
		if match, ok := c.cfgMgr.LookupSubscriberGroup(ct.AccessSVLAN, ct.AccessCVLAN); ok &&
			match.Group.L2GW != nil {
			groupName = match.Group.L2GW.HandoffGroup
		}
	}
	if groupName == "" {
		return fmt.Errorf("no handoff group: AAA returned none and subscriber group has no default")
	}

	hg, ok := cfg.L2GW.HandoffGroups[groupName]
	if !ok {
		return fmt.Errorf("unknown handoff-group %q", groupName)
	}
	handoffIdx, ok := c.ifMgr.GetSwIfIndex(hg.Interface)
	if !ok {
		return fmt.Errorf("handoff interface %q not found", hg.Interface)
	}

	var svlan, cvlan uint16
	var allocated bool
	if s, sOK := attrs[aaa.AttrL2GWSVLAN]; sOK {
		sv, err := strconv.ParseUint(s, 10, 16)
		if err != nil || sv == 0 || sv > 4094 {
			return fmt.Errorf("invalid %s %q", aaa.AttrL2GWSVLAN, s)
		}
		svlan = uint16(sv)
		if cs, cOK := attrs[aaa.AttrL2GWCVLAN]; cOK {
			cv, err := strconv.ParseUint(cs, 10, 16)
			if err != nil || cv > 4094 {
				return fmt.Errorf("invalid %s %q", aaa.AttrL2GWCVLAN, cs)
			}
			cvlan = uint16(cv)
		}
		if alloc, aErr := c.getAllocator(groupName, hg); aErr == nil {
			c.allocMu.Lock()
			alloc.mark(svlan, cvlan)
			c.allocMu.Unlock()
		}
	} else {
		alloc, aErr := c.getAllocator(groupName, hg)
		if aErr != nil {
			return aErr
		}
		c.allocMu.Lock()
		svlan, cvlan, err = alloc.allocate()
		c.allocMu.Unlock()
		if err != nil {
			return fmt.Errorf("handoff-group %q: %w", groupName, err)
		}
		allocated = true
	}

	if err := c.armPort(ct.AccessIfIndex, ct.AccessInterface); err != nil {
		return fmt.Errorf("arm access port: %w", err)
	}
	if err := c.armPort(handoffIdx, hg.Interface); err != nil {
		return fmt.Errorf("arm handoff port: %w", err)
	}

	ct.mu.Lock()
	ct.HandoffGroup = groupName
	ct.HandoffInterface = hg.Interface
	ct.HandoffIfIndex = handoffIdx
	ct.HandoffSVLAN = svlan
	ct.HandoffCVLAN = cvlan
	ct.HandoffTPID = hg.GetOuterTPID()
	ct.AllocatedEgress = allocated
	if ct.Attributes == nil {
		ct.Attributes = make(map[string]string)
	}
	for k, v := range attrs {
		ct.Attributes[k] = v
	}
	// Report the resolved egress back through accounting.
	ct.Attributes[aaa.AttrL2GWHandoffGroup] = groupName
	ct.Attributes[aaa.AttrL2GWSVLAN] = strconv.Itoa(int(svlan))
	if cvlan != 0 {
		ct.Attributes[aaa.AttrL2GWCVLAN] = strconv.Itoa(int(cvlan))
	}
	pending := ct.pendingTrigger
	ct.pendingTrigger = nil
	ct.mu.Unlock()

	if err := c.installCircuit(ct); err != nil {
		c.freeEgress(ct)
		return fmt.Errorf("dataplane install: %w", err)
	}

	c.checkpointCircuit(ct)

	c.logger.Info("l2gw circuit installed",
		"session_id", ct.SessionID, "mac", ct.MAC,
		"access", fmt.Sprintf("%s s%d c%d", ct.AccessInterface, ct.AccessSVLAN, ct.AccessCVLAN),
		"handoff", fmt.Sprintf("%s[%s] s%d c%d", groupName, hg.Interface, svlan, cvlan),
		"circuit_id", ct.CircuitID)

	c.eventBus.Publish(events.TopicSessionLifecycle, events.Event{
		Source: c.Name(),
		Data: &events.SessionLifecycleEvent{
			AccessType: models.AccessTypeL2GW,
			Protocol:   models.Protocol(ct.Protocol),
			SessionID:  ct.SessionID,
			State:      models.SessionStateActive,
			Session:    ct.buildSessionModel(models.SessionStateActive),
		},
	})

	c.replayTrigger(ct, pending)
	return nil
}

// replayTrigger re-injects the held trigger frame out the handoff with
// the circuit's egress tags, so the subscriber's first DISCOVER/SOLICIT
// reaches the retail ISP without waiting for a client retransmit.
func (c *Component) replayTrigger(ct *Circuit, pt *pendingTrigger) {
	if pt == nil || pt.l3Payload == nil || pt.dstMAC == nil {
		return
	}

	svlan, cvlan := ct.HandoffSVLAN, ct.HandoffCVLAN
	if ct.Transparent {
		svlan, cvlan = ct.AccessSVLAN, ct.AccessCVLAN
	}

	c.eventBus.Publish(events.TopicEgress, events.Event{
		Source: c.Name(),
		Data: &events.EgressEvent{
			Protocol: pt.protocol,
			Packet: models.EgressPacketPayload{
				RawData:   pt.l3Payload,
				DstMAC:    pt.dstMAC.String(),
				SrcMAC:    pt.srcMAC.String(),
				OuterVLAN: svlan,
				InnerVLAN: cvlan,
				OuterTPID: ct.HandoffTPID,
				SwIfIndex: ct.HandoffIfIndex,
			},
		},
	})
}

func (c *Component) handleSubscriberTerminate(event events.Event) {
	data, ok := event.Data.(*events.SubscriberTerminateEvent)
	if !ok {
		return
	}

	var ct *Circuit
	if data.SessionID != "" {
		if val, ok := c.sessionIndex.Load(data.SessionID); ok {
			ct = val.(*Circuit)
		}
	}
	if ct == nil && data.AcctSessionID != "" {
		c.circuits.Range(func(_, v any) bool {
			cand := v.(*Circuit)
			if cand.AcctSessionID == data.AcctSessionID {
				ct = cand
				return false
			}
			return true
		})
	}
	if ct == nil {
		return
	}

	reason := data.Reason
	if reason == "" {
		reason = "terminate-request"
	}
	c.teardownCircuit(ct, reason)
}

// janitor sweeps rejected circuits past their backoff and pending
// circuits whose AAA never answered, so retransmits can retrigger.
func (c *Component) janitor() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.Ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			c.circuits.Range(func(k, v any) bool {
				ct := v.(*Circuit)
				ct.mu.Lock()
				state := ct.State
				rejectedAt := ct.rejectedAt
				createdAt := ct.CreatedAt
				ct.mu.Unlock()

				switch state {
				case circuitStateRejected:
					if now.Sub(rejectedAt) > rejectBackoff {
						c.circuits.Delete(k)
						c.sessionIndex.Delete(ct.SessionID)
					}
				case circuitStateAuthenticating:
					if !ct.Static && now.Sub(createdAt) > pendingTimeout {
						c.circuits.Delete(k)
						c.sessionIndex.Delete(ct.SessionID)
					}
				}
				return true
			})
		}
	}
}

// dhcpv6TriggerIdentity resolves the effective client message type and,
// for LDRA/relay-encapsulated triggers, the relay identity options
// (interface-id 18, remote-id 37) used for AAA policy usernames.
func dhcpv6TriggerIdentity(pkt *dataplane.ParsedPacket) (layers.DHCPv6MsgType, *dhcp6.RelayInfo) {
	if pkt.DHCPv6.MsgType != layers.DHCPv6MsgTypeRelayForward {
		return pkt.DHCPv6.MsgType, nil
	}
	msg, info := dhcp6.UnwrapRelay(pkt.DHCPv6.Contents)
	if msg == nil {
		return layers.DHCPv6MsgTypeUnspecified, info
	}
	return layers.DHCPv6MsgType(msg.MsgType), info
}

func dhcpv4MessageType(options layers.DHCPOptions) layers.DHCPMsgType {
	for _, opt := range options {
		if opt.Type == layers.DHCPOptMessageType && len(opt.Data) == 1 {
			return layers.DHCPMsgType(opt.Data[0])
		}
	}
	return layers.DHCPMsgTypeUnspecified
}

func dhcpOption(options layers.DHCPOptions, code layers.DHCPOpt) []byte {
	for _, opt := range options {
		if opt.Type == code {
			return opt.Data
		}
	}
	return nil
}

// parseOption82 extracts circuit-id (sub-option 1) and remote-id
// (sub-option 2) from a DHCPv4 relay-agent-information option.
func parseOption82(data []byte) (circuitID, remoteID []byte) {
	for len(data) >= 2 {
		subOpt := data[0]
		subLen := int(data[1])
		if len(data) < 2+subLen {
			break
		}
		val := data[2 : 2+subLen]
		switch subOpt {
		case 1:
			circuitID = val
		case 2:
			remoteID = val
		}
		data = data[2+subLen:]
	}
	return
}
