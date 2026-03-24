// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package relay

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/ip"
	"github.com/veesix-networks/osvbng/pkg/dhcp/relay"
	"github.com/veesix-networks/osvbng/pkg/dhcp6"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/provider"
)

func init() {
	dhcp6.Register("relay", New)
}

type Provider struct {
	client *relay.Client
	logger *logger.Logger
}

func New(cfg *config.Config) (dhcp6.DHCPProvider, error) {
	return &Provider{
		client: relay.GetClient(),
		logger: logger.Get(logger.IPoERelay),
	}, nil
}

func (p *Provider) Info() provider.Info {
	return provider.Info{
		Name:    "relay",
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
	case 1, 3, 5, 6, 8, 9, 11: // Solicit, Request, Renew, Rebind, Release, Decline, InformationRequest
		return p.handleForward(pkt, dhcpOpts)
	default:
		return nil, fmt.Errorf("unsupported DHCPv6 message type: %d", msgType)
	}
}

func (p *Provider) handleForward(pkt *dhcp6.Packet, opts *ip.IPv6DHCPv6Options) (*dhcp6.Packet, error) {
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

	// If the incoming packet is already a Relay-Forward (from an LDRA),
	// increment hop count for nested relay
	if pkt.Raw[0] == relay.DHCPv6MsgRelayForward {
		params.HopCount = pkt.Raw[1] + 1
	}

	relayFwd := relay.BuildRelayForward(pkt.Raw, params)

	var txnID [3]byte
	if pkt.Raw[0] == relay.DHCPv6MsgRelayForward {
		txnID, _ = relay.GetRelayTransactionID(pkt.Raw)
	} else {
		copy(txnID[:], pkt.Raw[1:4])
	}

	prof := wrapProfile(opts)

	p.logger.Debug("forwarding DHCPv6 to server",
		slog.String("mac", pkt.MAC),
		slog.String("link_address", linkAddr.String()),
	)

	replyRaw, err := p.client.Forward6(
		relayFwd, txnID, servers,
		prof.GetServerTimeout(),
		prof.GetDeadTime(),
		prof.GetDeadThreshold(),
	)
	if err != nil {
		return nil, fmt.Errorf("forward: %w", err)
	}

	// Unwrap Relay-Reply to get the inner message for the client
	inner, err := relay.UnwrapRelayReply(replyRaw)
	if err != nil {
		return nil, fmt.Errorf("unwrap relay-reply: %w", err)
	}

	return &dhcp6.Packet{
		SessionID: pkt.SessionID,
		MAC:       pkt.MAC,
		SVLAN:     pkt.SVLAN,
		CVLAN:     pkt.CVLAN,
		DUID:      pkt.DUID,
		Raw:       inner,
	}, nil
}

func (p *Provider) ReleaseLease(duid []byte) {}

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
