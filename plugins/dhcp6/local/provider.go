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
	"github.com/veesix-networks/osvbng/pkg/dhcp6"
	"github.com/veesix-networks/osvbng/pkg/provider"
)

func init() {
	dhcp6.Register("local", New)
}

const (
	OptClientID     uint16 = 1
	OptServerID     uint16 = 2
	OptIANA         uint16 = 3
	OptIAAddr       uint16 = 5
	OptStatusCode   uint16 = 13
	OptDNSServers   uint16 = 23
	OptDomainList   uint16 = 24
	OptIAPD         uint16 = 25
	OptIAPrefix     uint16 = 26

	StatusSuccess uint16 = 0
	StatusNoAddrsAvail uint16 = 2
	StatusNoPrefixAvail uint16 = 6
)

type Provider struct {
	coreConfig   *config.Config
	serverDUID   []byte
	ianaPools    map[string]*IANAPool
	pdPools      map[string]*PDPool
	ianaLeases   map[string]*IANALease
	pdLeases     map[string]*PDLease
	leasesByAddr map[string]*IANALease
	leasesByPfx  map[string]*PDLease
	mu           sync.RWMutex
}

type IANAPool struct {
	Name          string
	Network       *net.IPNet
	RangeStart    net.IP
	RangeEnd      net.IP
	Gateway       net.IP
	PreferredTime uint32
	ValidTime     uint32
}

type PDPool struct {
	Name          string
	Network       *net.IPNet
	PrefixLength  uint8
	PreferredTime uint32
	ValidTime     uint32
	nextPrefix    net.IP
}

type IANALease struct {
	Address       net.IP
	DUID          []byte
	IAID          uint32
	SessionID     string
	PoolName      string
	PreferredTime uint32
	ValidTime     uint32
	ExpireTime    time.Time
}

type PDLease struct {
	Prefix        *net.IPNet
	DUID          []byte
	IAID          uint32
	SessionID     string
	PoolName      string
	PreferredTime uint32
	ValidTime     uint32
	ExpireTime    time.Time
}

func New(cfg *config.Config) (dhcp6.DHCPProvider, error) {
	p := &Provider{
		coreConfig:   cfg,
		serverDUID:   generateServerDUID(),
		ianaPools:    make(map[string]*IANAPool),
		pdPools:      make(map[string]*PDPool),
		ianaLeases:   make(map[string]*IANALease),
		pdLeases:     make(map[string]*PDLease),
		leasesByAddr: make(map[string]*IANALease),
		leasesByPfx:  make(map[string]*PDLease),
	}

	for profileName, profile := range cfg.IPv6Profiles {
		for _, poolCfg := range profile.IANAPools {
			if err := p.addIANAPool(poolCfg.Name, poolCfg.Network, poolCfg.RangeStart, poolCfg.RangeEnd, poolCfg.Gateway, poolCfg.PreferredTime, poolCfg.ValidTime); err != nil {
				return nil, fmt.Errorf("profile %s iana pool %s: %w", profileName, poolCfg.Name, err)
			}
		}
		for _, poolCfg := range profile.PDPools {
			if err := p.addPDPool(poolCfg.Name, poolCfg.Network, poolCfg.PrefixLength, poolCfg.PreferredTime, poolCfg.ValidTime); err != nil {
				return nil, fmt.Errorf("profile %s pd pool %s: %w", profileName, poolCfg.Name, err)
			}
		}
	}

	for _, poolCfg := range cfg.DHCPv6.IANAPools {
		if err := p.addIANAPool(poolCfg.Name, poolCfg.Network, poolCfg.RangeStart, poolCfg.RangeEnd, poolCfg.Gateway, poolCfg.PreferredTime, poolCfg.ValidTime); err != nil {
			return nil, fmt.Errorf("iana pool %s: %w", poolCfg.Name, err)
		}
	}

	for _, poolCfg := range cfg.DHCPv6.PDPools {
		if err := p.addPDPool(poolCfg.Name, poolCfg.Network, poolCfg.PrefixLength, poolCfg.PreferredTime, poolCfg.ValidTime); err != nil {
			return nil, fmt.Errorf("pd pool %s: %w", poolCfg.Name, err)
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

func generateServerDUID() []byte {
	duid := make([]byte, 14)
	binary.BigEndian.PutUint16(duid[0:2], 1)
	binary.BigEndian.PutUint16(duid[2:4], 1)
	binary.BigEndian.PutUint32(duid[4:8], uint32(time.Now().Unix()))
	copy(duid[8:14], []byte{0x00, 0x16, 0x3e, 0xaa, 0xbb, 0xcc})
	return duid
}

func (p *Provider) addIANAPool(name, network, rangeStart, rangeEnd, gateway string, preferredTime, validTime uint32) error {
	_, ipnet, err := net.ParseCIDR(network)
	if err != nil {
		return fmt.Errorf("invalid network: %w", err)
	}

	pool := &IANAPool{
		Name:          name,
		Network:       ipnet,
		RangeStart:    net.ParseIP(rangeStart),
		RangeEnd:      net.ParseIP(rangeEnd),
		Gateway:       net.ParseIP(gateway),
		PreferredTime: preferredTime,
		ValidTime:     validTime,
	}

	if pool.PreferredTime == 0 {
		pool.PreferredTime = 3600
	}
	if pool.ValidTime == 0 {
		pool.ValidTime = 7200
	}

	p.ianaPools[name] = pool
	return nil
}

func (p *Provider) addPDPool(name, network string, prefixLength uint8, preferredTime, validTime uint32) error {
	_, ipnet, err := net.ParseCIDR(network)
	if err != nil {
		return fmt.Errorf("invalid network: %w", err)
	}

	pool := &PDPool{
		Name:          name,
		Network:       ipnet,
		PrefixLength:  prefixLength,
		PreferredTime: preferredTime,
		ValidTime:     validTime,
		nextPrefix:    ipnet.IP,
	}

	if pool.PrefixLength == 0 {
		pool.PrefixLength = 64
	}
	if pool.PreferredTime == 0 {
		pool.PreferredTime = 3600
	}
	if pool.ValidTime == 0 {
		pool.ValidTime = 7200
	}

	p.pdPools[name] = pool
	return nil
}

func (p *Provider) HandlePacket(ctx context.Context, pkt *dhcp6.Packet) (*dhcp6.Packet, error) {
	dhcpPkt := gopacket.NewPacket(pkt.Raw, layers.LayerTypeDHCPv6, gopacket.Default)
	layer := dhcpPkt.Layer(layers.LayerTypeDHCPv6)
	if layer == nil {
		return nil, fmt.Errorf("no DHCPv6 layer found")
	}
	dhcp := layer.(*layers.DHCPv6)

	switch dhcp.MsgType {
	case layers.DHCPv6MsgTypeSolicit:
		return p.handleSolicit(pkt, dhcp)
	case layers.DHCPv6MsgTypeRequest:
		return p.handleRequest(pkt, dhcp)
	case layers.DHCPv6MsgTypeRenew:
		return p.handleRenew(pkt, dhcp)
	case layers.DHCPv6MsgTypeRebind:
		return p.handleRebind(pkt, dhcp)
	case layers.DHCPv6MsgTypeRelease:
		return p.handleRelease(pkt, dhcp)
	case layers.DHCPv6MsgTypeDecline:
		return p.handleDecline(pkt, dhcp)
	default:
		return nil, fmt.Errorf("unsupported DHCPv6 message type: %v", dhcp.MsgType)
	}
}

func (p *Provider) handleSolicit(pkt *dhcp6.Packet, dhcp *layers.DHCPv6) (*dhcp6.Packet, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	clientDUID := p.extractClientDUID(dhcp)
	if clientDUID == nil {
		return nil, fmt.Errorf("no client DUID")
	}

	duidKey := string(clientDUID)

	if pkt.Resolved != nil {
		return p.handleSolicitResolved(pkt, dhcp, clientDUID, duidKey)
	}

	var ianaPoolName, pdPoolName string

	var ianaAddr net.IP
	var ianaIAID uint32
	var ianaPool *IANAPool
	if iaid, ok := p.extractIANA(dhcp); ok {
		ianaIAID = iaid
		if lease, exists := p.ianaLeases[duidKey]; exists {
			ianaAddr = lease.Address
			for _, pool := range p.ianaPools {
				if pool.Network.Contains(ianaAddr) {
					ianaPool = pool
					break
				}
			}
		} else {
			ianaPool = p.selectIANAPool(ianaPoolName)
			if ianaPool != nil {
				var err error
				ianaAddr, err = p.allocateIANAAddress(ianaPool, clientDUID, ianaIAID, pkt.SessionID)
				if err != nil {
					return nil, fmt.Errorf("allocate iana: %w", err)
				}
			}
		}
	}

	var pdPrefix *net.IPNet
	var pdIAID uint32
	var pdPool *PDPool
	if iaid, ok := p.extractIAPD(dhcp); ok {
		pdIAID = iaid
		if lease, exists := p.pdLeases[duidKey]; exists {
			pdPrefix = lease.Prefix
			for _, pool := range p.pdPools {
				if pool.Network.Contains(pdPrefix.IP) {
					pdPool = pool
					break
				}
			}
		} else {
			pdPool = p.selectPDPool(pdPoolName)
			if pdPool != nil {
				var err error
				pdPrefix, err = p.allocatePDPrefix(pdPool, clientDUID, pdIAID, pkt.SessionID)
				if err != nil {
					return nil, fmt.Errorf("allocate pd: %w", err)
				}
			}
		}
	}

	response := p.buildAdvertise(dhcp, clientDUID, ianaAddr, ianaIAID, ianaPool, pdPrefix, pdIAID, pdPool, nil)
	return &dhcp6.Packet{
		SessionID: pkt.SessionID,
		MAC:       pkt.MAC,
		SVLAN:     pkt.SVLAN,
		CVLAN:     pkt.CVLAN,
		DUID:      clientDUID,
		Raw:       response,
	}, nil
}

func (p *Provider) handleRequest(pkt *dhcp6.Packet, dhcp *layers.DHCPv6) (*dhcp6.Packet, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	clientDUID := p.extractClientDUID(dhcp)
	if clientDUID == nil {
		return nil, fmt.Errorf("no client DUID")
	}

	duidKey := string(clientDUID)

	if pkt.Resolved != nil {
		return p.handleRequestResolved(pkt, dhcp, clientDUID, duidKey)
	}

	var ianaAddr net.IP
	var ianaIAID uint32
	var ianaPool *IANAPool
	if lease, exists := p.ianaLeases[duidKey]; exists {
		ianaAddr = lease.Address
		ianaIAID = lease.IAID
		lease.ExpireTime = time.Now().Add(time.Duration(lease.ValidTime) * time.Second)
		for _, pool := range p.ianaPools {
			if pool.Network.Contains(ianaAddr) {
				ianaPool = pool
				break
			}
		}
	}

	var pdPrefix *net.IPNet
	var pdIAID uint32
	var pdPool *PDPool
	if lease, exists := p.pdLeases[duidKey]; exists {
		pdPrefix = lease.Prefix
		pdIAID = lease.IAID
		lease.ExpireTime = time.Now().Add(time.Duration(lease.ValidTime) * time.Second)
		for _, pool := range p.pdPools {
			if pool.Network.Contains(pdPrefix.IP) {
				pdPool = pool
				break
			}
		}
	}

	response := p.buildReply(dhcp, clientDUID, ianaAddr, ianaIAID, ianaPool, pdPrefix, pdIAID, pdPool, nil)
	return &dhcp6.Packet{
		SessionID: pkt.SessionID,
		MAC:       pkt.MAC,
		SVLAN:     pkt.SVLAN,
		CVLAN:     pkt.CVLAN,
		DUID:      clientDUID,
		Raw:       response,
	}, nil
}

func (p *Provider) handleRenew(pkt *dhcp6.Packet, dhcp *layers.DHCPv6) (*dhcp6.Packet, error) {
	return p.handleRequest(pkt, dhcp)
}

func (p *Provider) handleRebind(pkt *dhcp6.Packet, dhcp *layers.DHCPv6) (*dhcp6.Packet, error) {
	return p.handleRequest(pkt, dhcp)
}

func (p *Provider) handleRelease(pkt *dhcp6.Packet, dhcp *layers.DHCPv6) (*dhcp6.Packet, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	clientDUID := p.extractClientDUID(dhcp)
	if clientDUID == nil {
		return nil, nil
	}

	duidKey := string(clientDUID)

	if lease, exists := p.ianaLeases[duidKey]; exists {
		if lease.PoolName != "" {
			allocator.GetGlobalRegistry().ReleaseIANA(lease.PoolName, lease.Address)
		}
		delete(p.leasesByAddr, lease.Address.String())
		delete(p.ianaLeases, duidKey)
	}

	if lease, exists := p.pdLeases[duidKey]; exists {
		if lease.PoolName != "" {
			allocator.GetGlobalRegistry().ReleasePD(lease.PoolName, lease.Prefix)
		}
		delete(p.leasesByPfx, lease.Prefix.String())
		delete(p.pdLeases, duidKey)
	}

	reply := p.buildReleaseReply(dhcp, clientDUID)
	return &dhcp6.Packet{
		SessionID: pkt.SessionID,
		MAC:       pkt.MAC,
		SVLAN:     pkt.SVLAN,
		CVLAN:     pkt.CVLAN,
		DUID:      clientDUID,
		Raw:       reply,
	}, nil
}

func (p *Provider) buildReleaseReply(req *layers.DHCPv6, clientDUID []byte) []byte {
	statusData := make([]byte, 2)
	binary.BigEndian.PutUint16(statusData[0:2], StatusSuccess)

	options := []layers.DHCPv6Option{
		layers.NewDHCPv6Option(layers.DHCPv6OptClientID, clientDUID),
		layers.NewDHCPv6Option(layers.DHCPv6OptServerID, p.serverDUID),
		layers.NewDHCPv6Option(layers.DHCPv6Opt(OptStatusCode), statusData),
	}

	dhcpReply := &layers.DHCPv6{
		MsgType:       layers.DHCPv6MsgTypeReply,
		TransactionID: req.TransactionID,
		Options:       options,
	}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true}
	if err := dhcpReply.SerializeTo(buf, opts); err != nil {
		return nil
	}
	return buf.Bytes()
}

func (p *Provider) handleDecline(pkt *dhcp6.Packet, dhcp *layers.DHCPv6) (*dhcp6.Packet, error) {
	return p.handleRelease(pkt, dhcp)
}

func (p *Provider) ReleaseLease(duid []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()

	duidKey := string(duid)

	if lease, exists := p.ianaLeases[duidKey]; exists {
		if lease.PoolName != "" {
			allocator.GetGlobalRegistry().ReleaseIANA(lease.PoolName, lease.Address)
		}
		delete(p.leasesByAddr, lease.Address.String())
		delete(p.ianaLeases, duidKey)
	}

	if lease, exists := p.pdLeases[duidKey]; exists {
		if lease.PoolName != "" {
			allocator.GetGlobalRegistry().ReleasePD(lease.PoolName, lease.Prefix)
		}
		delete(p.leasesByPfx, lease.Prefix.String())
		delete(p.pdLeases, duidKey)
	}
}

func (p *Provider) extractClientDUID(dhcp *layers.DHCPv6) []byte {
	for _, opt := range dhcp.Options {
		if opt.Code == layers.DHCPv6OptClientID {
			return opt.Data
		}
	}
	return nil
}

func (p *Provider) extractIANA(dhcp *layers.DHCPv6) (uint32, bool) {
	for _, opt := range dhcp.Options {
		if opt.Code == layers.DHCPv6OptIANA && len(opt.Data) >= 4 {
			return binary.BigEndian.Uint32(opt.Data[0:4]), true
		}
	}
	return 0, false
}

func (p *Provider) extractIAPD(dhcp *layers.DHCPv6) (uint32, bool) {
	for _, opt := range dhcp.Options {
		if opt.Code == layers.DHCPv6OptIAPD && len(opt.Data) >= 4 {
			return binary.BigEndian.Uint32(opt.Data[0:4]), true
		}
	}
	return 0, false
}

func (p *Provider) selectIANAPool(poolName string) *IANAPool {
	if poolName != "" {
		if pool, ok := p.ianaPools[poolName]; ok {
			return pool
		}
	}
	for _, pool := range p.ianaPools {
		return pool
	}
	return nil
}

func (p *Provider) selectPDPool(poolName string) *PDPool {
	if poolName != "" {
		if pool, ok := p.pdPools[poolName]; ok {
			return pool
		}
	}
	for _, pool := range p.pdPools {
		return pool
	}
	return nil
}

func (p *Provider) allocateIANAAddress(pool *IANAPool, duid []byte, iaid uint32, sessionID string) (net.IP, error) {
	start := pool.RangeStart.To16()
	end := pool.RangeEnd.To16()

	for ip := dupIP(start); compareIP(ip, end) <= 0; incIP(ip) {
		if _, used := p.leasesByAddr[ip.String()]; !used {
			lease := &IANALease{
				Address:       dupIP(ip),
				DUID:          duid,
				IAID:          iaid,
				SessionID:     sessionID,
				PreferredTime: pool.PreferredTime,
				ValidTime:     pool.ValidTime,
				ExpireTime:    time.Now().Add(time.Duration(pool.ValidTime) * time.Second),
			}
			p.ianaLeases[string(duid)] = lease
			p.leasesByAddr[ip.String()] = lease
			return lease.Address, nil
		}
	}

	return nil, fmt.Errorf("no available addresses")
}

func (p *Provider) allocatePDPrefix(pool *PDPool, duid []byte, iaid uint32, sessionID string) (*net.IPNet, error) {
	poolSize, _ := pool.Network.Mask.Size()
	step := uint(128 - pool.PrefixLength)

	for {
		prefix := &net.IPNet{
			IP:   dupIP(pool.nextPrefix),
			Mask: net.CIDRMask(int(pool.PrefixLength), 128),
		}

		if !pool.Network.Contains(prefix.IP) {
			return nil, fmt.Errorf("no available prefixes")
		}

		if _, used := p.leasesByPfx[prefix.String()]; !used {
			lease := &PDLease{
				Prefix:        prefix,
				DUID:          duid,
				IAID:          iaid,
				SessionID:     sessionID,
				PreferredTime: pool.PreferredTime,
				ValidTime:     pool.ValidTime,
				ExpireTime:    time.Now().Add(time.Duration(pool.ValidTime) * time.Second),
			}
			p.pdLeases[string(duid)] = lease
			p.leasesByPfx[prefix.String()] = lease
			incIPBy(pool.nextPrefix, step)
			return prefix, nil
		}

		incIPBy(pool.nextPrefix, step)

		if !pool.Network.Contains(pool.nextPrefix) {
			pool.nextPrefix = dupIP(pool.Network.IP)
			ones, _ := pool.Network.Mask.Size()
			if ones == poolSize {
				return nil, fmt.Errorf("prefix pool exhausted")
			}
		}
	}
}

func (p *Provider) buildAdvertise(req *layers.DHCPv6, clientDUID []byte, ianaAddr net.IP, ianaIAID uint32, ianaPool *IANAPool, pdPrefix *net.IPNet, pdIAID uint32, pdPool *PDPool, dnsOverride []net.IP) []byte {
	return p.buildResponse(req, layers.DHCPv6MsgTypeAdverstise, clientDUID, ianaAddr, ianaIAID, ianaPool, pdPrefix, pdIAID, pdPool, dnsOverride)
}

func (p *Provider) buildReply(req *layers.DHCPv6, clientDUID []byte, ianaAddr net.IP, ianaIAID uint32, ianaPool *IANAPool, pdPrefix *net.IPNet, pdIAID uint32, pdPool *PDPool, dnsOverride []net.IP) []byte {
	return p.buildResponse(req, layers.DHCPv6MsgTypeReply, clientDUID, ianaAddr, ianaIAID, ianaPool, pdPrefix, pdIAID, pdPool, dnsOverride)
}

func (p *Provider) buildResponse(req *layers.DHCPv6, msgType layers.DHCPv6MsgType, clientDUID []byte, ianaAddr net.IP, ianaIAID uint32, ianaPool *IANAPool, pdPrefix *net.IPNet, pdIAID uint32, pdPool *PDPool, dnsOverride []net.IP) []byte {
	var options []layers.DHCPv6Option

	options = append(options, layers.NewDHCPv6Option(layers.DHCPv6OptClientID, clientDUID))
	options = append(options, layers.NewDHCPv6Option(layers.DHCPv6OptServerID, p.serverDUID))

	if ianaAddr != nil && ianaPool != nil {
		ianaData := p.buildIANAOption(ianaIAID, ianaAddr, ianaPool)
		options = append(options, layers.NewDHCPv6Option(layers.DHCPv6OptIANA, ianaData))
	}

	if pdPrefix != nil && pdPool != nil {
		pdData := p.buildIAPDOption(pdIAID, pdPrefix, pdPool)
		options = append(options, layers.NewDHCPv6Option(layers.DHCPv6OptIAPD, pdData))
	}

	var dnsData []byte
	if len(dnsOverride) > 0 {
		dnsData = make([]byte, 0, len(dnsOverride)*16)
		for _, ip := range dnsOverride {
			dnsData = append(dnsData, ip.To16()...)
		}
	} else if len(p.coreConfig.DHCPv6.DNSServers) > 0 {
		dnsData = make([]byte, 0, len(p.coreConfig.DHCPv6.DNSServers)*16)
		for _, dns := range p.coreConfig.DHCPv6.DNSServers {
			ip := net.ParseIP(dns)
			if ip != nil {
				dnsData = append(dnsData, ip.To16()...)
			}
		}
	}
	if len(dnsData) > 0 {
		options = append(options, layers.NewDHCPv6Option(layers.DHCPv6OptDNSServers, dnsData))
	}

	dhcpReply := &layers.DHCPv6{
		MsgType:       msgType,
		TransactionID: req.TransactionID,
		Options:       options,
	}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths: true,
	}

	if err := dhcpReply.SerializeTo(buf, opts); err != nil {
		return nil
	}

	return buf.Bytes()
}

func (p *Provider) buildIANAOption(iaid uint32, addr net.IP, pool *IANAPool) []byte {
	t1 := pool.PreferredTime / 2
	t2 := uint32(float64(pool.PreferredTime) * 0.8)

	iaaddrLen := 24
	data := make([]byte, 12+4+iaaddrLen)

	binary.BigEndian.PutUint32(data[0:4], iaid)
	binary.BigEndian.PutUint32(data[4:8], t1)
	binary.BigEndian.PutUint32(data[8:12], t2)

	binary.BigEndian.PutUint16(data[12:14], uint16(OptIAAddr))
	binary.BigEndian.PutUint16(data[14:16], uint16(iaaddrLen))
	copy(data[16:32], addr.To16())
	binary.BigEndian.PutUint32(data[32:36], pool.PreferredTime)
	binary.BigEndian.PutUint32(data[36:40], pool.ValidTime)

	return data
}

func (p *Provider) buildIAPDOption(iaid uint32, prefix *net.IPNet, pool *PDPool) []byte {
	t1 := pool.PreferredTime / 2
	t2 := uint32(float64(pool.PreferredTime) * 0.8)

	prefixLen, _ := prefix.Mask.Size()
	iaprefixLen := 25

	data := make([]byte, 12+4+iaprefixLen)

	binary.BigEndian.PutUint32(data[0:4], iaid)
	binary.BigEndian.PutUint32(data[4:8], t1)
	binary.BigEndian.PutUint32(data[8:12], t2)

	binary.BigEndian.PutUint16(data[12:14], uint16(OptIAPrefix))
	binary.BigEndian.PutUint16(data[14:16], uint16(iaprefixLen))
	binary.BigEndian.PutUint32(data[16:20], pool.PreferredTime)
	binary.BigEndian.PutUint32(data[20:24], pool.ValidTime)
	data[24] = uint8(prefixLen)
	copy(data[25:41], prefix.IP.To16())

	return data
}

func (p *Provider) handleSolicitResolved(pkt *dhcp6.Packet, dhcpReq *layers.DHCPv6, clientDUID []byte, duidKey string) (*dhcp6.Packet, error) {
	var ianaAddr net.IP
	var ianaIAID uint32
	var ianaPool *IANAPool
	if iaid, ok := p.extractIANA(dhcpReq); ok && pkt.Resolved.IANAAddress != nil {
		ianaIAID = iaid
		ianaAddr = pkt.Resolved.IANAAddress
		ianaPool = &IANAPool{
			PreferredTime: pkt.Resolved.IANAPreferredTime,
			ValidTime:     pkt.Resolved.IANAValidTime,
		}
		if err := p.reserveIANA(ianaAddr, clientDUID, ianaIAID, pkt.SessionID, pkt.Resolved.IANAPoolName, pkt.Resolved.IANAPreferredTime, pkt.Resolved.IANAValidTime); err != nil {
			return nil, fmt.Errorf("reserve IANA: %w", err)
		}
	}

	var pdPrefix *net.IPNet
	var pdIAID uint32
	var pdPool *PDPool
	if iaid, ok := p.extractIAPD(dhcpReq); ok && pkt.Resolved.PDPrefix != nil {
		pdIAID = iaid
		pdPrefix = pkt.Resolved.PDPrefix
		pdPool = &PDPool{
			PreferredTime: pkt.Resolved.PDPreferredTime,
			ValidTime:     pkt.Resolved.PDValidTime,
		}
		if err := p.reservePD(pdPrefix, clientDUID, pdIAID, pkt.SessionID, pkt.Resolved.PDPoolName, pkt.Resolved.PDPreferredTime, pkt.Resolved.PDValidTime); err != nil {
			return nil, fmt.Errorf("reserve PD: %w", err)
		}
	}

	response := p.buildAdvertise(dhcpReq, clientDUID, ianaAddr, ianaIAID, ianaPool, pdPrefix, pdIAID, pdPool, pkt.Resolved.DNS)
	return &dhcp6.Packet{
		SessionID: pkt.SessionID,
		MAC:       pkt.MAC,
		SVLAN:     pkt.SVLAN,
		CVLAN:     pkt.CVLAN,
		DUID:      clientDUID,
		Raw:       response,
	}, nil
}

func (p *Provider) handleRequestResolved(pkt *dhcp6.Packet, dhcpReq *layers.DHCPv6, clientDUID []byte, duidKey string) (*dhcp6.Packet, error) {
	var ianaAddr net.IP
	var ianaIAID uint32
	var ianaPool *IANAPool
	if iaid, ok := p.extractIANA(dhcpReq); ok && pkt.Resolved.IANAAddress != nil {
		ianaIAID = iaid
		ianaAddr = pkt.Resolved.IANAAddress
		ianaPool = &IANAPool{
			PreferredTime: pkt.Resolved.IANAPreferredTime,
			ValidTime:     pkt.Resolved.IANAValidTime,
		}
		if err := p.reserveIANA(ianaAddr, clientDUID, ianaIAID, pkt.SessionID, pkt.Resolved.IANAPoolName, pkt.Resolved.IANAPreferredTime, pkt.Resolved.IANAValidTime); err != nil {
			return nil, fmt.Errorf("reserve IANA: %w", err)
		}
	}

	var pdPrefix *net.IPNet
	var pdIAID uint32
	var pdPool *PDPool
	if iaid, ok := p.extractIAPD(dhcpReq); ok && pkt.Resolved.PDPrefix != nil {
		pdIAID = iaid
		pdPrefix = pkt.Resolved.PDPrefix
		pdPool = &PDPool{
			PreferredTime: pkt.Resolved.PDPreferredTime,
			ValidTime:     pkt.Resolved.PDValidTime,
		}
		if err := p.reservePD(pdPrefix, clientDUID, pdIAID, pkt.SessionID, pkt.Resolved.PDPoolName, pkt.Resolved.PDPreferredTime, pkt.Resolved.PDValidTime); err != nil {
			return nil, fmt.Errorf("reserve PD: %w", err)
		}
	}

	response := p.buildReply(dhcpReq, clientDUID, ianaAddr, ianaIAID, ianaPool, pdPrefix, pdIAID, pdPool, pkt.Resolved.DNS)
	return &dhcp6.Packet{
		SessionID: pkt.SessionID,
		MAC:       pkt.MAC,
		SVLAN:     pkt.SVLAN,
		CVLAN:     pkt.CVLAN,
		DUID:      clientDUID,
		Raw:       response,
	}, nil
}

func (p *Provider) reserveIANA(addr net.IP, duid []byte, iaid uint32, sessionID, poolName string, preferredTime, validTime uint32) error {
	duidKey := string(duid)
	addrKey := addr.String()

	if existing, exists := p.leasesByAddr[addrKey]; exists && existing.SessionID != sessionID {
		return fmt.Errorf("address %s already leased to session %s", addrKey, existing.SessionID)
	}

	lease := &IANALease{
		Address:       dupIP(addr),
		DUID:          duid,
		IAID:          iaid,
		SessionID:     sessionID,
		PoolName:      poolName,
		PreferredTime: preferredTime,
		ValidTime:     validTime,
		ExpireTime:    time.Now().Add(time.Duration(validTime) * time.Second),
	}
	p.ianaLeases[duidKey] = lease
	p.leasesByAddr[addrKey] = lease
	return nil
}

func (p *Provider) reservePD(prefix *net.IPNet, duid []byte, iaid uint32, sessionID, poolName string, preferredTime, validTime uint32) error {
	duidKey := string(duid)
	pfxKey := prefix.String()

	if existing, exists := p.leasesByPfx[pfxKey]; exists && existing.SessionID != sessionID {
		return fmt.Errorf("prefix %s already leased to session %s", pfxKey, existing.SessionID)
	}

	lease := &PDLease{
		Prefix:        prefix,
		DUID:          duid,
		IAID:          iaid,
		SessionID:     sessionID,
		PoolName:      poolName,
		PreferredTime: preferredTime,
		ValidTime:     validTime,
		ExpireTime:    time.Now().Add(time.Duration(validTime) * time.Second),
	}
	p.pdLeases[duidKey] = lease
	p.leasesByPfx[pfxKey] = lease
	return nil
}

func dupIP(ip net.IP) net.IP {
	dup := make(net.IP, len(ip))
	copy(dup, ip)
	return dup
}

func incIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
}

func incIPBy(ip net.IP, bits uint) {
	if bits == 0 {
		incIP(ip)
		return
	}

	byteIndex := 15 - (bits / 8)
	bitOffset := bits % 8
	increment := uint16(1) << bitOffset

	for i := int(byteIndex); i >= 0; i-- {
		val := uint16(ip[i]) + increment
		ip[i] = byte(val & 0xff)
		increment = val >> 8
		if increment == 0 {
			break
		}
	}
}

func compareIP(a, b net.IP) int {
	a = a.To16()
	b = b.To16()
	for i := 0; i < 16; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}
