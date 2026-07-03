// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ipoe

import (
	"fmt"
	"hash/fnv"
	"net"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/veesix-networks/osvbng/internal/ra"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

// raEgressTarget carries the per-advertisement addressing for buildAndPublishRA.
// L2 and L3 destinations are independent so a multicast RA (dstIP ff02::1) uses
// the all-nodes multicast MAC rather than a unicast subscriber MAC.
type raEgressTarget struct {
	dstMAC          net.HardwareAddr
	srcMAC          net.HardwareAddr
	dstIP           net.IP
	srcIP           net.IP
	outerVLAN       uint16
	innerVLAN       uint16
	outerTPID       uint16
	parentSwIfIndex uint32
}

func (c *Component) consumeIPv6NDPackets() {
	if c.ipv6NDChan == nil {
		c.logger.Debug("IPv6 ND channel not configured, skipping IPv6 ND consumer")
		return
	}

	for {
		select {
		case <-c.Ctx.Done():
			return
		case pkt := <-c.ipv6NDChan:
			go func(pkt *dataplane.ParsedPacket) {
				if pkt.ICMPv6 == nil {
					return
				}
				switch pkt.ICMPv6.TypeCode.Type() {
				case layers.ICMPv6TypeRouterSolicitation:
					if err := c.processRSPacket(pkt); err != nil {
						c.logger.Error("Error processing RS packet", "error", err)
					}
				case layers.ICMPv6TypeNeighborSolicitation:
					if err := c.processNSPacket(pkt); err != nil {
						c.logger.Error("Error processing NS packet", "error", err)
					}
				}
			}(pkt)
		}
	}
}

func (c *Component) processRSPacket(pkt *dataplane.ParsedPacket) error {
	if pkt.ICMPv6 == nil {
		return fmt.Errorf("no ICMPv6 layer")
	}

	if pkt.ICMPv6.TypeCode.Type() != layers.ICMPv6TypeRouterSolicitation {
		return nil
	}

	if pkt.IPv6 == nil {
		return fmt.Errorf("no IPv6 layer")
	}

	if pkt.OuterVLAN == 0 {
		return fmt.Errorf("packet rejected: S-VLAN required")
	}

	if c.srgMgr != nil {
		srgName := c.resolveSRGName(pkt.OuterVLAN, pkt.InnerVLAN)
		if !c.srgMgr.IsActive(srgName) {
			return nil
		}
	}

	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil {
		return fmt.Errorf("no running config available")
	}

	match, matched := c.cfgMgr.LookupSubscriberGroup(pkt.OuterVLAN, pkt.InnerVLAN)
	if !matched {
		return nil
	}
	group := match.Group

	if !groupV6Enabled(group) {
		ndDropFamily.WithLabelValues(match.Name, "rs").Inc()
		return nil
	}

	raConfig, prefixes := ra.ResolveGroupRA(cfg, group)

	c.logger.Debug("Processing RS packet",
		"mac", pkt.MAC.String(),
		"svlan", pkt.OuterVLAN,
		"cvlan", pkt.InnerVLAN,
		"src_ip", pkt.IPv6.SrcIP,
		"managed", raConfig.Managed,
		"other", raConfig.Other,
		"prefixes", len(prefixes),
	)

	return c.sendRAResponse(pkt, raConfig, prefixes)
}

const (
	raTickInterval   = time.Second
	raMinBucketCount = 10
	raMaxBucketCount = 600
)

// computeRABucketCount sizes the emitter wheel so a session is visited within the
// shortest resolved refresh interval across RA-advertising groups, keeping the
// per-tick batch bounded (one bucket walked per second).
func (c *Component) computeRABucketCount(cfg *config.Config) int {
	minRefresh := raMaxBucketCount
	if cfg != nil && cfg.SubscriberGroups != nil {
		for _, g := range cfg.SubscriberGroups.Groups {
			if !groupV6Enabled(g) {
				continue
			}
			rc, _ := ra.ResolveGroupRA(cfg, g)
			if ri := int(ra.RefreshInterval(rc) / time.Second); ri > 0 && ri < minRefresh {
				minRefresh = ri
			}
		}
	}
	if minRefresh < raMinBucketCount {
		minRefresh = raMinBucketCount
	}
	if minRefresh > raMaxBucketCount {
		minRefresh = raMaxBucketCount
	}
	return minRefresh
}

func (c *Component) raBucketOf(sessionID string) int {
	if c.raBucketCount <= 0 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(sessionID))
	return int(h.Sum32() % uint32(c.raBucketCount))
}

func (c *Component) placeSessionInRABucket(sess *SessionState) {
	if c.raBucketCount <= 0 || sess.SessionID == "" {
		return
	}
	b := c.raBucketOf(sess.SessionID)
	c.raBucketMu.Lock()
	c.raBuckets[b] = append(c.raBuckets[b], sess.SessionID)
	c.raBucketMu.Unlock()
}

func (c *Component) removeSessionFromRABucket(sess *SessionState) {
	if c.raBucketCount <= 0 || sess.SessionID == "" {
		return
	}
	b := c.raBucketOf(sess.SessionID)
	c.raBucketMu.Lock()
	ids := c.raBuckets[b]
	for i, id := range ids {
		if id == sess.SessionID {
			c.raBuckets[b] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	c.raBucketMu.Unlock()
}

// periodicRAEmitter sends unsolicited RAs so subscriber default routes are refreshed
// before they expire (RFC 4861 §6.2). One goroutine walks one bucket per tick;
// sessions are hashed into buckets on add/remove, not re-scanned per tick.
func (c *Component) periodicRAEmitter() {
	if c.raBucketCount <= 0 {
		return
	}
	ticker := time.NewTicker(raTickInterval)
	defer ticker.Stop()

	bucket := 0
	for {
		select {
		case <-c.Ctx.Done():
			return
		case <-ticker.C:
			c.emitRABucket(bucket)
			bucket = (bucket + 1) % c.raBucketCount
		}
	}
}

func (c *Component) emitRABucket(bucket int) {
	c.raBucketMu.RLock()
	ids := append([]string(nil), c.raBuckets[bucket]...)
	c.raBucketMu.RUnlock()
	if len(ids) == 0 {
		return
	}

	cfg, err := c.cfgMgr.GetRunning()
	if err != nil || cfg == nil {
		return
	}

	now := time.Now()
	for _, id := range ids {
		if v, ok := c.sessionIndex.Load(id); ok {
			c.emitPeriodicRA(v.(*SessionState), cfg, now)
		}
	}
}

func (c *Component) emitPeriodicRA(sess *SessionState, cfg *config.Config, now time.Time) {
	sess.mu.Lock()
	closing := sess.Closing
	v6bound := sess.IPv6Bound
	svlan := sess.OuterVLAN
	cvlan := sess.InnerVLAN
	encapIfIndex := sess.EncapIfIndex
	mac := sess.MAC
	clientLL := sess.ClientLinkLocal
	due := sess.nextRADue
	sess.mu.Unlock()

	if closing || !v6bound {
		return
	}

	srgName := c.resolveSRGName(svlan, cvlan)
	if c.srgMgr != nil && !c.srgMgr.IsActive(srgName) {
		return
	}

	match, matched := c.cfgMgr.LookupSubscriberGroup(svlan, cvlan)
	if !matched || !groupV6Enabled(match.Group) {
		// group v6 was turned off while this session was advertising: send one final
		// RA with Router Lifetime 0 so the host drops its default route now instead of
		// waiting out the lifetime (RFC 4861 §6.2.5). due!=zero means it was advertising.
		if !due.IsZero() {
			c.ceaseSessionRA(srgName, encapIfIndex, svlan, cvlan)
			sess.mu.Lock()
			sess.nextRADue = time.Time{}
			sess.mu.Unlock()
		}
		return
	}

	var parentSwIfIndex uint32
	var outerTPID uint16
	if c.ifMgr != nil {
		if iface := c.ifMgr.Get(encapIfIndex); iface != nil {
			parentSwIfIndex = iface.SupSwIfIndex
		}
		outerTPID = c.ifMgr.OuterTPID(encapIfIndex)
	}

	// All config-derived work (resolve RA config, parse prefixes, serialize the RA
	// template, derive the source MAC, the unicast policy and the refresh interval) is
	// done once per (group x SRG x parent) and reused for every subscriber until the
	// running config changes — keyed on the config pointer, which GetRunning swaps only
	// on commit. Per subscriber this is just a map lookup plus a copy-and-patch.
	key := fmt.Sprintf("%s|%s|%d", match.Name, srgName, parentSwIfIndex)
	st := c.raEngine.GroupStateFor(key, cfg, match.Group, c.getLocalMAC(srgName, parentSwIfIndex))
	if st == nil {
		return
	}
	if !due.IsZero() && now.Before(due) {
		return
	}

	dstMAC := mac
	dstIP := clientLL
	if !st.Unicast || len(dstIP) == 0 || dstIP.IsUnspecified() {
		// multicast: the group opted out of unicast RAs, or the client link-local
		// isn't known yet
		dstMAC = net.HardwareAddr{0x33, 0x33, 0x00, 0x00, 0x00, 0x01}
		dstIP = net.ParseIP("ff02::1")
	}

	raw := make([]byte, len(st.RawData))
	copy(raw, st.RawData)
	copy(raw[24:40], dstIP.To16())
	ra.PatchChecksum(raw)

	c.publishRA(raw, raEgressTarget{
		dstMAC:          dstMAC,
		srcMAC:          st.SrcMAC,
		outerVLAN:       svlan,
		innerVLAN:       cvlan,
		outerTPID:       outerTPID,
		parentSwIfIndex: parentSwIfIndex,
	})

	sess.mu.Lock()
	sess.nextRADue = now.Add(st.Refresh)
	sess.mu.Unlock()
}

// ceaseSessionRA sends a single multicast RA with Router Lifetime 0 so a subscriber
// drops its default route immediately when the BNG stops being its default router
// (RFC 4861 §6.2.5). Used only on a genuine cessation (group v6 disabled), never on a
// transient osvbngd restart — that path keeps the route alive via opdb restore.
func (c *Component) ceaseSessionRA(srgName string, encapIfIndex uint32, svlan, cvlan uint16) {
	var parentSwIfIndex uint32
	if c.ifMgr != nil {
		if iface := c.ifMgr.Get(encapIfIndex); iface != nil {
			parentSwIfIndex = iface.SupSwIfIndex
		}
	}
	srcMAC := c.getLocalMAC(srgName, parentSwIfIndex)
	if srcMAC == nil {
		return
	}
	srcIP := ra.LinkLocalFromMAC(srcMAC)
	if srcIP == nil {
		return
	}
	var outerTPID uint16
	if c.ifMgr != nil {
		outerTPID = c.ifMgr.OuterTPID(encapIfIndex)
	}
	_ = c.buildAndPublishRA(raEgressTarget{
		dstMAC:          net.HardwareAddr{0x33, 0x33, 0x00, 0x00, 0x00, 0x01},
		srcMAC:          srcMAC,
		dstIP:           net.ParseIP("ff02::1"),
		srcIP:           srcIP,
		outerVLAN:       svlan,
		innerVLAN:       cvlan,
		outerTPID:       outerTPID,
		parentSwIfIndex: parentSwIfIndex,
	}, southbound.IPv6RAConfig{Managed: true, Other: true, RouterLifetime: 0}, nil)
}

func (c *Component) sendRAResponse(pkt *dataplane.ParsedPacket, raConfig southbound.IPv6RAConfig, prefixes []ra.PrefixInfo) error {
	var parentSwIfIndex uint32
	if c.ifMgr != nil {
		if iface := c.ifMgr.Get(pkt.SwIfIndex); iface != nil {
			parentSwIfIndex = iface.SupSwIfIndex
		}
	}

	srgName := c.resolveSRGName(pkt.OuterVLAN, pkt.InnerVLAN)
	srcMAC := c.getLocalMAC(srgName, parentSwIfIndex)
	if srcMAC == nil {
		return fmt.Errorf("no source MAC available")
	}

	srcIP := ra.LinkLocalFromMAC(srcMAC)
	if srcIP == nil {
		return fmt.Errorf("no IPv6 source address available for S-VLAN %d", pkt.OuterVLAN)
	}

	dstIP := pkt.IPv6.SrcIP
	if dstIP.IsUnspecified() {
		dstIP = net.ParseIP("ff02::1")
	}

	var outerTPID uint16
	if c.ifMgr != nil {
		outerTPID = c.ifMgr.OuterTPID(pkt.SwIfIndex)
	}

	return c.buildAndPublishRA(raEgressTarget{
		dstMAC:          pkt.MAC,
		srcMAC:          srcMAC,
		dstIP:           dstIP,
		srcIP:           srcIP,
		outerVLAN:       pkt.OuterVLAN,
		innerVLAN:       pkt.InnerVLAN,
		outerTPID:       outerTPID,
		parentSwIfIndex: parentSwIfIndex,
	}, raConfig, prefixes)
}

func (c *Component) buildAndPublishRA(t raEgressTarget, raConfig southbound.IPv6RAConfig, prefixes []ra.PrefixInfo) error {
	raw, err := ra.BuildRARawData(raConfig, prefixes, t.srcMAC, t.srcIP, t.dstIP, true, c.logger)
	if err != nil {
		return err
	}
	c.publishRA(raw, t)
	return nil
}

func (c *Component) publishRA(rawData []byte, t raEgressTarget) {
	c.eventBus.Publish(events.TopicEgress, events.Event{
		Source: c.Name(),
		Data: &events.EgressEvent{
			Protocol: models.ProtocolIPv6ND,
			Packet: models.EgressPacketPayload{
				DstMAC:    t.dstMAC.String(),
				SrcMAC:    t.srcMAC.String(),
				OuterVLAN: t.outerVLAN,
				InnerVLAN: t.innerVLAN,
				OuterTPID: t.outerTPID,
				SwIfIndex: t.parentSwIfIndex,
				RawData:   rawData,
			},
		},
	})
}

func (c *Component) processNSPacket(pkt *dataplane.ParsedPacket) error {
	if pkt.ICMPv6 == nil {
		return fmt.Errorf("no ICMPv6 layer")
	}
	if pkt.ICMPv6.TypeCode.Type() != layers.ICMPv6TypeNeighborSolicitation {
		return nil
	}
	if pkt.IPv6 == nil {
		return fmt.Errorf("no IPv6 layer")
	}
	if pkt.OuterVLAN == 0 {
		return fmt.Errorf("packet rejected: S-VLAN required")
	}

	match, ok := c.cfgMgr.LookupSubscriberGroup(pkt.OuterVLAN, pkt.InnerVLAN)
	if !ok {
		return nil
	}
	if !groupV6Enabled(match.Group) {
		ndDropFamily.WithLabelValues(match.Name, "ns").Inc()
		return nil
	}

	body := pkt.ICMPv6.LayerPayload()
	if len(body) < 20 {
		return fmt.Errorf("NS body too short: %d bytes", len(body))
	}
	target := net.IP(body[4:20])

	srgName := c.resolveSRGName(pkt.OuterVLAN, pkt.InnerVLAN)
	if c.srgMgr != nil && !c.srgMgr.IsActive(srgName) {
		return nil
	}

	var parentSwIfIndex uint32
	if c.ifMgr != nil {
		if iface := c.ifMgr.Get(pkt.SwIfIndex); iface != nil {
			parentSwIfIndex = iface.SupSwIfIndex
		}
	}

	localMAC := c.getLocalMAC(srgName, parentSwIfIndex)
	if localMAC == nil {
		return fmt.Errorf("no source MAC available for NS reply")
	}

	expected := ra.LinkLocalFromMAC(localMAC)
	if expected == nil {
		return fmt.Errorf("no IPv6 source address for NS reply S-VLAN %d", pkt.OuterVLAN)
	}

	if !target.Equal(expected) {
		return nil
	}

	return c.sendNAResponse(pkt, parentSwIfIndex, localMAC, expected)
}

func (c *Component) sendNAResponse(pkt *dataplane.ParsedPacket, parentSwIfIndex uint32, localMAC net.HardwareAddr, srcIP net.IP) error {
	dstIP := pkt.IPv6.SrcIP
	solicited := !dstIP.IsUnspecified()
	if !solicited {
		dstIP = net.ParseIP("ff02::1")
	}

	var naFlags uint8 = 0x80 | 0x20
	if solicited {
		naFlags |= 0x40
	}

	naOptions := layers.ICMPv6Options{
		{
			Type: layers.ICMPv6OptTargetAddress,
			Data: localMAC,
		},
	}

	naLayer := &layers.ICMPv6NeighborAdvertisement{
		Flags:         naFlags,
		TargetAddress: srcIP,
		Options:       naOptions,
	}

	icmpv6Layer := &layers.ICMPv6{
		TypeCode: layers.CreateICMPv6TypeCode(layers.ICMPv6TypeNeighborAdvertisement, 0),
	}

	ipv6Layer := &layers.IPv6{
		Version:    6,
		HopLimit:   255,
		NextHeader: layers.IPProtocolICMPv6,
		SrcIP:      srcIP,
		DstIP:      dstIP,
	}
	icmpv6Layer.SetNetworkLayerForChecksum(ipv6Layer)

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}
	if err := gopacket.SerializeLayers(buf, opts, ipv6Layer, icmpv6Layer, naLayer); err != nil {
		return fmt.Errorf("serialize NA: %w", err)
	}

	egressPayload := &models.EgressPacketPayload{
		DstMAC:    pkt.MAC.String(),
		SrcMAC:    localMAC.String(),
		OuterVLAN: pkt.OuterVLAN,
		InnerVLAN: pkt.InnerVLAN,
		OuterTPID: c.ifMgr.OuterTPID(pkt.SwIfIndex),
		SwIfIndex: parentSwIfIndex,
		RawData:   buf.Bytes(),
	}

	c.logger.Debug("Sending NA response",
		"dst_mac", pkt.MAC.String(),
		"src_mac", localMAC.String(),
		"svlan", pkt.OuterVLAN,
		"cvlan", pkt.InnerVLAN,
		"solicited", solicited,
	)

	c.eventBus.Publish(events.TopicEgress, events.Event{
		Source: c.Name(),
		Data: &events.EgressEvent{
			Protocol: models.ProtocolIPv6ND,
			Packet:   *egressPayload,
		},
	})
	return nil
}
