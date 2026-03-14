// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package proxy

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/ip"
	"github.com/veesix-networks/osvbng/pkg/dhcp/relay"
	"github.com/veesix-networks/osvbng/pkg/dhcp6"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/provider"
)

func init() {
	dhcp6.Register("proxy", New)
}

type Provider struct {
	client   *relay.Client
	bindings *Bindings
	duidMu   sync.RWMutex
	duidMap  map[string][]byte
	logger   *slog.Logger
}

func New(cfg *config.Config) (dhcp6.DHCPProvider, error) {
	return &Provider{
		client:   relay.GetClient(),
		bindings: NewBindings(),
		duidMap:  make(map[string][]byte),
		logger:   logger.Get(logger.IPoERelay),
	}, nil
}

var dhcpv6Epoch = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

func (p *Provider) getDUID(mac net.HardwareAddr) []byte {
	if len(mac) < 6 {
		return nil
	}
	key := string(mac[:6])

	p.duidMu.RLock()
	if duid, ok := p.duidMap[key]; ok {
		p.duidMu.RUnlock()
		return duid
	}
	p.duidMu.RUnlock()

	duid := make([]byte, 14)
	binary.BigEndian.PutUint16(duid[0:2], 1)
	binary.BigEndian.PutUint16(duid[2:4], 1)
	binary.BigEndian.PutUint32(duid[4:8], uint32(time.Since(dhcpv6Epoch).Seconds()))
	copy(duid[8:14], mac[:6])

	p.duidMu.Lock()
	p.duidMap[key] = duid
	p.duidMu.Unlock()
	return duid
}

func (p *Provider) BindingCount() int {
	return p.bindings.Count()
}

func (p *Provider) Info() provider.Info {
	return provider.Info{
		Name:    "proxy",
		Version: "1.0.0",
		Author:  "OSVBNG Core",
	}
}

func (p *Provider) HandlePacket(ctx context.Context, pkt *dhcp6.Packet) (*dhcp6.Packet, error) {
	profile := pkt.Profile
	if profile == nil {
		return nil, fmt.Errorf("no profile set on packet")
	}
	dhcpOpts := profile.DHCPv6
	if dhcpOpts == nil {
		return nil, fmt.Errorf("no dhcpv6 options in profile")
	}

	if len(pkt.Raw) < 4 {
		return nil, fmt.Errorf("packet too short: %d bytes", len(pkt.Raw))
	}

	msgType := pkt.Raw[0]

	switch msgType {
	case 1, 3, 5, 6, 8: // Solicit, Request, Renew, Rebind, Release
		return p.handleForwardAndRewrite(pkt, dhcpOpts, msgType)
	default:
		return nil, fmt.Errorf("unsupported DHCPv6 message type: %d", msgType)
	}
}

func (p *Provider) handleForwardAndRewrite(pkt *dhcp6.Packet, opts *ip.IPv6DHCPv6Options, msgType byte) (*dhcp6.Packet, error) {
	if msgType == 8 { // Release
		return p.handleRelease(pkt, opts)
	}

	servers, err := relay.ResolveServers(opts.Servers)
	if err != nil {
		return nil, fmt.Errorf("resolve servers: %w", err)
	}

	linkAddr := net.ParseIP(opts.LinkAddress)
	if linkAddr == nil {
		linkAddr = net.IPv6zero
	}

	peerAddr := pkt.PeerAddr
	if peerAddr == nil {
		peerAddr = net.IPv6zero
	}

	params := &relay.RelayForwardParams{
		HopCount:    0,
		LinkAddress: linkAddr,
		PeerAddress: peerAddr,
	}

	if opts.InterfaceIDFormat != "" {
		params.InterfaceID = formatV6Field(opts.InterfaceIDFormat, pkt)
	} else {
		params.InterfaceID = formatV6Field("{interface}:{svlan}:{cvlan}", pkt)
	}

	if opts.RemoteIDFormat != "" {
		params.RemoteID = formatV6Field(opts.RemoteIDFormat, pkt)
		params.EnterpriseNumber = opts.EnterpriseNumber
	}

	if opts.SubscriberIDFormat != "" {
		params.SubscriberID = formatV6Field(opts.SubscriberIDFormat, pkt)
	}

	if pkt.Raw[0] == relay.DHCPv6MsgRelayForward {
		params.HopCount = pkt.Raw[1] + 1
	}

	if msgType == 3 || msgType == 5 || msgType == 6 {
		if b, ok := p.bindings.Get(string(pkt.DUID)); ok && len(b.ServerDUID) > 0 {
			pkt.Raw = relay.ReplaceServerDUID(pkt.Raw, b.ServerDUID)
		}
	}

	relayFwd := relay.BuildRelayForward(pkt.Raw, params)

	var txnID [3]byte
	if pkt.Raw[0] == relay.DHCPv6MsgRelayForward {
		txnID, _ = relay.GetRelayTransactionID(pkt.Raw)
	} else {
		copy(txnID[:], pkt.Raw[1:4])
	}

	prof := wrapProfile(opts)

	replyRaw, err := p.client.Forward6(
		relayFwd, txnID, servers,
		prof.GetServerTimeout(),
		prof.GetDeadTime(),
		prof.GetDeadThreshold(),
	)
	if err != nil {
		return nil, fmt.Errorf("forward: %w", err)
	}

	inner, err := relay.UnwrapRelayReply(replyRaw)
	if err != nil {
		return nil, fmt.Errorf("unwrap relay-reply: %w", err)
	}

	serverDUID := relay.GetServerDUID(inner)

	if msgType == 1 && len(pkt.DUID) > 0 {
		p.bindings.Set(string(pkt.DUID), Binding{
			ServerDUID: serverDUID,
		})
	} else if msgType == 3 && len(pkt.DUID) > 0 {
		p.bindings.Set(string(pkt.DUID), Binding{
			ServerDUID:      serverDUID,
			ServerPreferred: prof.GetPreferredTime(),
			ServerValid:     prof.GetValidTime(),
			ClientPreferred: prof.GetClientPreferredLifetime(),
			ClientValid:     prof.GetClientValidLifetime(),
			ServerBoundAt:   time.Now().Unix(),
			LastClientRenew: time.Now().Unix(),
		})
	}

	inner = relay.ReplaceServerDUID(inner, p.getDUID(pkt.LocalMAC))
	inner = relay.RewriteV6Lifetimes(inner, prof.GetClientPreferredLifetime(), prof.GetClientValidLifetime())

	return &dhcp6.Packet{
		SessionID: pkt.SessionID,
		MAC:       pkt.MAC,
		SVLAN:     pkt.SVLAN,
		CVLAN:     pkt.CVLAN,
		DUID:      pkt.DUID,
		Raw:       inner,
	}, nil
}

func (p *Provider) handleRelease(pkt *dhcp6.Packet, opts *ip.IPv6DHCPv6Options) (*dhcp6.Packet, error) {
	servers, err := relay.ResolveServers(opts.Servers)
	if err != nil {
		return nil, fmt.Errorf("resolve servers: %w", err)
	}

	linkAddr := net.ParseIP(opts.LinkAddress)
	if linkAddr == nil {
		linkAddr = net.IPv6zero
	}

	peerAddr := pkt.PeerAddr
	if peerAddr == nil {
		peerAddr = net.IPv6zero
	}

	params := &relay.RelayForwardParams{
		HopCount:    0,
		LinkAddress: linkAddr,
		PeerAddress: peerAddr,
	}

	if opts.InterfaceIDFormat != "" {
		params.InterfaceID = formatV6Field(opts.InterfaceIDFormat, pkt)
	} else {
		params.InterfaceID = formatV6Field("{interface}:{svlan}:{cvlan}", pkt)
	}

	relayFwd := relay.BuildRelayForward(pkt.Raw, params)

	var txnID [3]byte
	copy(txnID[:], pkt.Raw[1:4])

	prof := wrapProfile(opts)

	_, err = p.client.Forward6(
		relayFwd, txnID, servers,
		prof.GetServerTimeout(),
		prof.GetDeadTime(),
		prof.GetDeadThreshold(),
	)
	if err != nil {
		p.logger.Warn("release forward failed",
			slog.String("mac", pkt.MAC),
			slog.Any("error", err),
		)
	}

	if len(pkt.DUID) > 0 {
		p.bindings.Delete(string(pkt.DUID))
	}

	return nil, nil
}

func (p *Provider) ReleaseLease(duid []byte) {
	if len(duid) > 0 {
		p.bindings.Delete(string(duid))
	}
}

func formatV6Field(format string, pkt *dhcp6.Packet) []byte {
	r := strings.NewReplacer(
		"{interface}", pkt.Interface,
		"{svlan}", fmt.Sprintf("%d", pkt.SVLAN),
		"{cvlan}", fmt.Sprintf("%d", pkt.CVLAN),
		"{mac}", pkt.MAC,
	)
	return []byte(r.Replace(format))
}

func wrapProfile(opts *ip.IPv6DHCPv6Options) *ip.IPv6Profile {
	return &ip.IPv6Profile{DHCPv6: opts}
}
