package local

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/allocator"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/dhcp"
	"github.com/veesix-networks/osvbng/pkg/dhcp4"
	"github.com/veesix-networks/osvbng/pkg/provider"
)

func init() {
	dhcp4.Register("local", New)
}

type Provider struct {
	coreConfig *config.Config
	pools      map[string]*IPPool
	leases     map[string]*Lease
	leasesByIP map[string]*Lease
	mu         sync.RWMutex
}

type IPPool struct {
	Name       string
	Network    *net.IPNet
	Gateway    net.IP
	DNSServers []net.IP
	LeaseTime  uint32
}

type Lease struct {
	IP         net.IP
	MAC        string
	SessionID  string
	PoolName   string
	ExpireTime time.Time
}

func New(cfg *config.Config) (dhcp4.DHCPProvider, error) {
	p := &Provider{
		coreConfig: cfg,
		pools:      make(map[string]*IPPool),
		leases:     make(map[string]*Lease),
		leasesByIP: make(map[string]*Lease),
	}

	allocator.InitGlobalRegistry(cfg.IPv4Profiles, cfg.IPv6Profiles)

	for profileName, profile := range cfg.IPv4Profiles {
		leaseTime := profile.GetLeaseTime()
		for _, poolCfg := range profile.Pools {
			gateway := poolCfg.Gateway
			if gateway == "" {
				gateway = profile.Gateway
			}
			dns := poolCfg.DNSServers
			if len(dns) == 0 {
				dns = profile.DNS
			}
			lt := poolCfg.LeaseTime
			if lt == 0 {
				lt = leaseTime
			}

			if err := p.addPool(poolCfg.Name, poolCfg.Network, gateway, dns, lt); err != nil {
				return nil, fmt.Errorf("profile %s pool %s: %w", profileName, poolCfg.Name, err)
			}
		}
	}

	for _, poolCfg := range cfg.DHCP.Pools {
		if err := p.addPool(poolCfg.Name, poolCfg.Network, poolCfg.Gateway, poolCfg.DNSServers, poolCfg.LeaseTime); err != nil {
			return nil, fmt.Errorf("manual pool %s: %w", poolCfg.Name, err)
		}
	}

	return p, nil
}

func (p *Provider) Info() provider.Info {
	return provider.Info{
		Name:    "local",
		Version: "1.0.0",
		Author:  "OSVBNG Core",
	}
}

func (p *Provider) addPool(name, network, gateway string, dnsServers []string, leaseTime uint32) error {
	_, ipnet, err := net.ParseCIDR(network)
	if err != nil {
		return fmt.Errorf("invalid network: %w", err)
	}

	pool := &IPPool{
		Name:       name,
		Network:    ipnet,
		Gateway:    net.ParseIP(gateway),
		DNSServers: make([]net.IP, 0),
		LeaseTime:  leaseTime,
	}

	for _, dns := range dnsServers {
		pool.DNSServers = append(pool.DNSServers, net.ParseIP(dns))
	}

	p.pools[name] = pool
	return nil
}

func (p *Provider) findPoolForIP(ip net.IP) *IPPool {
	for _, pool := range p.pools {
		if pool.Network.Contains(ip) {
			return pool
		}
	}
	return nil
}

func (p *Provider) HandlePacket(ctx context.Context, pkt *dhcp4.Packet) (*dhcp4.Packet, error) {
	msg, err := dhcp4.ParseMessage(pkt.Raw)
	if err != nil {
		return nil, fmt.Errorf("failed to decode DHCP layer: %w", err)
	}

	switch msg.Options.MessageType {
	case dhcp4.MsgTypeDiscover:
		return p.handleDiscover(pkt, msg)
	case dhcp4.MsgTypeRequest:
		return p.handleRequest(pkt, msg)
	case dhcp4.MsgTypeRelease:
		return p.handleRelease(pkt, msg)
	default:
		return nil, fmt.Errorf("unsupported DHCP message type: %v", msg.Options.MessageType)
	}
}

func (p *Provider) handleDiscover(pkt *dhcp4.Packet, msg *dhcp4.Message) (*dhcp4.Packet, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	mac := msg.ClientHWAddr.String()

	if pkt.Resolved != nil {
		if err := p.reserveIP(pkt.Resolved.YourIP, mac, pkt.SessionID, pkt.Resolved.PoolName, pkt.Resolved.LeaseTime); err != nil {
			return nil, fmt.Errorf("reserve IP: %w", err)
		}
		response := p.buildResponseFromResolved(msg, pkt.Resolved, dhcp4.MsgTypeOffer)
		return &dhcp4.Packet{
			SessionID: pkt.SessionID,
			MAC:       pkt.MAC,
			SVLAN:     pkt.SVLAN,
			CVLAN:     pkt.CVLAN,
			Raw:       response,
		}, nil
	}

	existingLease, exists := p.leases[mac]
	if !exists {
		return nil, fmt.Errorf("no resolved address and no existing lease for %s", mac)
	}

	pool := p.findPoolForIP(existingLease.IP)
	response := p.buildOffer(msg, existingLease.IP, pool)
	return &dhcp4.Packet{
		SessionID: pkt.SessionID,
		MAC:       pkt.MAC,
		SVLAN:     pkt.SVLAN,
		CVLAN:     pkt.CVLAN,
		Raw:       response,
	}, nil
}

func (p *Provider) handleRequest(pkt *dhcp4.Packet, msg *dhcp4.Message) (*dhcp4.Packet, error) {
	mac := msg.ClientHWAddr.String()

	if pkt.Resolved != nil {
		p.mu.Lock()
		defer p.mu.Unlock()

		if err := p.reserveIP(pkt.Resolved.YourIP, mac, pkt.SessionID, pkt.Resolved.PoolName, pkt.Resolved.LeaseTime); err != nil {
			return nil, fmt.Errorf("reserve IP: %w", err)
		}
		response := p.buildResponseFromResolved(msg, pkt.Resolved, dhcp4.MsgTypeAck)
		return &dhcp4.Packet{
			SessionID: pkt.SessionID,
			MAC:       pkt.MAC,
			SVLAN:     pkt.SVLAN,
			CVLAN:     pkt.CVLAN,
			Raw:       response,
		}, nil
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	requestedIP := msg.Options.RequestedIP
	if requestedIP == nil {
		requestedIP = msg.ClientIP
	}

	lease, exists := p.leases[mac]
	if !exists || !lease.IP.Equal(requestedIP) {
		return nil, fmt.Errorf("no resolved address and no matching lease for %s", mac)
	}

	pool := p.findPoolForIP(requestedIP)
	if pool != nil {
		lease.ExpireTime = time.Now().Add(time.Duration(pool.LeaseTime) * time.Second)
	}

	response := p.buildAck(msg, requestedIP, pool)
	return &dhcp4.Packet{
		SessionID: pkt.SessionID,
		MAC:       pkt.MAC,
		SVLAN:     pkt.SVLAN,
		CVLAN:     pkt.CVLAN,
		Raw:       response,
	}, nil
}

func (p *Provider) handleRelease(pkt *dhcp4.Packet, msg *dhcp4.Message) (*dhcp4.Packet, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	mac := msg.ClientHWAddr.String()
	if lease, exists := p.leases[mac]; exists {
		if lease.PoolName != "" {
			allocator.GetGlobalRegistry().Release(lease.PoolName, lease.IP)
		}
		delete(p.leasesByIP, lease.IP.String())
		delete(p.leases, mac)
	}

	return nil, nil
}

func (p *Provider) ReleaseLease(mac string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	lease, exists := p.leases[mac]
	if !exists {
		return
	}
	if lease.PoolName != "" {
		allocator.GetGlobalRegistry().Release(lease.PoolName, lease.IP)
	}
	delete(p.leasesByIP, lease.IP.String())
	delete(p.leases, mac)
}

func (p *Provider) reserveIP(reserveIP net.IP, mac, sessionID, poolName string, leaseTime time.Duration) error {
	ipStr := reserveIP.String()
	if existing, exists := p.leasesByIP[ipStr]; exists {
		if existing.MAC == mac {
			existing.ExpireTime = time.Now().Add(leaseTime)
			return nil
		}
		if time.Now().After(existing.ExpireTime) {
			if existing.PoolName != "" {
				allocator.GetGlobalRegistry().Release(existing.PoolName, existing.IP)
			}
			delete(p.leasesByIP, ipStr)
			delete(p.leases, existing.MAC)
		} else {
			return fmt.Errorf("IP %s already leased to %s", ipStr, existing.MAC)
		}
	}
	lease := &Lease{
		IP:         reserveIP,
		MAC:        mac,
		SessionID:  sessionID,
		PoolName:   poolName,
		ExpireTime: time.Now().Add(leaseTime),
	}
	p.leases[mac] = lease
	p.leasesByIP[ipStr] = lease
	return nil
}

func (p *Provider) buildOffer(req *dhcp4.Message, offerIP net.IP, pool *IPPool) []byte {
	return p.buildResponse(req, offerIP, pool, dhcp4.MsgTypeOffer)
}

func (p *Provider) buildAck(req *dhcp4.Message, ackIP net.IP, pool *IPPool) []byte {
	return p.buildResponse(req, ackIP, pool, dhcp4.MsgTypeAck)
}

func (p *Provider) buildResponse(req *dhcp4.Message, ip net.IP, pool *IPPool, msgType dhcp4.MessageType) []byte {
	dhcpPayload := buildDHCPv4Reply(req, ip, pool.Gateway, msgType, func(opts *optionWriter) {
		opts.addByte(dhcp4.OptServerID, pool.Gateway.To4())
		leaseData := make([]byte, 4)
		binary.BigEndian.PutUint32(leaseData, pool.LeaseTime)
		opts.addByte(dhcp4.OptLeaseTime, leaseData)
		opts.addByte(dhcp4.OptSubnetMask, []byte(pool.Network.Mask))
		opts.addByte(dhcp4.OptRouter, pool.Gateway.To4())
		if len(pool.DNSServers) > 0 {
			dnsData := make([]byte, 0, len(pool.DNSServers)*4)
			for _, dns := range pool.DNSServers {
				dnsData = append(dnsData, dns.To4()...)
			}
			opts.addByte(dhcp4.OptDNS, dnsData)
		}
	})

	srcIP := pool.Gateway
	return dhcp.BuildIPv4UDPFrame(srcIP, net.IPv4bcast, 67, 68, dhcpPayload)
}

func (p *Provider) buildResponseFromResolved(req *dhcp4.Message, resolved *dhcp.ResolvedDHCPv4, msgType dhcp4.MessageType) []byte {
	srcIP := resolved.Router
	if resolved.ServerID != nil {
		srcIP = resolved.ServerID
	}

	dhcpPayload := buildDHCPv4Reply(req, resolved.YourIP, srcIP, msgType, func(opts *optionWriter) {
		leaseData := make([]byte, 4)
		binary.BigEndian.PutUint32(leaseData, uint32(resolved.LeaseTime.Seconds()))
		opts.addByte(dhcp4.OptLeaseTime, leaseData)
		opts.addByte(dhcp4.OptSubnetMask, []byte(resolved.Netmask))
		if resolved.ServerID != nil {
			opts.addByte(dhcp4.OptServerID, resolved.ServerID.To4())
		}
		if resolved.Router != nil {
			opts.addByte(dhcp4.OptRouter, resolved.Router.To4())
		}
		if len(resolved.DNS) > 0 {
			dnsData := make([]byte, 0, len(resolved.DNS)*4)
			for _, dns := range resolved.DNS {
				dnsData = append(dnsData, dns.To4()...)
			}
			opts.addByte(dhcp4.OptDNS, dnsData)
		}
		if len(resolved.ClasslessRoutes) > 0 {
			routeData := encodeClasslessRoutes(resolved.ClasslessRoutes)
			opts.addByte(121, routeData)
		}
	})

	return dhcp.BuildIPv4UDPFrame(srcIP, net.IPv4bcast, 67, 68, dhcpPayload)
}

type optionWriter struct {
	buf []byte
}

func (w *optionWriter) addByte(optType uint8, data []byte) {
	w.buf = append(w.buf, optType, uint8(len(data)))
	w.buf = append(w.buf, data...)
}

func buildDHCPv4Reply(req *dhcp4.Message, yourIP net.IP, serverIP net.IP, msgType dhcp4.MessageType, writeOptions func(*optionWriter)) []byte {
	buf := make([]byte, dhcp4.DHCPv4HeaderLen+4)

	buf[0] = 2
	buf[1] = 1
	buf[2] = 6
	buf[3] = 0
	binary.BigEndian.PutUint32(buf[4:8], req.XID)

	if req.ClientIP != nil {
		copy(buf[12:16], req.ClientIP.To4())
	}
	if yourIP != nil {
		copy(buf[16:20], yourIP.To4())
	}
	if serverIP != nil {
		copy(buf[20:24], serverIP.To4())
	}
	if len(req.ClientHWAddr) > 0 {
		copy(buf[28:28+len(req.ClientHWAddr)], req.ClientHWAddr)
	}

	binary.BigEndian.PutUint32(buf[236:240], 0x63825363)

	opts := &optionWriter{}
	opts.addByte(dhcp4.OptMessageType, []byte{byte(msgType)})
	writeOptions(opts)
	opts.buf = append(opts.buf, dhcp4.OptEnd)

	buf = append(buf, opts.buf...)
	return buf
}

func encodeClasslessRoutes(routes []dhcp.ClasslessRoute) []byte {
	var data []byte
	for _, route := range routes {
		ones, _ := route.Destination.Mask.Size()
		data = append(data, byte(ones))
		significantBytes := (ones + 7) / 8
		data = append(data, route.Destination.IP.To4()[:significantBytes]...)
		data = append(data, route.NextHop.To4()...)
	}
	return data
}
