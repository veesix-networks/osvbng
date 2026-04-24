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
	"github.com/veesix-networks/osvbng/pkg/dhcp/relay"
	dhcp6msg "github.com/veesix-networks/osvbng/pkg/dhcp6"
	"github.com/veesix-networks/osvbng/pkg/provider"
)

func init() {
	dhcp6msg.Register("local", New)
}

const (
	StatusSuccess       uint16 = 0
	StatusNoAddrsAvail  uint16 = 2
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

func New(cfg *config.Config) (dhcp6msg.DHCPProvider, error) {
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

func (p *Provider) HandlePacket(ctx context.Context, pkt *dhcp6msg.Packet) (*dhcp6msg.Packet, error) {
	msg, err := dhcp6msg.ParseMessage(pkt.Raw)
	if err != nil {
		return nil, fmt.Errorf("parse DHCPv6: %w", err)
	}

	var resp *dhcp6msg.Packet
	switch msg.MsgType {
	case dhcp6msg.MsgTypeSolicit:
		resp, err = p.handleSolicit(pkt, msg)
	case dhcp6msg.MsgTypeRequest:
		resp, err = p.handleRequest(pkt, msg)
	case dhcp6msg.MsgTypeRenew:
		resp, err = p.handleRenew(pkt, msg)
	case dhcp6msg.MsgTypeRebind:
		resp, err = p.handleRebind(pkt, msg)
	case dhcp6msg.MsgTypeRelease:
		resp, err = p.handleRelease(pkt, msg)
	case dhcp6msg.MsgTypeDecline:
		resp, err = p.handleDecline(pkt, msg)
	default:
		return nil, fmt.Errorf("unsupported DHCPv6 message type: %v", msg.MsgType)
	}
	if err != nil || resp == nil || len(resp.Raw) == 0 || pkt.RelayInfo == nil {
		return resp, err
	}
	if wrapped := relay.BuildRelayReply(resp.Raw, pkt.RelayInfo); wrapped != nil {
		resp.Raw = wrapped
	}
	return resp, nil
}

// readExistingLeases reads IANA/PD leases and their pools for a DUID.
// Caller must hold p.mu (read or write).
func (p *Provider) readExistingLeases(msg *dhcp6msg.Message, duidKey string) (net.IP, uint32, *IANAPool, *net.IPNet, uint32, *PDPool) {
	var ianaAddr net.IP
	var ianaIAID uint32
	var ianaPool *IANAPool
	if msg.Options.IANA != nil {
		if lease := p.ianaLeases[duidKey]; lease != nil {
			ianaIAID = msg.Options.IANA.IAID
			ianaAddr = lease.Address
			for _, pool := range p.ianaPools {
				if pool.Network.Contains(ianaAddr) {
					ianaPool = pool
					break
				}
			}
		}
	}
	var pdPrefix *net.IPNet
	var pdIAID uint32
	var pdPool *PDPool
	if msg.Options.IAPD != nil {
		if lease := p.pdLeases[duidKey]; lease != nil {
			pdIAID = msg.Options.IAPD.IAID
			pdPrefix = lease.Prefix
			for _, pool := range p.pdPools {
				if pool.Network.Contains(pdPrefix.IP) {
					pdPool = pool
					break
				}
			}
		}
	}
	return ianaAddr, ianaIAID, ianaPool, pdPrefix, pdIAID, pdPool
}

func (p *Provider) handleSolicit(pkt *dhcp6msg.Packet, msg *dhcp6msg.Message) (*dhcp6msg.Packet, error) {
	clientDUID := msg.Options.ClientID
	if clientDUID == nil {
		return nil, fmt.Errorf("no client DUID")
	}

	duidKey := string(clientDUID)

	if pkt.Resolved != nil {
		// Fast path: if leases already exist for this session (retry Solicit),
		// build response without exclusive lock. reserveIANA/reservePD would
		// just overwrite with the same data.
		p.mu.RLock()
		existingIANA := p.ianaLeases[duidKey]
		existingPD := p.pdLeases[duidKey]
		p.mu.RUnlock()

		alreadyReserved := true
		if msg.Options.IANA != nil && pkt.Resolved.IANAAddress != nil {
			if existingIANA == nil || existingIANA.SessionID != pkt.SessionID {
				alreadyReserved = false
			}
		}
		if msg.Options.IAPD != nil && pkt.Resolved.PDPrefix != nil {
			if existingPD == nil || existingPD.SessionID != pkt.SessionID {
				alreadyReserved = false
			}
		}

		if alreadyReserved {
			return p.buildSolicitResolvedResponse(pkt, msg, clientDUID)
		}

		p.mu.Lock()
		defer p.mu.Unlock()
		return p.handleSolicitResolved(pkt, msg, clientDUID, duidKey)
	}

	// Non-resolved: check for existing leases under single read lock (retry case).
	p.mu.RLock()
	if p.ianaLeases[duidKey] != nil || p.pdLeases[duidKey] != nil {
		ianaAddr, ianaIAID, ianaPool, pdPrefix, pdIAID, pdPool := p.readExistingLeases(msg, duidKey)
		p.mu.RUnlock()
		response := p.buildAdvertise(msg, clientDUID, ianaAddr, ianaIAID, ianaPool, pdPrefix, pdIAID, pdPool, nil)
		return &dhcp6msg.Packet{
			SessionID: pkt.SessionID,
			MAC:       pkt.MAC,
			SVLAN:     pkt.SVLAN,
			CVLAN:     pkt.CVLAN,
			DUID:      clientDUID,
			Raw:       response,
		}, nil
	}
	p.mu.RUnlock()

	// First allocation needs exclusive lock.
	p.mu.Lock()
	defer p.mu.Unlock()

	// Re-check under write lock in case another goroutine allocated.
	if p.ianaLeases[duidKey] != nil || p.pdLeases[duidKey] != nil {
		ianaAddr, ianaIAID, ianaPool, pdPrefix, pdIAID, pdPool := p.readExistingLeases(msg, duidKey)
		response := p.buildAdvertise(msg, clientDUID, ianaAddr, ianaIAID, ianaPool, pdPrefix, pdIAID, pdPool, nil)
		return &dhcp6msg.Packet{
			SessionID: pkt.SessionID,
			MAC:       pkt.MAC,
			SVLAN:     pkt.SVLAN,
			CVLAN:     pkt.CVLAN,
			DUID:      clientDUID,
			Raw:       response,
		}, nil
	}

	var ianaPoolName, pdPoolName string

	var ianaAddr net.IP
	var ianaIAID uint32
	var ianaPool *IANAPool
	if msg.Options.IANA != nil {
		ianaIAID = msg.Options.IANA.IAID
		ianaPool = p.selectIANAPool(ianaPoolName)
		if ianaPool != nil {
			var err error
			ianaAddr, err = p.allocateIANAAddress(ianaPool, clientDUID, ianaIAID, pkt.SessionID)
			if err != nil {
				return nil, fmt.Errorf("allocate iana: %w", err)
			}
		}
	}

	var pdPrefix *net.IPNet
	var pdIAID uint32
	var pdPool *PDPool
	if msg.Options.IAPD != nil {
		pdIAID = msg.Options.IAPD.IAID
		pdPool = p.selectPDPool(pdPoolName)
		if pdPool != nil {
			var err error
			pdPrefix, err = p.allocatePDPrefix(pdPool, clientDUID, pdIAID, pkt.SessionID)
			if err != nil {
				return nil, fmt.Errorf("allocate pd: %w", err)
			}
		}
	}

	response := p.buildAdvertise(msg, clientDUID, ianaAddr, ianaIAID, ianaPool, pdPrefix, pdIAID, pdPool, nil)
	return &dhcp6msg.Packet{
		SessionID: pkt.SessionID,
		MAC:       pkt.MAC,
		SVLAN:     pkt.SVLAN,
		CVLAN:     pkt.CVLAN,
		DUID:      clientDUID,
		Raw:       response,
	}, nil
}

func (p *Provider) handleRequest(pkt *dhcp6msg.Packet, msg *dhcp6msg.Message) (*dhcp6msg.Packet, error) {
	clientDUID := msg.Options.ClientID
	if clientDUID == nil {
		return nil, fmt.Errorf("no client DUID")
	}

	duidKey := string(clientDUID)

	if pkt.Resolved != nil {
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.handleRequestResolved(pkt, msg, clientDUID, duidKey)
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

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

	response := p.buildReply(msg, clientDUID, ianaAddr, ianaIAID, ianaPool, pdPrefix, pdIAID, pdPool, nil)
	return &dhcp6msg.Packet{
		SessionID: pkt.SessionID,
		MAC:       pkt.MAC,
		SVLAN:     pkt.SVLAN,
		CVLAN:     pkt.CVLAN,
		DUID:      clientDUID,
		Raw:       response,
	}, nil
}

func (p *Provider) handleRenew(pkt *dhcp6msg.Packet, msg *dhcp6msg.Message) (*dhcp6msg.Packet, error) {
	return p.handleRequest(pkt, msg)
}

func (p *Provider) handleRebind(pkt *dhcp6msg.Packet, msg *dhcp6msg.Message) (*dhcp6msg.Packet, error) {
	return p.handleRequest(pkt, msg)
}

func (p *Provider) handleRelease(pkt *dhcp6msg.Packet, msg *dhcp6msg.Message) (*dhcp6msg.Packet, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	clientDUID := msg.Options.ClientID
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

	reply := p.buildReleaseReply(msg, clientDUID)
	return &dhcp6msg.Packet{
		SessionID: pkt.SessionID,
		MAC:       pkt.MAC,
		SVLAN:     pkt.SVLAN,
		CVLAN:     pkt.CVLAN,
		DUID:      clientDUID,
		Raw:       reply,
	}, nil
}

func (p *Provider) buildReleaseReply(req *dhcp6msg.Message, clientDUID []byte) []byte {
	resp := &dhcp6msg.Response{
		MsgType:       dhcp6msg.MsgTypeReply,
		TransactionID: req.TransactionID,
		ClientID:      clientDUID,
		ServerID:      p.serverDUID,
		StatusCode: &dhcp6msg.StatusCodeOption{
			Code: StatusSuccess,
		},
	}
	return resp.Serialize()
}

func (p *Provider) handleDecline(pkt *dhcp6msg.Packet, msg *dhcp6msg.Message) (*dhcp6msg.Packet, error) {
	return p.handleRelease(pkt, msg)
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

func (p *Provider) buildAdvertise(req *dhcp6msg.Message, clientDUID []byte, ianaAddr net.IP, ianaIAID uint32, ianaPool *IANAPool, pdPrefix *net.IPNet, pdIAID uint32, pdPool *PDPool, dnsOverride []net.IP) []byte {
	return p.buildResponse(req, dhcp6msg.MsgTypeAdvertise, clientDUID, ianaAddr, ianaIAID, ianaPool, pdPrefix, pdIAID, pdPool, dnsOverride)
}

func (p *Provider) buildReply(req *dhcp6msg.Message, clientDUID []byte, ianaAddr net.IP, ianaIAID uint32, ianaPool *IANAPool, pdPrefix *net.IPNet, pdIAID uint32, pdPool *PDPool, dnsOverride []net.IP) []byte {
	return p.buildResponse(req, dhcp6msg.MsgTypeReply, clientDUID, ianaAddr, ianaIAID, ianaPool, pdPrefix, pdIAID, pdPool, dnsOverride)
}

func (p *Provider) buildResponse(req *dhcp6msg.Message, msgType dhcp6msg.MessageType, clientDUID []byte, ianaAddr net.IP, ianaIAID uint32, ianaPool *IANAPool, pdPrefix *net.IPNet, pdIAID uint32, pdPool *PDPool, dnsOverride []net.IP) []byte {
	resp := &dhcp6msg.Response{
		MsgType:       msgType,
		TransactionID: req.TransactionID,
		ClientID:      clientDUID,
		ServerID:      p.serverDUID,
	}

	if ianaAddr != nil && ianaPool != nil {
		t1 := ianaPool.PreferredTime / 2
		t2 := uint32(float64(ianaPool.PreferredTime) * 0.8)
		resp.IANA = &dhcp6msg.IANAOption{
			IAID:          ianaIAID,
			T1:            t1,
			T2:            t2,
			Address:       ianaAddr,
			PreferredTime: ianaPool.PreferredTime,
			ValidTime:     ianaPool.ValidTime,
		}
	}

	if pdPrefix != nil && pdPool != nil {
		t1 := pdPool.PreferredTime / 2
		t2 := uint32(float64(pdPool.PreferredTime) * 0.8)
		prefixLen, _ := pdPrefix.Mask.Size()
		resp.IAPD = &dhcp6msg.IAPDOption{
			IAID:          pdIAID,
			T1:            t1,
			T2:            t2,
			PrefixLen:     uint8(prefixLen),
			Prefix:        pdPrefix.IP,
			PreferredTime: pdPool.PreferredTime,
			ValidTime:     pdPool.ValidTime,
		}
	}

	if len(dnsOverride) > 0 {
		resp.DNS = dnsOverride
	} else if len(p.coreConfig.DHCPv6.DNSServers) > 0 {
		for _, dns := range p.coreConfig.DHCPv6.DNSServers {
			ip := net.ParseIP(dns)
			if ip != nil {
				resp.DNS = append(resp.DNS, ip)
			}
		}
	}

	return resp.Serialize()
}

func (p *Provider) buildSolicitResolvedResponse(pkt *dhcp6msg.Packet, msg *dhcp6msg.Message, clientDUID []byte) (*dhcp6msg.Packet, error) {
	var ianaAddr net.IP
	var ianaIAID uint32
	var ianaPool *IANAPool
	if msg.Options.IANA != nil && pkt.Resolved.IANAAddress != nil {
		ianaIAID = msg.Options.IANA.IAID
		ianaAddr = pkt.Resolved.IANAAddress
		ianaPool = &IANAPool{
			PreferredTime: pkt.Resolved.IANAPreferredTime,
			ValidTime:     pkt.Resolved.IANAValidTime,
		}
	}

	var pdPrefix *net.IPNet
	var pdIAID uint32
	var pdPool *PDPool
	if msg.Options.IAPD != nil && pkt.Resolved.PDPrefix != nil {
		pdIAID = msg.Options.IAPD.IAID
		pdPrefix = pkt.Resolved.PDPrefix
		pdPool = &PDPool{
			PreferredTime: pkt.Resolved.PDPreferredTime,
			ValidTime:     pkt.Resolved.PDValidTime,
		}
	}

	response := p.buildAdvertise(msg, clientDUID, ianaAddr, ianaIAID, ianaPool, pdPrefix, pdIAID, pdPool, pkt.Resolved.DNS)
	return &dhcp6msg.Packet{
		SessionID: pkt.SessionID,
		MAC:       pkt.MAC,
		SVLAN:     pkt.SVLAN,
		CVLAN:     pkt.CVLAN,
		DUID:      clientDUID,
		Raw:       response,
	}, nil
}

func (p *Provider) handleSolicitResolved(pkt *dhcp6msg.Packet, msg *dhcp6msg.Message, clientDUID []byte, duidKey string) (*dhcp6msg.Packet, error) {
	var ianaAddr net.IP
	var ianaIAID uint32
	var ianaPool *IANAPool
	if msg.Options.IANA != nil && pkt.Resolved.IANAAddress != nil {
		ianaIAID = msg.Options.IANA.IAID
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
	if msg.Options.IAPD != nil && pkt.Resolved.PDPrefix != nil {
		pdIAID = msg.Options.IAPD.IAID
		pdPrefix = pkt.Resolved.PDPrefix
		pdPool = &PDPool{
			PreferredTime: pkt.Resolved.PDPreferredTime,
			ValidTime:     pkt.Resolved.PDValidTime,
		}
		if err := p.reservePD(pdPrefix, clientDUID, pdIAID, pkt.SessionID, pkt.Resolved.PDPoolName, pkt.Resolved.PDPreferredTime, pkt.Resolved.PDValidTime); err != nil {
			return nil, fmt.Errorf("reserve PD: %w", err)
		}
	}

	response := p.buildAdvertise(msg, clientDUID, ianaAddr, ianaIAID, ianaPool, pdPrefix, pdIAID, pdPool, pkt.Resolved.DNS)
	return &dhcp6msg.Packet{
		SessionID: pkt.SessionID,
		MAC:       pkt.MAC,
		SVLAN:     pkt.SVLAN,
		CVLAN:     pkt.CVLAN,
		DUID:      clientDUID,
		Raw:       response,
	}, nil
}

func (p *Provider) handleRequestResolved(pkt *dhcp6msg.Packet, msg *dhcp6msg.Message, clientDUID []byte, duidKey string) (*dhcp6msg.Packet, error) {
	var ianaAddr net.IP
	var ianaIAID uint32
	var ianaPool *IANAPool
	if msg.Options.IANA != nil && pkt.Resolved.IANAAddress != nil {
		ianaIAID = msg.Options.IANA.IAID
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
	if msg.Options.IAPD != nil && pkt.Resolved.PDPrefix != nil {
		pdIAID = msg.Options.IAPD.IAID
		pdPrefix = pkt.Resolved.PDPrefix
		pdPool = &PDPool{
			PreferredTime: pkt.Resolved.PDPreferredTime,
			ValidTime:     pkt.Resolved.PDValidTime,
		}
		if err := p.reservePD(pdPrefix, clientDUID, pdIAID, pkt.SessionID, pkt.Resolved.PDPoolName, pkt.Resolved.PDPreferredTime, pkt.Resolved.PDValidTime); err != nil {
			return nil, fmt.Errorf("reserve PD: %w", err)
		}
	}

	response := p.buildReply(msg, clientDUID, ianaAddr, ianaIAID, ianaPool, pdPrefix, pdIAID, pdPool, pkt.Resolved.DNS)
	return &dhcp6msg.Packet{
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
