// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ipoe

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/uuid"
	"github.com/veesix-networks/osvbng/pkg/aaa"
	"github.com/veesix-networks/osvbng/pkg/allocator"
	aaacfg "github.com/veesix-networks/osvbng/pkg/config/aaa"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/dhcp"
	"github.com/veesix-networks/osvbng/pkg/dhcp4"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/session"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func (c *Component) consumeDHCPPackets() {
	for {
		select {
		case <-c.Ctx.Done():
			return
		case pkt := <-c.dhcpChan:
			go func(pkt *dataplane.ParsedPacket) {
				if err := c.processDHCPPacket(pkt); err != nil {
					c.logger.Error("Error processing DHCP packet", "error", err)
				}
			}(pkt)
		}
	}
}

func (c *Component) processDHCPPacket(pkt *dataplane.ParsedPacket) error {

	if pkt.DHCPv4 == nil {
		return fmt.Errorf("no DHCPv4 layer")
	}

	if pkt.OuterVLAN == 0 {
		return fmt.Errorf("packet rejected: S-VLAN required (untagged not supported)")
	}

	if c.srgMgr != nil {
		srgName := c.resolveSRGName(pkt.OuterVLAN, pkt.InnerVLAN)
		if !c.srgMgr.IsActive(srgName) {
			return nil
		}
	}

	msgType := getDHCPMessageType(pkt.DHCPv4.Options)
	if msgType == layers.DHCPMsgTypeUnspecified {
		return fmt.Errorf("missing DHCP message type")
	}

	c.logger.WithGroup(logger.IPoEDHCP4).Debug("[DF] Received DHCP packet",
		"message_type", msgType.String(),
		"mac", pkt.MAC.String(),
		"xid", fmt.Sprintf("0x%x", pkt.DHCPv4.Xid))

	switch msgType {
	case layers.DHCPMsgTypeDiscover:
		return c.handleDiscover(pkt)
	case layers.DHCPMsgTypeRequest:
		return c.handleRequest(pkt)
	case layers.DHCPMsgTypeRelease:
		return c.handleRelease(pkt)
	case layers.DHCPMsgTypeOffer, layers.DHCPMsgTypeAck, layers.DHCPMsgTypeNak:
		return c.handleServerResponse(pkt)
	}

	return nil
}

// dhcpv4Mode reports the configured DHCPv4 mode for the access VLAN's
// subscriber group and the group name. It returns "server" (osvbng is the
// authoritative DHCP server) when no group matches or no profile is bound.
func (c *Component) dhcpv4Mode(svlan, cvlan uint16) (mode, group string) {
	match, ok := c.cfgMgr.LookupSubscriberGroup(svlan, cvlan)
	if !ok {
		return "server", ""
	}
	mode = "server"
	if cfg, err := c.cfgMgr.GetRunning(); err == nil && cfg != nil {
		mode = cfg.IPv4Profiles[match.Group.IPv4Profile].GetMode()
	}
	return mode, match.Name
}

// sessionDHCPv4Mode resolves the authoritative DHCPv4 mode for a session,
// preferring its AAA-resolved profile (which may differ from the group default,
// e.g. RADIUS assigning a relay profile) and falling back to the access VLAN's
// group profile when no profile is resolved yet.
func (c *Component) sessionDHCPv4Mode(sess *SessionState) string {
	if p := c.resolveIPv4Profile(sess.AllocCtx); p != nil {
		return p.GetMode()
	}
	mode, _ := c.dhcpv4Mode(sess.OuterVLAN, sess.InnerVLAN)
	return mode
}

func getDHCPMessageType(options layers.DHCPOptions) layers.DHCPMsgType {
	for _, opt := range options {
		if opt.Type == layers.DHCPOptMessageType {
			if len(opt.Data) == 1 {
				return layers.DHCPMsgType(opt.Data[0])
			}
		}
	}
	return layers.DHCPMsgTypeUnspecified
}

func getDHCPOption(options layers.DHCPOptions, optType layers.DHCPOpt) []byte {
	for _, opt := range options {
		if opt.Type == optType {
			return opt.Data
		}
	}
	return nil
}

func parseOption82(data []byte) (circuitID, remoteID []byte) {
	i := 0
	for i < len(data) {
		if i+1 >= len(data) {
			break
		}

		subOptCode := data[i]
		subOptLen := int(data[i+1])

		if i+2+subOptLen > len(data) {
			break
		}

		subOptData := data[i+2 : i+2+subOptLen]

		switch subOptCode {
		case 1:
			circuitID = subOptData
		case 2:
			remoteID = subOptData
		}

		i += 2 + subOptLen
	}
	return
}

func (c *Component) handleDiscover(pkt *dataplane.ParsedPacket) error {
	lookupKey := c.makeSessionKeyV4(pkt.MAC, pkt.OuterVLAN, pkt.InnerVLAN)

	var sess *SessionState
	if val, ok := c.sessions.Load(lookupKey); ok {
		sess = val.(*SessionState)
	}

	if sess != nil && !c.sessionV4Enabled(sess) {
		ipoeDropFamilyV4.WithLabelValues(sess.GroupName).Inc()
		c.logger.WithGroup(logger.IPoEDHCP4).Debug("DHCPDISCOVER dropped: session v4-disabled",
			"session_id", sess.SessionID, "group", sess.GroupName)
		return nil
	}

	if sess == nil {
		if !c.IsReady() {
			c.logger.WithGroup(logger.IPoEDHCP4).Debug("DHCPDISCOVER dropped: component not ready",
				"mac", pkt.MAC.String(), "svlan", pkt.OuterVLAN, "cvlan", pkt.InnerVLAN,
				"state", c.ReadyState().String())
			return nil
		}
		match, matched := c.cfgMgr.LookupSubscriberGroup(pkt.OuterVLAN, pkt.InnerVLAN)
		if !matched {
			c.logger.WithGroup(logger.IPoEDHCP4).Debug("DHCPDISCOVER dropped: no subscriber-group match",
				"mac", pkt.MAC.String(), "svlan", pkt.OuterVLAN, "cvlan", pkt.InnerVLAN)
			return nil
		}
		if !groupV4Enabled(match.Group) {
			ipoeDropFamilyV4.WithLabelValues(match.Name).Inc()
			c.logger.WithGroup(logger.IPoEDHCP4).Debug("DHCPDISCOVER dropped: group v4-disabled",
				"mac", pkt.MAC.String(), "svlan", pkt.OuterVLAN, "cvlan", pkt.InnerVLAN, "group", match.Name)
			return nil
		}
		if err := c.checkSessionLimit(pkt.MAC, pkt.OuterVLAN, pkt.InnerVLAN); err != nil {
			c.logger.WithGroup(logger.IPoEDHCP4).Debug("DHCPDISCOVER rejected", "error", err)
			return nil
		}

		sessID := session.GenerateID()
		newSess := &SessionState{
			SessionID:     sessID,
			AcctSessionID: session.ToAcctSessionID(sessID),
			MAC:           pkt.MAC,
			OuterVLAN:     pkt.OuterVLAN,
			InnerVLAN:     pkt.InnerVLAN,
			EncapIfIndex:  pkt.SwIfIndex,
			State:         "discovering",
			GroupName:     match.Name,
			MixedAccess:   c.isMixedAccessSVLAN(pkt.OuterVLAN),
		}

		c.sessionIndex.Store(sessID, newSess)
		if actual, loaded := c.sessions.LoadOrStore(lookupKey, newSess); loaded {
			c.sessionIndex.Delete(sessID)
			sess = actual.(*SessionState)
		} else {
			sess = newSess
			c.claimTuple(sess)
		}
	}

	hostname := string(getDHCPOption(pkt.DHCPv4.Options, layers.DHCPOptHostname))
	clientID := getDHCPOption(pkt.DHCPv4.Options, layers.DHCPOptClientID)
	circuitID, remoteID := parseOption82(getDHCPOption(pkt.DHCPv4.Options, 82))

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}
	if err := pkt.DHCPv4.SerializeTo(buf, opts); err != nil {
		return fmt.Errorf("serialize DHCP: %w", err)
	}

	sess.mu.Lock()
	if sess.Closing {
		sess.mu.Unlock()
		return nil
	}
	sess.XID = pkt.DHCPv4.Xid
	sess.Hostname = hostname
	sess.ClientID = clientID
	sess.CircuitID = circuitID
	sess.RemoteID = remoteID
	sess.LastSeen = time.Now()
	sess.EncapIfIndex = pkt.SwIfIndex
	sess.PendingDHCPDiscover = buf.Bytes()
	alreadyApproved := sess.AAAApproved
	ipoeCreated := sess.IPoESessionCreated
	aaaInFlight := sess.AAAInFlight
	if !alreadyApproved && !aaaInFlight {
		sess.AAAInFlight = true
	}
	sess.mu.Unlock()
	c.xidIndex.Store(pkt.DHCPv4.Xid, sess)

	c.logger.WithGroup(logger.IPoEDHCP4).Debug("Session discovering", "session_id", sess.SessionID, "circuit_id", string(circuitID), "remote_id", string(remoteID))

	if alreadyApproved && ipoeCreated {
		c.logger.WithGroup(logger.IPoEDHCP4).Debug("Session already approved, forwarding DISCOVER to provider", "session_id", sess.SessionID)
		v4Profile := c.resolveIPv4Profile(sess.AllocCtx)
		var resolved *dhcp.ResolvedDHCPv4
		if v4Profile == nil || v4Profile.GetMode() == "server" {
			resolved = c.resolveDHCPv4(sess.AllocCtx)
		}
		pkt := &dhcp4.Packet{
			SessionID: sess.SessionID,
			MAC:       sess.MAC.String(),
			SVLAN:     sess.OuterVLAN,
			CVLAN:     sess.InnerVLAN,
			Raw:       buf.Bytes(),
			Resolved:  resolved,
			SwIfIndex: sess.EncapIfIndex,
			Interface: c.resolveAccessInterfaceName(sess.EncapIfIndex),
			Profile:   v4Profile,
			LocalMAC:  c.getLocalMAC(sess.SRGName, sess.EncapIfIndex),
		}
		provider := c.getDHCP4Provider(v4Profile)
		if provider == nil {
			mode := "local"
			if v4Profile != nil {
				mode = v4Profile.GetMode()
			}
			return fmt.Errorf("no DHCPv4 provider for mode %s", mode)
		}
		response, err := provider.HandlePacket(c.Ctx, pkt)
		if err != nil {
			return fmt.Errorf("dhcp provider failed: %w", err)
		}
		if response != nil && len(response.Raw) > 0 {
			return c.sendDHCPResponse(sess.SessionID, sess.OuterVLAN, sess.InnerVLAN, sess.EncapIfIndex, sess.MAC, response.Raw, "OFFER")
		}
		return nil
	}

	if alreadyApproved && !ipoeCreated {
		c.logger.WithGroup(logger.IPoEDHCP4).Debug("DHCP DISCOVER received, AAA approved but IPoE session pending", "session_id", sess.SessionID)
		return nil
	}

	if aaaInFlight {
		c.logger.WithGroup(logger.IPoEDHCP4).Debug("AAA already in flight, skipping duplicate request", "session_id", sess.SessionID)
		return nil
	}

	cfg, _ := c.cfgMgr.GetRunning()
	username := pkt.MAC.String()
	var policyName string
	var groupName string
	var accessInterface string
	if cfg != nil {
		accessInterface, _ = cfg.GetAccessInterface()
		if match, ok := c.cfgMgr.LookupSubscriberGroup(pkt.OuterVLAN, pkt.InnerVLAN); ok {
			groupName = match.Name
			if match.VR != nil && match.VR.AAA != nil && match.VR.AAA.Policy != "" {
				policyName = match.VR.AAA.Policy
			} else {
				policyName = match.Group.AAAPolicy
			}
		}
	}
	aaaAttrs := make(map[string]string)
	var usernameFallback bool
	if policyName != "" {
		if policy := cfg.AAA.GetPolicyByType(policyName, aaacfg.PolicyTypeDHCP); policy != nil {
			ctx := &aaacfg.PolicyContext{
				MACAddress: pkt.MAC,
				SVLAN:      pkt.OuterVLAN,
				CVLAN:      pkt.InnerVLAN,
				RemoteID:   string(remoteID),
				CircuitID:  string(circuitID),
				Hostname:   hostname,
			}
			expanded, ok := policy.ExpandFormatChecked(ctx)
			if ok {
				username = expanded
			} else if policy.Format != "" {
				usernameFallback = true
				c.logger.WithGroup(logger.IPoEDHCP4).Warn("AAA policy username unresolved; using MAC fallback",
					"policy", policyName, "group", groupName, "mac", pkt.MAC.String(),
					"svlan", pkt.OuterVLAN, "cvlan", pkt.InnerVLAN, "format", policy.Format,
					"remote_id", string(remoteID), "circuit_id", string(circuitID), "hostname", hostname)
				aaa.UsernameFallbacks.WithLabelValues(policyName, groupName, "ipoe-dhcpv4").Inc()
			}
			if policy.Password != "" {
				aaaAttrs[aaa.AttrPassword] = policy.ExpandPassword(ctx)
			}
			c.logger.WithGroup(logger.IPoEDHCP4).Debug("Built username from policy", "policy", policyName, "format", policy.Format, "username", username)
		}
	}

	sess.Username = username

	c.logger.WithGroup(logger.IPoEDHCP4).Debug("Publishing AAA request for DISCOVER", "session_id", sess.SessionID, "username", username)
	requestID := uuid.New().String()

	if len(circuitID) > 0 {
		aaaAttrs[aaa.AttrCircuitID] = string(circuitID)
	}
	if len(remoteID) > 0 {
		aaaAttrs[aaa.AttrRemoteID] = string(remoteID)
	}
	if hostname != "" {
		aaaAttrs[aaa.AttrHostname] = hostname
	}

	aaaPayload := &models.AAARequest{
		RequestID:        requestID,
		Username:         username,
		MAC:              pkt.MAC.String(),
		AcctSessionID:    sess.AcctSessionID,
		SVLAN:            pkt.OuterVLAN,
		CVLAN:            pkt.InnerVLAN,
		Interface:        accessInterface,
		AccessIfIndex:    sess.EncapIfIndex,
		AccessInterface:  c.accessInterfaceName(sess.EncapIfIndex),
		PolicyName:       policyName,
		UsernameFallback: usernameFallback,
		Attributes:       aaaAttrs,
	}

	c.eventBus.Publish(events.TopicAAARequest, events.Event{
		Source: c.Name(),
		Data: &events.AAARequestEvent{
			AccessType: models.AccessTypeIPoE,
			Protocol:   models.ProtocolDHCPv4,
			SessionID:  sess.SessionID,
			Request:    *aaaPayload,
		},
	})
	return nil
}

func (c *Component) handleRequest(pkt *dataplane.ParsedPacket) error {
	lookupKey := c.makeSessionKeyV4(pkt.MAC, pkt.OuterVLAN, pkt.InnerVLAN)

	var sess *SessionState
	if val, ok := c.sessions.Load(lookupKey); ok {
		sess = val.(*SessionState)
	}

	if sess != nil && !c.sessionV4Enabled(sess) {
		ipoeDropFamilyV4.WithLabelValues(sess.GroupName).Inc()
		c.logger.WithGroup(logger.IPoEDHCP4).Debug("DHCPREQUEST dropped: session v4-disabled",
			"session_id", sess.SessionID, "group", sess.GroupName)
		return nil
	}

	if sess == nil {
		match, matched := c.cfgMgr.LookupSubscriberGroup(pkt.OuterVLAN, pkt.InnerVLAN)
		if !matched {
			c.logger.WithGroup(logger.IPoEDHCP4).Debug("DHCPREQUEST dropped: no subscriber-group match",
				"mac", pkt.MAC.String(), "svlan", pkt.OuterVLAN, "cvlan", pkt.InnerVLAN)
			return nil
		}
		if !groupV4Enabled(match.Group) {
			ipoeDropFamilyV4.WithLabelValues(match.Name).Inc()
			c.logger.WithGroup(logger.IPoEDHCP4).Debug("DHCPREQUEST dropped: group v4-disabled",
				"mac", pkt.MAC.String(), "svlan", pkt.OuterVLAN, "cvlan", pkt.InnerVLAN, "group", match.Name)
			return nil
		}
		sessID := session.GenerateID()
		newSess := &SessionState{
			SessionID:     sessID,
			AcctSessionID: session.ToAcctSessionID(sessID),
			MAC:           pkt.MAC,
			OuterVLAN:     pkt.OuterVLAN,
			InnerVLAN:     pkt.InnerVLAN,
			EncapIfIndex:  pkt.SwIfIndex,
			State:         "requesting",
			GroupName:     match.Name,
		}
		c.sessionIndex.Store(sessID, newSess)
		if actual, loaded := c.sessions.LoadOrStore(lookupKey, newSess); loaded {
			c.sessionIndex.Delete(sessID)
			sess = actual.(*SessionState)
		} else {
			sess = newSess
			c.claimTuple(sess)
		}
	} else {
		sess.mu.Lock()
		sess.State = "requesting"
		sess.mu.Unlock()
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}
	if err := pkt.DHCPv4.SerializeTo(buf, opts); err != nil {
		return fmt.Errorf("serialize DHCP: %w", err)
	}

	sess.mu.Lock()
	if sess.Closing {
		sess.mu.Unlock()
		return nil
	}
	sess.XID = pkt.DHCPv4.Xid
	sess.LastSeen = time.Now()
	sess.PendingDHCPRequest = buf.Bytes()
	alreadyApproved := sess.AAAApproved
	aaaInFlight := sess.AAAInFlight
	if !alreadyApproved && !aaaInFlight {
		sess.AAAInFlight = true
	}
	sess.mu.Unlock()
	c.xidIndex.Store(pkt.DHCPv4.Xid, sess)

	if alreadyApproved {
		c.logger.WithGroup(logger.IPoEDHCP4).Debug("Session already AAA approved, processing REQUEST with DHCP provider", "session_id", sess.SessionID)

		buf := gopacket.NewSerializeBuffer()
		opts := gopacket.SerializeOptions{
			ComputeChecksums: true,
			FixLengths:       true,
		}
		if err := pkt.DHCPv4.SerializeTo(buf, opts); err != nil {
			return fmt.Errorf("serialize DHCP: %w", err)
		}

		v4Profile := c.resolveIPv4Profile(sess.AllocCtx)
		var resolved *dhcp.ResolvedDHCPv4
		if v4Profile == nil || v4Profile.GetMode() == "server" {
			resolved = c.resolveDHCPv4(sess.AllocCtx)
		}
		dhcpPkt := &dhcp4.Packet{
			SessionID: sess.SessionID,
			MAC:       pkt.MAC.String(),
			SVLAN:     pkt.OuterVLAN,
			CVLAN:     pkt.InnerVLAN,
			Raw:       buf.Bytes(),
			Resolved:  resolved,
			SwIfIndex: sess.EncapIfIndex,
			Interface: c.resolveAccessInterfaceName(sess.EncapIfIndex),
			Profile:   v4Profile,
			LocalMAC:  c.getLocalMAC(sess.SRGName, sess.EncapIfIndex),
		}

		provider := c.getDHCP4Provider(v4Profile)
		if provider == nil {
			mode := "local"
			if v4Profile != nil {
				mode = v4Profile.GetMode()
			}
			return fmt.Errorf("no DHCPv4 provider for mode %s", mode)
		}
		response, err := provider.HandlePacket(c.Ctx, dhcpPkt)
		if err != nil {
			c.logger.WithGroup(logger.IPoEDHCP4).Error("DHCP provider failed for REQUEST", "session_id", sess.SessionID, "error", err)
			return fmt.Errorf("dhcp provider failed: %w", err)
		}

		if response != nil && len(response.Raw) > 0 {
			if err := c.sendDHCPResponse(sess.SessionID, pkt.OuterVLAN, pkt.InnerVLAN, sess.EncapIfIndex, pkt.MAC, response.Raw, "ACK"); err != nil {
				return err
			}

			parsedResponse := &layers.DHCPv4{}
			if err := parsedResponse.DecodeFromBytes(response.Raw[28:], gopacket.NilDecodeFeedback); err == nil {
				msgType := layers.DHCPMsgTypeUnspecified
				for _, opt := range parsedResponse.Options {
					if opt.Type == layers.DHCPOptMessageType && len(opt.Data) == 1 {
						msgType = layers.DHCPMsgType(opt.Data[0])
						break
					}
				}

				if msgType == layers.DHCPMsgTypeAck {
					parsedPkt := &dataplane.ParsedPacket{
						DHCPv4: parsedResponse,
					}
					return c.handleAck(sess, parsedPkt)
				}
			}
		}

		return nil
	}

	if aaaInFlight {
		c.logger.WithGroup(logger.IPoEDHCP4).Debug("AAA already in flight, skipping duplicate request", "session_id", sess.SessionID)
		return nil
	}

	hostname := string(getDHCPOption(pkt.DHCPv4.Options, layers.DHCPOptHostname))
	circuitID, remoteID := parseOption82(getDHCPOption(pkt.DHCPv4.Options, 82))

	cfg, _ := c.cfgMgr.GetRunning()
	username := pkt.MAC.String()
	var policyName string
	var groupName string
	var accessInterface string
	if cfg != nil {
		accessInterface, _ = cfg.GetAccessInterface()
		if match, ok := c.cfgMgr.LookupSubscriberGroup(pkt.OuterVLAN, pkt.InnerVLAN); ok {
			groupName = match.Name
			if match.VR != nil && match.VR.AAA != nil && match.VR.AAA.Policy != "" {
				policyName = match.VR.AAA.Policy
			} else {
				policyName = match.Group.AAAPolicy
			}
		}
	}
	aaaAttrs := make(map[string]string)
	var usernameFallback bool
	if policyName != "" {
		if policy := cfg.AAA.GetPolicyByType(policyName, aaacfg.PolicyTypeDHCP); policy != nil {
			ctx := &aaacfg.PolicyContext{
				MACAddress: pkt.MAC,
				SVLAN:      pkt.OuterVLAN,
				CVLAN:      pkt.InnerVLAN,
				RemoteID:   string(remoteID),
				CircuitID:  string(circuitID),
				Hostname:   hostname,
			}
			expanded, ok := policy.ExpandFormatChecked(ctx)
			if ok {
				username = expanded
			} else if policy.Format != "" {
				usernameFallback = true
				c.logger.WithGroup(logger.IPoEDHCP4).Warn("AAA policy username unresolved; using MAC fallback",
					"policy", policyName, "group", groupName, "mac", pkt.MAC.String(),
					"svlan", pkt.OuterVLAN, "cvlan", pkt.InnerVLAN, "format", policy.Format,
					"remote_id", string(remoteID), "circuit_id", string(circuitID), "hostname", hostname)
				aaa.UsernameFallbacks.WithLabelValues(policyName, groupName, "ipoe-dhcpv4").Inc()
			}
			if policy.Password != "" {
				aaaAttrs[aaa.AttrPassword] = policy.ExpandPassword(ctx)
			}
			c.logger.WithGroup(logger.IPoEDHCP4).Debug("Built username from policy", "policy", policyName, "format", policy.Format, "username", username)
		}
	}

	sess.Username = username

	c.logger.WithGroup(logger.IPoEDHCP4).Debug("Publishing AAA request", "session_id", sess.SessionID, "username", username)
	requestID := uuid.New().String()

	if len(circuitID) > 0 {
		aaaAttrs[aaa.AttrCircuitID] = string(circuitID)
	}
	if len(remoteID) > 0 {
		aaaAttrs[aaa.AttrRemoteID] = string(remoteID)
	}
	if hostname != "" {
		aaaAttrs[aaa.AttrHostname] = hostname
	}

	aaaPayload := &models.AAARequest{
		RequestID:        requestID,
		Username:         username,
		MAC:              pkt.MAC.String(),
		AcctSessionID:    sess.AcctSessionID,
		SVLAN:            pkt.OuterVLAN,
		CVLAN:            pkt.InnerVLAN,
		Interface:        accessInterface,
		AccessIfIndex:    sess.EncapIfIndex,
		AccessInterface:  c.accessInterfaceName(sess.EncapIfIndex),
		PolicyName:       policyName,
		UsernameFallback: usernameFallback,
		Attributes:       aaaAttrs,
	}

	c.eventBus.Publish(events.TopicAAARequest, events.Event{
		Source: c.Name(),
		Data: &events.AAARequestEvent{
			AccessType: models.AccessTypeIPoE,
			Protocol:   models.ProtocolDHCPv4,
			SessionID:  sess.SessionID,
			Request:    *aaaPayload,
		},
	})
	return nil
}

func (c *Component) handleRelease(pkt *dataplane.ParsedPacket) error {
	lookupKey := c.makeSessionKeyV4(pkt.MAC, pkt.OuterVLAN, pkt.InnerVLAN)

	val, ok := c.sessions.Load(lookupKey)
	if !ok {
		c.logger.Debug("Received DHCPRELEASE for unknown session", "mac", pkt.MAC.String(), "svlan", pkt.OuterVLAN, "cvlan", pkt.InnerVLAN)
		return nil
	}
	sess := val.(*SessionState)

	sess.mu.Lock()
	ciaddr := pkt.DHCPv4.ClientIP
	if sess.IPv4 != nil && !sess.IPv4.Equal(ciaddr) {
		sess.mu.Unlock()
		c.logger.Warn("DHCPRELEASE anti-spoof: ciaddr mismatch",
			"session_id", sess.SessionID,
			"expected", sess.IPv4,
			"received", ciaddr,
			"mac", pkt.MAC.String())
		return nil
	}

	sessID := sess.SessionID
	acctSessionID := sess.AcctSessionID
	xid := sess.XID
	ipoeSwIfIndex := sess.IPoESwIfIndex
	ipv4 := sess.IPv4
	mac := sess.MAC
	encapIfIndex := sess.EncapIfIndex
	innerVLAN := sess.InnerVLAN
	ipv6Bound := sess.IPv6Bound
	ipv6Address := sess.IPv6Address
	ipv6Prefix := sess.IPv6Prefix
	v6duid := sess.DHCPv6DUID

	sess.IPv4 = nil
	sess.State = "released"
	sess.mu.Unlock()
	c.xidIndex.Delete(xid)

	sessionMode := c.getSessionMode(pkt.OuterVLAN, pkt.InnerVLAN)
	deleteSession := true
	if sessionMode == subscriber.SessionModeUnified && ipv6Bound {
		deleteSession = false
	}

	if deleteSession {
		sess.mu.Lock()
		sess.Closing = true
		sess.IPv6Bound = false
		sess.IPv6Address = nil
		sess.IPv6Prefix = nil
		dhcpv6XID := sess.DHCPv6XID
		sess.mu.Unlock()
		c.xid6Index.Delete(dhcpv6XID)
		c.sessions.Delete(lookupKey)
		c.sessionIndex.Delete(sessID)
		c.removeSessionFromIndexes(sess)
	}

	c.logger.Debug("IPv4 released by client", "session_id", sessID, "delete_session", deleteSession)

	if ipv4 != nil {
		allocator.GetGlobalRegistry().ReleaseIP(ipv4)
	}
	for _, p := range c.dhcp4Providers {
		p.ReleaseLease(mac.String())
	}
	if deleteSession {
		if ipv6Address != nil {
			allocator.GetGlobalRegistry().ReleaseIANAByIP(ipv6Address)
		}
		if ipv6Prefix != nil {
			allocator.GetGlobalRegistry().ReleasePDByPrefix(ipv6Prefix)
		}
		if v6duid != nil {
			for _, p := range c.dhcp6Providers {
				p.ReleaseLease(v6duid)
			}
		}
	}

	if c.vpp != nil && ipoeSwIfIndex != 0 {
		if ipv4 != nil {
			c.vpp.IPoESetSessionIPv4Async(ipoeSwIfIndex, ipv4, false, func(err error) {
				if err != nil {
					c.logger.Debug("Failed to unbind IPv4 from IPoE session", "session_id", sessID, "error", err)
				}
			})
		}
		if deleteSession {
			if ipv6Address != nil {
				c.vpp.IPoESetSessionIPv6Async(ipoeSwIfIndex, ipv6Address, false, func(err error) {
					if err != nil {
						c.logger.Debug("Failed to unbind IPv6 from IPoE session", "session_id", sessID, "error", err)
					}
				})
			}
			if ipv6Prefix != nil {
				c.vpp.IPoESetDelegatedPrefixAsync(ipoeSwIfIndex, *ipv6Prefix, net.ParseIP("::"), false, func(err error) {
					if err != nil {
						c.logger.Debug("Failed to unbind delegated prefix from IPoE session", "session_id", sessID, "error", err)
					}
				})
			}
			c.vpp.DeleteIPoESessionAsync(mac, encapIfIndex, innerVLAN, func(err error) {
				if err != nil {
					c.logger.Warn("Failed to delete IPoE session", "session_id", sessID, "error", err)
				} else {
					c.logger.Debug("Deleted IPoE session", "session_id", sessID, "sw_if_index", ipoeSwIfIndex)
				}
			})
		}
	}

	if deleteSession {
		counterKey := fmt.Sprintf("osvbng:session_count:%s:%d:%d", pkt.MAC.String(), pkt.OuterVLAN, pkt.InnerVLAN)
		newCount, err := c.cache.Decr(c.Ctx, counterKey)
		if err != nil {
			c.logger.Warn("Failed to decrement session counter", "error", err, "key", counterKey)
		} else if newCount <= 0 {
			c.cache.Delete(c.Ctx, counterKey)
		}
		c.deleteSessionCheckpoint(sessID)
	} else if sess != nil {
		c.checkpointSession(sess)
	}

	return c.publishSessionLifecycle(&models.IPoESession{
		SessionID:    sessID,
		State:        models.SessionStateReleased,
		AccessType:   string(models.AccessTypeIPoE),
		Protocol:     string(models.ProtocolDHCPv4),
		AAASessionID: acctSessionID,
		MAC:          mac,
		OuterVLAN:    pkt.OuterVLAN,
		InnerVLAN:    pkt.InnerVLAN,
		VRF:          sess.VRF,
		SRGName:      sess.SRGName,
		Username:     sess.Username,
		IPv4Address:  ipv4,
		IfIndex:      ipoeSwIfIndex,
	})
}

func (c *Component) handleServerResponse(pkt *dataplane.ParsedPacket) error {
	val, ok := c.xidIndex.Load(pkt.DHCPv4.Xid)
	if !ok {
		msgType := getDHCPMessageType(pkt.DHCPv4.Options)
		c.logger.Debug("Received DHCP response but no session found", "message_type", msgType.String(), "xid", fmt.Sprintf("0x%x", pkt.DHCPv4.Xid))
		return nil
	}
	sess := val.(*SessionState)

	msgType := getDHCPMessageType(pkt.DHCPv4.Options)

	if mode := c.sessionDHCPv4Mode(sess); mode != "relay" && mode != "proxy" {
		ipoeDropForeignServer.WithLabelValues(sess.GroupName, msgType.String()).Inc()
		c.logger.WithGroup(logger.IPoEDHCP4).Warn("Dropping server-sourced DHCPv4 in server mode (foreign DHCP server)",
			"message_type", msgType.String(),
			"session_id", sess.SessionID,
			"client_mac", sess.MAC.String(),
			"svlan", sess.OuterVLAN,
			"cvlan", sess.InnerVLAN,
			"offered_ip", pkt.DHCPv4.YourClientIP.String())
		return nil
	}

	c.logger.Debug("Forwarding DHCP to client", "message_type", msgType.String(), "mac", sess.MAC.String(), "session_id", sess.SessionID, "xid", fmt.Sprintf("0x%x", pkt.DHCPv4.Xid))

	var vmac net.HardwareAddr
	var parentSwIfIndex uint32
	if c.srgMgr != nil {
		vmac = c.srgMgr.GetVirtualMAC(sess.SRGName)
	}
	if c.ifMgr != nil {
		if iface := c.ifMgr.Get(sess.EncapIfIndex); iface != nil {
			parentSwIfIndex = iface.SupSwIfIndex
		}
		if vmac == nil {
			if parent := c.ifMgr.Get(parentSwIfIndex); parent != nil && len(parent.MAC) >= 6 {
				vmac = net.HardwareAddr(parent.MAC[:6])
			}
		}
	}
	if vmac == nil {
		return fmt.Errorf("no virtual MAC for S-VLAN %d", sess.OuterVLAN)
	}

	modifiedDHCP := *pkt.DHCPv4
	modifiedDHCP.RelayAgentIP = net.IPv4zero

	broadcast := (pkt.DHCPv4.Flags & 0x8000) != 0
	dstIP := pkt.DHCPv4.YourClientIP
	if broadcast || dstIP.IsUnspecified() {
		dstIP = net.IPv4bcast
	}

	udpLayer := &layers.UDP{
		SrcPort: 67,
		DstPort: 68,
	}
	udpLayer.SetNetworkLayerForChecksum(&layers.IPv4{
		SrcIP: net.IPv4zero,
		DstIP: dstIP,
	})

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}

	if err := gopacket.SerializeLayers(buf, opts, &modifiedDHCP, udpLayer); err != nil {
		return fmt.Errorf("serialize DHCP/UDP: %w", err)
	}

	ipLayer := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: layers.IPProtocolUDP,
		SrcIP:    net.IPv4zero,
		DstIP:    dstIP,
	}

	finalBuf := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(finalBuf, opts, ipLayer, udpLayer, &modifiedDHCP); err != nil {
		return fmt.Errorf("serialize IP/UDP/DHCP: %w", err)
	}

	dstMAC := sess.MAC.String()
	if broadcast {
		dstMAC = "ff:ff:ff:ff:ff:ff"
	}

	egressPayload := &models.EgressPacketPayload{
		DstMAC:    dstMAC,
		SrcMAC:    vmac.String(),
		OuterVLAN: sess.OuterVLAN,
		InnerVLAN: sess.InnerVLAN,
		OuterTPID: c.ifMgr.OuterTPID(sess.EncapIfIndex),
		SwIfIndex: parentSwIfIndex,
		RawData:   finalBuf.Bytes(),
	}

	c.logger.Debug("Sending DHCP via egress", "message_type", msgType.String(), "dst_mac", dstMAC, "svlan", sess.OuterVLAN, "cvlan", sess.InnerVLAN)

	c.eventBus.Publish(events.TopicEgress, events.Event{
		Source: c.Name(),
		Data: &events.EgressEvent{
			Protocol: models.ProtocolDHCPv4,
			Packet:   *egressPayload,
		},
	})

	if msgType == layers.DHCPMsgTypeAck {
		return c.handleAck(sess, pkt)
	}

	return nil
}

func (c *Component) handleAck(sess *SessionState, pkt *dataplane.ParsedPacket) error {
	leaseTime := uint32(0)
	if leaseOpt := getDHCPOption(pkt.DHCPv4.Options, 51); len(leaseOpt) == 4 {
		leaseTime = binary.BigEndian.Uint32(leaseOpt)
	}

	sess.mu.Lock()
	alreadyBound := sess.State == "bound"
	sess.State = "bound"
	sess.IPv4 = pkt.DHCPv4.YourClientIP
	sess.LeaseTime = leaseTime
	sess.BoundAt = time.Now()
	if sess.ActivatedAt.IsZero() {
		sess.ActivatedAt = sess.BoundAt
	}
	mac := sess.MAC
	svlan := sess.OuterVLAN
	cvlan := sess.InnerVLAN
	ipoeSwIfIndex := sess.IPoESwIfIndex
	snapshotIPv6 := sess.IPv6Address
	snapshotIPv6LeaseTime := sess.IPv6LeaseTime
	snapshotIPv6Prefix := sess.IPv6Prefix
	sess.mu.Unlock()

	c.logger.WithGroup(logger.IPoEDHCP4).Debug("Session bound", "session_id", sess.SessionID, "ipv4", sess.IPv4.String())

	if c.vpp != nil {
		sessID := sess.SessionID
		ipv4 := sess.IPv4
		if ipoeSwIfIndex != 0 {
			c.vpp.IPoESetSessionIPv4Async(ipoeSwIfIndex, ipv4, true, func(err error) {
				if err != nil {
					if errors.Is(err, southbound.ErrUnavailable) {
						c.logger.WithGroup(logger.IPoEDHCP4).Debug("VPP unavailable, cannot bind IPv4", "session_id", sessID)
					} else {
						c.logger.WithGroup(logger.IPoEDHCP4).Error("Failed to bind IPv4 to IPoE session", "session_id", sessID, "error", err)
					}
					return
				}
				c.logger.WithGroup(logger.IPoEDHCP4).Debug("Bound IPv4 to IPoE session", "session_id", sessID, "sw_if_index", ipoeSwIfIndex, "ipv4", ipv4.String())
				c.publishSessionProgrammed(sess, ipoeSwIfIndex)
			})
		} else {
			sess.mu.Lock()
			sess.PendingIPv4Binding = ipv4
			sess.mu.Unlock()
			c.logger.WithGroup(logger.IPoEDHCP4).Debug("IPoE session not ready, queued IPv4 binding", "session_id", sessID, "ipv4", ipv4.String())
		}
	}

	counterKey := fmt.Sprintf("osvbng:session_count:%s:%d:%d", mac.String(), svlan, cvlan)
	if !alreadyBound {
		if _, err := c.cache.Incr(c.Ctx, counterKey); err != nil {
			c.logger.Warn("Failed to increment session counter", "error", err, "key", counterKey)
		}
	}
	expiry := time.Duration(sess.LeaseTime*2) * time.Second
	if expiry == 0 || expiry > 24*time.Hour {
		expiry = 24 * time.Hour
	}
	c.cache.Expire(c.Ctx, counterKey, expiry)

	c.checkpointSession(sess)

	c.logger.Debug("Publishing session lifecycle event", "session_id", sess.SessionID, "sw_if_index", ipoeSwIfIndex, "ipv4", sess.IPv4.String())

	ipoeSess := &models.IPoESession{
		SessionID:       sess.SessionID,
		State:           models.SessionStateActive,
		AccessType:      string(models.AccessTypeIPoE),
		Protocol:        string(models.ProtocolDHCPv4),
		MAC:             sess.MAC,
		OuterVLAN:       sess.OuterVLAN,
		InnerVLAN:       sess.InnerVLAN,
		VLANCount:       c.getVLANCount(sess.OuterVLAN, sess.InnerVLAN),
		IfIndex:         ipoeSwIfIndex,
		AccessIfIndex:   sess.EncapIfIndex,
		AccessInterface: c.accessInterfaceName(sess.EncapIfIndex),
		VRF:             sess.VRF,
		ServiceGroup:    sess.ServiceGroup.Name,
		SRGName:         sess.SRGName,
		IPv4Address:     sess.IPv4,
		LeaseTime:       sess.LeaseTime,
		IPv6Address:     snapshotIPv6,
		IPv6LeaseTime:   snapshotIPv6LeaseTime,
		DUID:            sess.DHCPv6DUID,
		Username:        sess.Username,
		AAASessionID:    sess.AcctSessionID,
		ActivatedAt:     sess.ActivatedAt,
	}
	if sess.AllocCtx != nil {
		ipoeSess.IPv4Pool = sess.AllocCtx.AllocatedPool
		ipoeSess.IANAPool = sess.AllocCtx.AllocatedIANAPool
		ipoeSess.PDPool = sess.AllocCtx.AllocatedPDPool
	}
	if snapshotIPv6Prefix != nil {
		ipoeSess.IPv6Prefix = snapshotIPv6Prefix.String()
	}

	return c.publishSessionLifecycle(ipoeSess)
}

func (c *Component) sendDHCPResponse(sessID string, svlan, cvlan uint16, encapIfIndex uint32, mac net.HardwareAddr, rawData []byte, msgType string) error {
	var srcMAC string
	var parentSwIfIndex uint32
	var srgName string

	if val, ok := c.sessionIndex.Load(sessID); ok {
		sess := val.(*SessionState)
		sess.mu.Lock()
		srgName = sess.SRGName
		sess.mu.Unlock()
	}

	outerTPID := c.ifMgr.OuterTPID(encapIfIndex)

	if c.srgMgr != nil {
		if vmac := c.srgMgr.GetVirtualMAC(srgName); vmac != nil {
			srcMAC = vmac.String()
		}
	}
	if c.ifMgr != nil {
		if iface := c.ifMgr.Get(encapIfIndex); iface != nil {
			parentSwIfIndex = iface.SupSwIfIndex
		}
		if srcMAC == "" {
			if parent := c.ifMgr.Get(parentSwIfIndex); parent != nil && len(parent.MAC) >= 6 {
				srcMAC = net.HardwareAddr(parent.MAC[:6]).String()
			}
		}
	}

	egressPayload := &models.EgressPacketPayload{
		DstMAC:    mac.String(),
		SrcMAC:    srcMAC,
		OuterVLAN: svlan,
		InnerVLAN: cvlan,
		OuterTPID: outerTPID,
		SwIfIndex: parentSwIfIndex,
		RawData:   rawData,
	}

	c.logger.Debug("Sending DHCP "+msgType+" to client", "session_id", sessID, "size", len(rawData))

	c.eventBus.Publish(events.TopicEgress, events.Event{
		Source: c.Name(),
		Data: &events.EgressEvent{
			Protocol: models.ProtocolDHCPv4,
			Packet:   *egressPayload,
		},
	})

	return nil
}
