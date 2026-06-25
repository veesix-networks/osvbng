// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

// Package ra builds IPv6 Router Advertisements and Neighbor Advertisements and
// caches the per-group RA template for replication. It is encap-agnostic: the
// IPoE and PPPoE components share the byte builders, config resolution and the
// template cache, and supply their own ingress, egress framing and source
// identity. The serialized RawData carries the IPv6 + ICMPv6 payload only; the
// Ethernet/QinQ or PPP frame is assembled downstream by the caller.
package ra

import (
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

// PrefixInfo is one Prefix Information Option to advertise.
type PrefixInfo struct {
	Network       string
	ValidTime     uint32
	PreferredTime uint32
	OnLink        bool
}

func LinkLocalFromMAC(mac net.HardwareAddr) net.IP {
	if len(mac) < 6 {
		return nil
	}
	addr := make(net.IP, 16)
	addr[0] = 0xfe
	addr[1] = 0x80
	addr[8] = mac[0] ^ 0x02
	addr[9] = mac[1]
	addr[10] = mac[2]
	addr[11] = 0xff
	addr[12] = 0xfe
	addr[13] = mac[3]
	addr[14] = mac[4]
	addr[15] = mac[5]
	return addr
}

// ResolveGroupRA walks the running config for a group's effective RA parameters
// and the prefixes to advertise (global dhcpv6.ra default, per-group override).
func ResolveGroupRA(cfg *config.Config, group *subscriber.SubscriberGroup) (southbound.IPv6RAConfig, []PrefixInfo) {
	raConfig := southbound.IPv6RAConfig{
		Managed:        true,
		Other:          true,
		RouterLifetime: 1800,
		MaxInterval:    600,
		MinInterval:    200,
	}

	if cfg.DHCPv6.RA != nil {
		raConfig.Managed = cfg.DHCPv6.RA.GetManaged()
		raConfig.Other = cfg.DHCPv6.RA.GetOther()
		raConfig.RouterLifetime = cfg.DHCPv6.RA.GetRouterLifetime()
		raConfig.MaxInterval = cfg.DHCPv6.RA.GetMaxInterval()
		raConfig.MinInterval = cfg.DHCPv6.RA.GetMinInterval()
	}

	if group != nil && group.IPv6 != nil && group.IPv6.RA != nil {
		groupRA := group.IPv6.RA
		if groupRA.Managed != nil {
			raConfig.Managed = *groupRA.Managed
		}
		if groupRA.Other != nil {
			raConfig.Other = *groupRA.Other
		}
		if groupRA.RouterLifetime != 0 {
			raConfig.RouterLifetime = groupRA.RouterLifetime
		}
		if groupRA.MaxInterval != 0 {
			raConfig.MaxInterval = groupRA.MaxInterval
		}
		if groupRA.MinInterval != 0 {
			raConfig.MinInterval = groupRA.MinInterval
		}
	}

	onLink := cfg.DHCPv6.RA.GetOnLink()
	if group != nil && group.IPv6 != nil && group.IPv6.RA != nil && group.IPv6.RA.OnLink != nil {
		onLink = *group.IPv6.RA.OnLink
	}

	var prefixes []PrefixInfo
	if group != nil {
		if profile := cfg.IPv6Profiles[group.IPv6Profile]; profile != nil {
			for _, pool := range profile.IANAPools {
				prefixes = append(prefixes, PrefixInfo{
					Network:       pool.Network,
					ValidTime:     pool.ValidTime,
					PreferredTime: pool.PreferredTime,
					OnLink:        onLink,
				})
			}
		}
	}

	return raConfig, prefixes
}

// Unicast reports whether periodic RAs for this group are delivered as
// per-subscriber unicast (default) or multicast. Moot on point-to-point links.
func Unicast(cfg *config.Config, group *subscriber.SubscriberGroup) bool {
	unicast := cfg.DHCPv6.RA.GetUnicast()
	if group != nil && group.IPv6 != nil && group.IPv6.RA != nil && group.IPv6.RA.Unicast != nil {
		unicast = *group.IPv6.RA.Unicast
	}
	return unicast
}

// RefreshInterval is how often a session's RA must be re-sent to keep its default
// route alive: well inside the Router Lifetime (RFC 4861 §6.2.1), with a /3 margin so
// a single lost RA does not drop the route. Zero means not a default router (no RAs).
func RefreshInterval(rc southbound.IPv6RAConfig) time.Duration {
	rl := rc.RouterLifetime
	if rl == 0 {
		return 0
	}
	if rl > 9000 {
		rl = 9000
	}
	refresh := rc.MaxInterval
	if refresh == 0 {
		refresh = 600
	}
	if third := rl / 3; third < refresh {
		refresh = third
	}
	minI := rc.MinInterval
	if minI == 0 {
		minI = 3
	}
	if refresh < minI {
		refresh = minI
	}
	if refresh < 1 {
		refresh = 1
	}
	return time.Duration(refresh) * time.Second
}

// BuildRARawData serializes the IPv6 + ICMPv6 Router Advertisement payload.
// includeSourceLinkLayer adds the Source Link-Layer Address option (Ethernet
// links such as IPoE); point-to-point links (PPP) omit it per RFC 4861 §4.6.1.
func BuildRARawData(raConfig southbound.IPv6RAConfig, prefixes []PrefixInfo, srcMAC net.HardwareAddr, srcIP, dstIP net.IP, includeSourceLinkLayer bool, log *logger.Logger) ([]byte, error) {
	var raFlags uint8
	if raConfig.Managed {
		raFlags |= 0x80
	}
	if raConfig.Other {
		raFlags |= 0x40
	}

	var raOptions layers.ICMPv6Options
	if includeSourceLinkLayer {
		raOptions = append(raOptions, layers.ICMPv6Option{
			Type: layers.ICMPv6OptSourceAddress,
			Data: srcMAC,
		})
	}

	for _, prefix := range prefixes {
		_, ipNet, err := net.ParseCIDR(prefix.Network)
		if err != nil {
			if log != nil {
				log.Warn("Invalid prefix in RA config", "prefix", prefix.Network, "error", err)
			}
			continue
		}

		prefixLen, _ := ipNet.Mask.Size()

		validLifetime := prefix.ValidTime
		if validLifetime == 0 {
			validLifetime = 2592000
		}
		preferredLifetime := prefix.PreferredTime
		if preferredLifetime == 0 {
			preferredLifetime = 604800
		}
		if !prefix.OnLink {
			// off-link: deprecate the prefix (L=1, lifetime 0) so a host drops any stale
			// on-link route and installs none, routing via the link-local default gateway
			// instead of resolving in-prefix destinations directly (RFC 4861 §6.3.4)
			validLifetime = 0
			preferredLifetime = 0
		}

		prefixData := make([]byte, 30)
		prefixData[0] = byte(prefixLen)
		prefixData[1] = 0x80 // L flag
		binary.BigEndian.PutUint32(prefixData[2:6], validLifetime)
		binary.BigEndian.PutUint32(prefixData[6:10], preferredLifetime)
		// 4 bytes reserved (10:14)
		copy(prefixData[14:30], ipNet.IP.To16())

		raOptions = append(raOptions, layers.ICMPv6Option{
			Type: layers.ICMPv6OptPrefixInfo,
			Data: prefixData,
		})
	}

	routerLifetime := raConfig.RouterLifetime
	if routerLifetime > 9000 {
		routerLifetime = 9000 // RFC 4861 §4.2 maximum router lifetime
	}

	raLayer := &layers.ICMPv6RouterAdvertisement{
		HopLimit:       64,
		Flags:          raFlags,
		RouterLifetime: uint16(routerLifetime),
		ReachableTime:  0,
		RetransTimer:   0,
		Options:        raOptions,
	}

	icmpv6Layer := &layers.ICMPv6{
		TypeCode: layers.CreateICMPv6TypeCode(layers.ICMPv6TypeRouterAdvertisement, 0),
	}

	ipv6Layer := &layers.IPv6{
		Version:    6,
		HopLimit:   255,
		NextHeader: layers.IPProtocolICMPv6,
		SrcIP:      srcIP,
		DstIP:      dstIP,
	}

	_ = icmpv6Layer.SetNetworkLayerForChecksum(ipv6Layer)

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}

	if err := gopacket.SerializeLayers(buf, opts, ipv6Layer, icmpv6Layer, raLayer); err != nil {
		return nil, fmt.Errorf("serialize RA: %w", err)
	}
	return buf.Bytes(), nil
}

// BuildNARawData serializes the IPv6 + ICMPv6 Neighbor Advertisement payload for
// targetAddr (the address being advertised). includeTargetLinkLayer adds the
// Target Link-Layer Address option carrying localMAC (Ethernet links); PPP omits
// it (RFC 4861 §4.4). solicited sets the S flag and is the caller's choice.
func BuildNARawData(targetAddr, srcIP, dstIP net.IP, localMAC net.HardwareAddr, solicited, includeTargetLinkLayer bool) ([]byte, error) {
	var naFlags uint8 = 0x80 | 0x20 // Router | Override
	if solicited {
		naFlags |= 0x40
	}

	var naOptions layers.ICMPv6Options
	if includeTargetLinkLayer {
		naOptions = append(naOptions, layers.ICMPv6Option{
			Type: layers.ICMPv6OptTargetAddress,
			Data: localMAC,
		})
	}

	naLayer := &layers.ICMPv6NeighborAdvertisement{
		Flags:         naFlags,
		TargetAddress: targetAddr,
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
	_ = icmpv6Layer.SetNetworkLayerForChecksum(ipv6Layer)

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}
	if err := gopacket.SerializeLayers(buf, opts, ipv6Layer, icmpv6Layer, naLayer); err != nil {
		return nil, fmt.Errorf("serialize NA: %w", err)
	}
	return buf.Bytes(), nil
}

func sum16(b []byte) uint32 {
	var s uint32
	n := len(b)
	for i := 0; i+1 < n; i += 2 {
		s += uint32(b[i])<<8 | uint32(b[i+1])
	}
	if n%2 == 1 {
		s += uint32(b[n-1]) << 8
	}
	return s
}

func fold16(s uint32) uint16 {
	for s>>16 != 0 {
		s = (s & 0xffff) + (s >> 16)
	}
	return uint16(s)
}

// PatchChecksum recomputes the ICMPv6 checksum over a replicated RA buffer
// (IPv6 + ICMPv6, checksum field [42:44] must already be 0 and the dst patched).
func PatchChecksum(raw []byte) {
	if len(raw) < 44 {
		return
	}
	msg := raw[40:]
	s := sum16(raw[8:24]) + sum16(raw[24:40]) + uint32(len(msg)) + 58 + sum16(msg)
	binary.BigEndian.PutUint16(raw[42:44], ^fold16(s))
}

// GroupState is the fully-resolved RA state shared by every subscriber in a
// (group x SRG x parent) tuple: the pre-serialized RA template (IPv6 + ICMPv6 with
// the destination and checksum zeroed), the unicast policy, the refresh interval
// and the source MAC. Resolved once and reused until the running config changes.
type GroupState struct {
	Cfg     *config.Config
	RawData []byte
	Unicast bool
	Refresh time.Duration
	SrcMAC  net.HardwareAddr
}

// Engine is a per-caller RA template cache. Each component (IPoE, PPPoE)
// constructs its own so their templates and link-layer-option policy never
// cross. includeSourceLinkLayer is fixed per caller (Ethernet vs PPP). The
// cache is owned by the caller's single emitter goroutine, so it is lock-free;
// callers must not share an Engine across goroutines.
type Engine struct {
	states                 map[string]*GroupState
	includeSourceLinkLayer bool
	logger                 *logger.Logger
}

func NewEngine(includeSourceLinkLayer bool, log *logger.Logger) *Engine {
	return &Engine{
		states:                 make(map[string]*GroupState),
		includeSourceLinkLayer: includeSourceLinkLayer,
		logger:                 log,
	}
}

// GroupStateFor returns the cached state for key, re-resolving (the expensive
// config walk + serialize) only when the running config pointer has changed. It
// returns nil on a transient failure (e.g. source MAC not yet available) without
// caching so the next emit retries. The destination and checksum in RawData are
// zeroed; the caller patches the dst and calls PatchChecksum per subscriber.
func (e *Engine) GroupStateFor(key string, cfg *config.Config, group *subscriber.SubscriberGroup, srcMAC net.HardwareAddr) *GroupState {
	if st, ok := e.states[key]; ok && st.Cfg == cfg {
		return st
	}

	if srcMAC == nil {
		return nil
	}
	srcIP := LinkLocalFromMAC(srcMAC)
	if srcIP == nil {
		return nil
	}

	raConfig, prefixes := ResolveGroupRA(cfg, group)
	refresh := RefreshInterval(raConfig)
	if refresh <= 0 {
		return nil
	}

	raw, err := BuildRARawData(raConfig, prefixes, srcMAC, srcIP, net.IPv6zero, e.includeSourceLinkLayer, e.logger)
	if err != nil {
		return nil
	}
	raw[42], raw[43] = 0, 0 // checksum recomputed per subscriber once the dst is patched

	st := &GroupState{
		Cfg:     cfg,
		RawData: raw,
		Unicast: Unicast(cfg, group),
		Refresh: refresh,
		SrcMAC:  srcMAC,
	}
	e.states[key] = st
	return st
}
