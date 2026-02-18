package local

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
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
	dhcp := &layers.DHCPv4{}
	if err := dhcp.DecodeFromBytes(pkt.Raw, gopacket.NilDecodeFeedback); err != nil {
		return nil, fmt.Errorf("failed to decode DHCP layer: %w", err)
	}

	msgType := layers.DHCPMsgTypeUnspecified
	for _, opt := range dhcp.Options {
		if opt.Type == layers.DHCPOptMessageType && len(opt.Data) == 1 {
			msgType = layers.DHCPMsgType(opt.Data[0])
			break
		}
	}

	switch msgType {
	case layers.DHCPMsgTypeDiscover:
		return p.handleDiscover(pkt, dhcp)
	case layers.DHCPMsgTypeRequest:
		return p.handleRequest(pkt, dhcp)
	case layers.DHCPMsgTypeRelease:
		return p.handleRelease(pkt, dhcp)
	default:
		return nil, fmt.Errorf("unsupported DHCP message type: %v", msgType)
	}
}

func (p *Provider) handleDiscover(pkt *dhcp4.Packet, dhcpPkt *layers.DHCPv4) (*dhcp4.Packet, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	mac := dhcpPkt.ClientHWAddr.String()

	if pkt.Resolved != nil {
		if err := p.reserveIP(pkt.Resolved.YourIP, mac, pkt.SessionID, pkt.Resolved.PoolName, pkt.Resolved.LeaseTime); err != nil {
			return nil, fmt.Errorf("reserve IP: %w", err)
		}
		response := p.buildResponseFromResolved(dhcpPkt, pkt.Resolved, layers.DHCPMsgTypeOffer)
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
	response := p.buildOffer(dhcpPkt, existingLease.IP, pool)
	return &dhcp4.Packet{
		SessionID: pkt.SessionID,
		MAC:       pkt.MAC,
		SVLAN:     pkt.SVLAN,
		CVLAN:     pkt.CVLAN,
		Raw:       response,
	}, nil
}

func (p *Provider) handleRequest(pkt *dhcp4.Packet, dhcpPkt *layers.DHCPv4) (*dhcp4.Packet, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	mac := dhcpPkt.ClientHWAddr.String()

	if pkt.Resolved != nil {
		if err := p.reserveIP(pkt.Resolved.YourIP, mac, pkt.SessionID, pkt.Resolved.PoolName, pkt.Resolved.LeaseTime); err != nil {
			return nil, fmt.Errorf("reserve IP: %w", err)
		}
		response := p.buildResponseFromResolved(dhcpPkt, pkt.Resolved, layers.DHCPMsgTypeAck)
		return &dhcp4.Packet{
			SessionID: pkt.SessionID,
			MAC:       pkt.MAC,
			SVLAN:     pkt.SVLAN,
			CVLAN:     pkt.CVLAN,
			Raw:       response,
		}, nil
	}

	var requestedIP net.IP
	for _, opt := range dhcpPkt.Options {
		if opt.Type == layers.DHCPOptRequestIP && len(opt.Data) == 4 {
			requestedIP = net.IP(opt.Data)
			break
		}
	}

	if requestedIP == nil {
		requestedIP = dhcpPkt.ClientIP
	}

	lease, exists := p.leases[mac]
	if !exists || !lease.IP.Equal(requestedIP) {
		return nil, fmt.Errorf("no resolved address and no matching lease for %s", mac)
	}

	pool := p.findPoolForIP(requestedIP)
	if pool != nil {
		lease.ExpireTime = time.Now().Add(time.Duration(pool.LeaseTime) * time.Second)
	}

	response := p.buildAck(dhcpPkt, requestedIP, pool)
	return &dhcp4.Packet{
		SessionID: pkt.SessionID,
		MAC:       pkt.MAC,
		SVLAN:     pkt.SVLAN,
		CVLAN:     pkt.CVLAN,
		Raw:       response,
	}, nil
}

func (p *Provider) handleRelease(pkt *dhcp4.Packet, dhcp *layers.DHCPv4) (*dhcp4.Packet, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	mac := dhcp.ClientHWAddr.String()
	if lease, exists := p.leases[mac]; exists {
		if lease.PoolName != "" {
			allocator.GetGlobalRegistry().Release(lease.PoolName, lease.IP)
		}
		delete(p.leasesByIP, lease.IP.String())
		delete(p.leases, mac)
	}

	return nil, nil
}

func (p *Provider) reserveIP(reserveIP net.IP, mac, sessionID, poolName string, leaseTime time.Duration) error {
	ipStr := reserveIP.String()
	if existing, exists := p.leasesByIP[ipStr]; exists {
		if existing.MAC == mac {
			existing.ExpireTime = time.Now().Add(leaseTime)
			return nil
		}
		return fmt.Errorf("IP %s already leased to %s", ipStr, existing.MAC)
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

func (p *Provider) buildOffer(req *layers.DHCPv4, offerIP net.IP, pool *IPPool) []byte {
	return p.buildResponse(req, offerIP, pool, layers.DHCPMsgTypeOffer)
}

func (p *Provider) buildAck(req *layers.DHCPv4, ackIP net.IP, pool *IPPool) []byte {
	return p.buildResponse(req, ackIP, pool, layers.DHCPMsgTypeAck)
}

func (p *Provider) buildResponse(req *layers.DHCPv4, ip net.IP, pool *IPPool, msgType layers.DHCPMsgType) []byte {
	options := []layers.DHCPOption{
		{
			Type:   layers.DHCPOptMessageType,
			Data:   []byte{byte(msgType)},
			Length: 1,
		},
		{
			Type:   layers.DHCPOptServerID,
			Data:   pool.Gateway.To4(),
			Length: 4,
		},
		{
			Type:   layers.DHCPOptLeaseTime,
			Data:   make([]byte, 4),
			Length: 4,
		},
		{
			Type:   layers.DHCPOptSubnetMask,
			Data:   pool.Network.Mask,
			Length: 4,
		},
		{
			Type:   layers.DHCPOptRouter,
			Data:   pool.Gateway.To4(),
			Length: 4,
		},
	}

	binary.BigEndian.PutUint32(options[2].Data, pool.LeaseTime)

	if len(pool.DNSServers) > 0 {
		dnsData := make([]byte, 0)
		for _, dns := range pool.DNSServers {
			dnsData = append(dnsData, dns.To4()...)
		}
		options = append(options, layers.DHCPOption{
			Type:   layers.DHCPOptDNS,
			Data:   dnsData,
			Length: uint8(len(dnsData)),
		})
	}

	dhcpReply := &layers.DHCPv4{
		Operation:    layers.DHCPOpReply,
		HardwareType: layers.LinkTypeEthernet,
		HardwareLen:  6,
		Xid:          req.Xid,
		ClientIP:     req.ClientIP,
		YourClientIP: ip,
		NextServerIP: pool.Gateway,
		ClientHWAddr: req.ClientHWAddr,
		Options:      options,
	}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}

	ipv4 := &layers.IPv4{
		Version:  4,
		TTL:      64,
		Protocol: layers.IPProtocolUDP,
		SrcIP:    pool.Gateway,
		DstIP:    net.IPv4bcast,
	}

	udp := &layers.UDP{
		SrcPort: 67,
		DstPort: 68,
	}
	udp.SetNetworkLayerForChecksum(ipv4)

	gopacket.SerializeLayers(buf, opts, ipv4, udp, dhcpReply)
	return buf.Bytes()
}

func (p *Provider) buildResponseFromResolved(req *layers.DHCPv4, resolved *dhcp.ResolvedDHCPv4, msgType layers.DHCPMsgType) []byte {
	leaseData := make([]byte, 4)
	binary.BigEndian.PutUint32(leaseData, uint32(resolved.LeaseTime.Seconds()))

	options := []layers.DHCPOption{
		{Type: layers.DHCPOptMessageType, Data: []byte{byte(msgType)}, Length: 1},
		{Type: layers.DHCPOptLeaseTime, Data: leaseData, Length: 4},
		{Type: layers.DHCPOptSubnetMask, Data: resolved.Netmask, Length: uint8(len(resolved.Netmask))},
	}

	if resolved.ServerID != nil {
		options = append(options, layers.DHCPOption{
			Type: layers.DHCPOptServerID, Data: resolved.ServerID.To4(), Length: 4,
		})
	}

	if resolved.Router != nil {
		options = append(options, layers.DHCPOption{
			Type: layers.DHCPOptRouter, Data: resolved.Router.To4(), Length: 4,
		})
	}

	if len(resolved.DNS) > 0 {
		dnsData := make([]byte, 0, len(resolved.DNS)*4)
		for _, dns := range resolved.DNS {
			dnsData = append(dnsData, dns.To4()...)
		}
		options = append(options, layers.DHCPOption{
			Type: layers.DHCPOptDNS, Data: dnsData, Length: uint8(len(dnsData)),
		})
	}

	if len(resolved.ClasslessRoutes) > 0 {
		routeData := encodeClasslessRoutes(resolved.ClasslessRoutes)
		options = append(options, layers.DHCPOption{
			Type: 121, Data: routeData, Length: uint8(len(routeData)),
		})
	}

	srcIP := resolved.Router
	if resolved.ServerID != nil {
		srcIP = resolved.ServerID
	}

	dhcpReply := &layers.DHCPv4{
		Operation:    layers.DHCPOpReply,
		HardwareType: layers.LinkTypeEthernet,
		HardwareLen:  6,
		Xid:          req.Xid,
		ClientIP:     req.ClientIP,
		YourClientIP: resolved.YourIP,
		NextServerIP: srcIP,
		ClientHWAddr: req.ClientHWAddr,
		Options:      options,
	}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}

	ipv4 := &layers.IPv4{
		Version:  4,
		TTL:      64,
		Protocol: layers.IPProtocolUDP,
		SrcIP:    srcIP,
		DstIP:    net.IPv4bcast,
	}

	udp := &layers.UDP{SrcPort: 67, DstPort: 68}
	udp.SetNetworkLayerForChecksum(ipv4)

	gopacket.SerializeLayers(buf, opts, ipv4, udp, dhcpReply)
	return buf.Bytes()
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
