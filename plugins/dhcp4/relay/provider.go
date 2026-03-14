// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package relay

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/ip"
	"github.com/veesix-networks/osvbng/pkg/dhcp"
	"github.com/veesix-networks/osvbng/pkg/dhcp/relay"
	"github.com/veesix-networks/osvbng/pkg/dhcp4"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/provider"
)

func init() {
	dhcp4.Register("relay", New)
}

type Provider struct {
	client *relay.Client
	logger *slog.Logger
}

func New(cfg *config.Config) (dhcp4.DHCPProvider, error) {
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

func (p *Provider) HandlePacket(ctx context.Context, pkt *dhcp4.Packet) (*dhcp4.Packet, error) {
	profile := pkt.Profile
	if profile == nil {
		return nil, fmt.Errorf("no profile set on packet")
	}
	dhcpOpts := profile.DHCP
	if dhcpOpts == nil {
		return nil, fmt.Errorf("no dhcp options in profile")
	}

	parsed, err := dhcp.Parse(pkt.Raw)
	if err != nil {
		return nil, fmt.Errorf("parse dhcp: %w", err)
	}

	switch parsed.MessageType {
	case dhcp.DHCPDiscover, dhcp.DHCPRequest:
		return p.handleForward(pkt, dhcpOpts)
	case dhcp.DHCPRelease:
		return p.handleRelease(pkt, dhcpOpts)
	default:
		return nil, fmt.Errorf("unsupported message type: %s", parsed.MessageType)
	}
}

func (p *Provider) handleForward(pkt *dhcp4.Packet, opts *ip.IPv4DHCPOptions) (*dhcp4.Packet, error) {
	servers, err := relay.ResolveServers(opts.Servers)
	if err != nil {
		return nil, fmt.Errorf("resolve servers: %w", err)
	}

	raw := make([]byte, len(pkt.Raw))
	copy(raw, pkt.Raw)

	giaddr := net.ParseIP(opts.GIAddr)
	if giaddr == nil {
		return nil, fmt.Errorf("invalid giaddr: %s", opts.GIAddr)
	}

	relay.SetGIAddr(raw, giaddr)
	relay.IncrementHops(raw)

	if opts.Option82 != nil {
		opt82Data, err := relay.BuildOption82(opts.Option82, &relay.Option82Params{
			Interface: pkt.Interface,
			SVLAN:     pkt.SVLAN,
			CVLAN:     pkt.CVLAN,
			MAC:       pkt.MAC,
		}, false)
		if err != nil {
			return nil, fmt.Errorf("build option 82: %w", err)
		}
		raw = relay.InsertOption82(raw, opt82Data, opts.Option82.GetPolicy())
	}

	xid := uint32(raw[4])<<24 | uint32(raw[5])<<16 | uint32(raw[6])<<8 | uint32(raw[7])

	p.logger.Debug("forwarding to server",
		slog.String("mac", pkt.MAC),
		slog.String("giaddr", opts.GIAddr),
		slog.Uint64("xid", uint64(xid)),
	)

	reply, err := p.client.Forward4(
		raw, xid, servers,
		profile(opts).GetServerTimeout(),
		profile(opts).GetDeadTime(),
		profile(opts).GetDeadThreshold(),
	)
	if err != nil {
		return nil, fmt.Errorf("forward: %w", err)
	}

	reply = relay.StripOption82(reply)

	serverID := relay.GetOptionIP(reply, relay.OptServerID)
	if serverID == nil {
		serverID = giaddr
	}
	reply = relay.WrapIPUDP(reply, serverID, net.IPv4bcast)

	return &dhcp4.Packet{
		SessionID: pkt.SessionID,
		MAC:       pkt.MAC,
		SVLAN:     pkt.SVLAN,
		CVLAN:     pkt.CVLAN,
		Raw:       reply,
	}, nil
}

func (p *Provider) handleRelease(pkt *dhcp4.Packet, opts *ip.IPv4DHCPOptions) (*dhcp4.Packet, error) {
	servers, err := relay.ResolveServers(opts.Servers)
	if err != nil {
		return nil, fmt.Errorf("resolve servers: %w", err)
	}

	raw := make([]byte, len(pkt.Raw))
	copy(raw, pkt.Raw)

	giaddr := net.ParseIP(opts.GIAddr)
	if giaddr == nil {
		return nil, fmt.Errorf("invalid giaddr: %s", opts.GIAddr)
	}

	relay.SetGIAddr(raw, giaddr)
	relay.IncrementHops(raw)

	if opts.Option82 != nil {
		opt82Data, err := relay.BuildOption82(opts.Option82, &relay.Option82Params{
			Interface: pkt.Interface,
			SVLAN:     pkt.SVLAN,
			CVLAN:     pkt.CVLAN,
			MAC:       pkt.MAC,
		}, false)
		if err != nil {
			return nil, fmt.Errorf("build option 82: %w", err)
		}
		raw = relay.InsertOption82(raw, opt82Data, opts.Option82.GetPolicy())
	}

	err = p.client.SendOnly4(raw, servers, profile(opts).GetDeadTime(), profile(opts).GetDeadThreshold())
	if err != nil {
		p.logger.Warn("release forward failed", slog.String("mac", pkt.MAC), slog.Any("error", err))
	}

	return nil, nil
}

func (p *Provider) ReleaseLease(mac string) {}

// profile wraps DHCPOptions in a temporary IPv4Profile to use getter methods.
func profile(opts *ip.IPv4DHCPOptions) *ip.IPv4Profile {
	return &ip.IPv4Profile{DHCP: opts}
}
