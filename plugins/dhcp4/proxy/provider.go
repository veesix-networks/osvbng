// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/ip"
	"github.com/veesix-networks/osvbng/pkg/dhcp"
	"github.com/veesix-networks/osvbng/pkg/dhcp/relay"
	"github.com/veesix-networks/osvbng/pkg/dhcp4"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/provider"
)

func init() {
	dhcp4.Register("proxy", New)
}

type Provider struct {
	client   *relay.Client
	bindings *Bindings
	logger   *logger.Logger
}

func New(cfg *config.Config) (dhcp4.DHCPProvider, error) {
	return &Provider{
		client:   relay.GetClient(),
		bindings: NewBindings(),
		logger:   logger.Get(logger.IPoERelay),
	}, nil
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

func (p *Provider) HandlePacket(ctx context.Context, pkt *dhcp4.Packet) (*dhcp4.Packet, error) {
	prof := pkt.Profile
	if prof == nil {
		return nil, fmt.Errorf("no profile set on packet")
	}
	dhcpOpts := prof.DHCP
	if dhcpOpts == nil {
		return nil, fmt.Errorf("no dhcp options in profile")
	}

	parsed, err := dhcp.Parse(pkt.Raw)
	if err != nil {
		return nil, fmt.Errorf("parse dhcp: %w", err)
	}

	switch parsed.MessageType {
	case dhcp.DHCPDiscover, dhcp.DHCPRequest:
		return p.handleForwardAndRewrite(pkt, dhcpOpts, parsed)
	case dhcp.DHCPRelease:
		return p.handleRelease(pkt, dhcpOpts)
	default:
		return nil, fmt.Errorf("unsupported message type: %s", parsed.MessageType)
	}
}

func (p *Provider) handleForwardAndRewrite(pkt *dhcp4.Packet, opts *ip.IPv4DHCPOptions, parsed *dhcp.Packet) (*dhcp4.Packet, error) {
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

	if parsed.MessageType == dhcp.DHCPRequest {
		if b, ok := p.bindings.Get(pkt.MAC); ok {
			raw = relay.SetOptionIP(raw, relay.OptServerID, net.IP(b.ServerIP[:]))
		}
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

	prof := wrapProfile(opts)
	reply, err := p.client.Forward4(
		raw, xid, servers,
		prof.GetServerTimeout(),
		prof.GetDeadTime(),
		prof.GetDeadThreshold(),
	)
	if err != nil {
		return nil, fmt.Errorf("forward: %w", err)
	}

	reply = relay.StripOption82(reply)

	serverLease, _ := relay.GetOptionUint32(reply, relay.OptLeaseTime)
	serverID := relay.GetOptionIP(reply, relay.OptServerID)

	clientLease := prof.GetClientLease()

	var srvIP [4]byte
	if serverID != nil {
		copy(srvIP[:], serverID.To4())
	}

	if parsed.MessageType == dhcp.DHCPDiscover {
		p.bindings.Set(pkt.MAC, Binding{
			ServerIP: srvIP,
		})
	} else if parsed.MessageType == dhcp.DHCPRequest && serverLease > 0 {
		var clientIP [4]byte
		if len(reply) >= 20 {
			copy(clientIP[:], reply[16:20])
		}
		p.bindings.Set(pkt.MAC, Binding{
			ClientIP:        clientIP,
			ServerIP:        srvIP,
			ServerLease:     serverLease,
			ClientLease:     clientLease,
			ServerBoundAt:   time.Now().Unix(),
			LastClientRenew: time.Now().Unix(),
		})
	}

	reply = relay.RewriteForProxy(reply, giaddr, clientLease)
	reply = relay.WrapIPUDP(reply, giaddr, net.IPv4bcast)

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

	prof := wrapProfile(opts)
	err = p.client.SendOnly4(raw, servers, prof.GetDeadTime(), prof.GetDeadThreshold())
	if err != nil {
		p.logger.Warn("release forward failed", slog.String("mac", pkt.MAC), slog.Any("error", err))
	}

	p.bindings.Delete(pkt.MAC)
	return nil, nil
}

func (p *Provider) ReleaseLease(mac string) {
	p.bindings.Delete(mac)
}

func wrapProfile(opts *ip.IPv4DHCPOptions) *ip.IPv4Profile {
	return &ip.IPv4Profile{DHCP: opts}
}
