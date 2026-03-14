// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package radius

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"regexp"
	"sync/atomic"
	"time"

	internalaaa "github.com/veesix-networks/osvbng/internal/aaa"
	"github.com/veesix-networks/osvbng/pkg/aaa"
	"github.com/veesix-networks/osvbng/pkg/auth"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/provider"
	"layeh.com/radius"
)

var globalProvider atomic.Pointer[Provider]

func GetProvider() *Provider {
	return globalProvider.Load()
}

type compiledCustomMapping struct {
	vendorID   uint32
	vendorType byte
	internal   string
	extract    *regexp.Regexp
}

type compiledRequestMapping struct {
	internal   string
	attrType   radius.Type
	vendorID   uint32
	vendorType byte
}

type Provider struct {
	cfg       *Config
	globalCfg *config.Config
	logger    *slog.Logger

	authConns       []*radiusConn
	acctConns       []*radiusConn
	tier1Index      map[byte]*responseMapping
	tier2Index      map[vendorKey]*vendorMapping
	tier3           []compiledCustomMapping
	requestMappings []compiledRequestMapping

	radiusStats *internalaaa.RADIUSStats
}

func New(cfg *config.Config) (auth.AuthProvider, error) {
	pluginCfgRaw, ok := configmgr.GetPluginConfig(Namespace)
	if !ok {
		return nil, nil
	}

	pluginCfg, ok := pluginCfgRaw.(*Config)
	if !ok {
		return nil, fmt.Errorf("invalid config type for %s", Namespace)
	}

	pluginCfg.applyDefaults()
	if err := pluginCfg.validate(); err != nil {
		return nil, fmt.Errorf("radius config: %w", err)
	}

	if pluginCfg.NASIdentifier == "" {
		pluginCfg.NASIdentifier = cfg.AAA.NASIdentifier
	}
	if pluginCfg.NASIP == "" {
		pluginCfg.NASIP = cfg.AAA.NASIP
	}

	stats := internalaaa.NewRADIUSStats()

	authConns := make([]*radiusConn, 0, len(pluginCfg.Servers))
	acctConns := make([]*radiusConn, 0, len(pluginCfg.Servers))

	for _, s := range pluginCfg.Servers {
		authAddr := net.JoinHostPort(s.Host, fmt.Sprintf("%d", pluginCfg.AuthPort))
		acctAddr := net.JoinHostPort(s.Host, fmt.Sprintf("%d", pluginCfg.AcctPort))
		secret := []byte(s.Secret)

		ac, err := newRadiusConn(authAddr, secret, pluginCfg.Timeout)
		if err != nil {
			for _, c := range authConns {
				_ = c.close()
			}
			for _, c := range acctConns {
				_ = c.close()
			}
			return nil, fmt.Errorf("auth conn to %s: %w", authAddr, err)
		}
		authConns = append(authConns, ac)

		acc, err := newRadiusConn(acctAddr, secret, pluginCfg.Timeout)
		if err != nil {
			for _, c := range authConns {
				_ = c.close()
			}
			for _, c := range acctConns {
				_ = c.close()
			}
			return nil, fmt.Errorf("acct conn to %s: %w", acctAddr, err)
		}
		acctConns = append(acctConns, acc)
	}

	tier1 := buildTier1Index()
	var tier3 []compiledCustomMapping
	for _, m := range pluginCfg.ResponseMappings {
		if m.RadiusAttr != "" && m.VendorID == 0 {
			attrType, ok := resolveAttrName(m.RadiusAttr)
			if !ok {
				return nil, fmt.Errorf("response_mappings: unknown radius attribute %q", m.RadiusAttr)
			}
			tier1[byte(attrType)] = &responseMapping{
				attrType: byte(attrType),
				internal: m.Internal,
				decode:   decodeString,
			}
			continue
		}
		cm := compiledCustomMapping{
			vendorID:   m.VendorID,
			vendorType: m.VendorType,
			internal:   m.Internal,
		}
		if m.Extract != "" {
			re, err := regexp.Compile(m.Extract)
			if err != nil {
				return nil, fmt.Errorf("response_mappings: invalid extract regex %q: %w", m.Extract, err)
			}
			cm.extract = re
		}
		tier3 = append(tier3, cm)
	}

	var reqMappings []compiledRequestMapping
	for _, m := range pluginCfg.RequestMappings {
		cm := compiledRequestMapping{
			internal:   m.Internal,
			vendorID:   m.VendorID,
			vendorType: m.VendorType,
		}
		if m.RadiusAttr != "" {
			attrType, ok := resolveAttrName(m.RadiusAttr)
			if !ok {
				return nil, fmt.Errorf("request_mappings: unknown radius attribute %q", m.RadiusAttr)
			}
			cm.attrType = attrType
		}
		reqMappings = append(reqMappings, cm)
	}

	p := &Provider{
		cfg:             pluginCfg,
		globalCfg:       cfg,
		logger:          logger.Get(Namespace),
		authConns:       authConns,
		acctConns:       acctConns,
		tier1Index:      tier1,
		tier2Index:      buildTier2Index(),
		tier3:           tier3,
		requestMappings: reqMappings,
		radiusStats:     stats,
	}

	globalProvider.Store(p)

	p.logger.Info("RADIUS auth provider initialized",
		"servers", len(authConns),
		"nas_identifier", pluginCfg.NASIdentifier)

	return p, nil
}

func (p *Provider) Info() provider.Info {
	return provider.Info{
		Name:    "radius",
		Version: "0.1.0",
		Author:  "osvbng Core Team",
	}
}

func (p *Provider) Close() error {
	for _, c := range p.authConns {
		_ = c.close()
	}
	for _, c := range p.acctConns {
		_ = c.close()
	}
	return nil
}

func (p *Provider) Stats() *internalaaa.RADIUSStats {
	return p.radiusStats
}

func (p *Provider) Authenticate(ctx context.Context, req *auth.AuthRequest) (*auth.AuthResponse, error) {
	packet := radius.New(radius.CodeAccessRequest, nil)

	p.addRequestAVPs(packet, req)

	if chapResp, ok := req.Attributes[aaa.AttrCHAPResponse]; ok {
		p.encodeCHAP(packet, req.Attributes, chapResp)
	}

	resp, rc, err := p.sendAuthWithFailover(packet, req.Attributes)
	if err != nil {
		return nil, err
	}

	if resp.Code == radius.CodeAccessReject {
		p.radiusStats.IncrAuthReject(rc.addr)
		p.logger.Info("authentication rejected",
			"username", req.Username,
			"server", rc.addr)
		return &auth.AuthResponse{Allowed: false}, nil
	}

	if resp.Code != radius.CodeAccessAccept {
		p.radiusStats.IncrAuthError(rc.addr, fmt.Errorf("unexpected code %d", resp.Code))
		return nil, fmt.Errorf("unexpected RADIUS response code: %d", resp.Code)
	}

	p.radiusStats.IncrAuthAccept(rc.addr)
	attrs := p.extractAttributes(resp)

	p.logger.Info("authentication accepted",
		"username", req.Username,
		"server", rc.addr,
		"attributes", len(attrs))

	return &auth.AuthResponse{
		Allowed:    true,
		Attributes: attrs,
	}, nil
}

func (p *Provider) sendAuthWithFailover(packet *radius.Packet, attrs map[string]string) (*radius.Packet, *radiusConn, error) {
	var lastErr error

	for _, rc := range p.authConns {
		if rc.isDead(p.cfg.DeadTime) {
			continue
		}

		if pw, ok := attrs[aaa.AttrPassword]; ok {
			packet.Set(2, radius.Attribute(papEncode([]byte(pw), rc.secret, packet.Authenticator[:])))
		}

		for attempt := 0; attempt < p.cfg.Retries; attempt++ {
			p.radiusStats.IncrAuthRequest(rc.addr)

			resp, err := rc.exchange(packet)
			if err != nil {
				lastErr = err
				p.logger.Debug("RADIUS request failed",
					"server", rc.addr,
					"attempt", attempt+1,
					"error", err)
				continue
			}

			rc.recordSuccess()
			return resp, rc, nil
		}

		rc.recordFailure(p.cfg.DeadThreshold)
		p.radiusStats.IncrAuthTimeout(rc.addr)
	}

	if lastErr != nil {
		return nil, nil, fmt.Errorf("all RADIUS servers failed: %w", lastErr)
	}
	return nil, nil, fmt.Errorf("no RADIUS servers available")
}

func (p *Provider) sendAcctWithFailover(packet *radius.Packet) (*radius.Packet, *radiusConn, error) {
	var lastErr error

	for _, rc := range p.acctConns {
		if rc.isDead(p.cfg.DeadTime) {
			continue
		}

		for attempt := 0; attempt < p.cfg.Retries; attempt++ {
			p.radiusStats.IncrAcctRequest(rc.addr)

			resp, err := rc.exchange(packet)
			if err != nil {
				lastErr = err
				p.logger.Debug("RADIUS accounting request failed",
					"server", rc.addr,
					"attempt", attempt+1,
					"error", err)
				continue
			}

			rc.recordSuccess()
			return resp, rc, nil
		}

		rc.recordFailure(p.cfg.DeadThreshold)
		p.radiusStats.IncrAcctTimeout(rc.addr)
	}

	if lastErr != nil {
		return nil, nil, fmt.Errorf("all RADIUS servers failed: %w", lastErr)
	}
	return nil, nil, fmt.Errorf("no RADIUS servers available")
}

func (p *Provider) addRequestAVPs(packet *radius.Packet, req *auth.AuthRequest) {
	packet.Add(1, radius.Attribute(req.Username))

	if p.cfg.NASIdentifier != "" {
		packet.Add(32, radius.Attribute(p.cfg.NASIdentifier))
	}
	if p.cfg.NASIP != "" {
		if ip := net.ParseIP(p.cfg.NASIP); ip != nil {
			packet.Add(4, radius.Attribute(ip.To4()))
		}
	}

	serviceType := serviceTypeForAccess(req.AccessType)
	packet.Add(6, encodeUint32(serviceType))

	packet.Add(61, encodeUint32(nasPortTypeValue(p.cfg.NASPortType)))

	if req.MAC != "" {
		packet.Add(31, radius.Attribute(req.MAC))
	}

	if req.Interface != "" {
		packet.Add(87, radius.Attribute(req.Interface))
	}

	if req.AcctSessionID != "" {
		packet.Add(44, radius.Attribute(req.AcctSessionID))
	}

	if circuitID, ok := req.Attributes[aaa.AttrCircuitID]; ok {
		packet.Add(30, radius.Attribute(circuitID))
	}

	for i := range p.requestMappings {
		v, ok := req.Attributes[p.requestMappings[i].internal]
		if !ok {
			continue
		}
		if p.requestMappings[i].vendorID > 0 {
			packet.Add(26, encodeVSARequest(p.requestMappings[i].vendorID, p.requestMappings[i].vendorType, []byte(v)))
		} else {
			packet.Add(p.requestMappings[i].attrType, radius.Attribute(v))
		}
	}

	now := uint32(time.Now().Unix())
	packet.Add(55, encodeUint32(now))
}

func (p *Provider) encodeCHAP(packet *radius.Packet, attrs map[string]string, chapResp string) {
	respBytes, _ := hex.DecodeString(chapResp)
	if len(respBytes) > 0 {
		chapID := byte(0)
		if idStr, ok := attrs[aaa.AttrCHAPID]; ok {
			idBytes, _ := hex.DecodeString(idStr)
			if len(idBytes) > 0 {
				chapID = idBytes[0]
			}
		}
		chapPassword := append([]byte{chapID}, respBytes...)
		packet.Add(3, radius.Attribute(chapPassword))
	}

	if challenge, ok := attrs[aaa.AttrCHAPChallenge]; ok {
		challengeBytes, _ := hex.DecodeString(challenge)
		if len(challengeBytes) > 0 {
			packet.Add(60, radius.Attribute(challengeBytes))
		}
	}
}

func papEncode(password, secret, authenticator []byte) []byte {
	if len(password) == 0 {
		password = []byte{0}
	}

	padLen := (len(password) + 15) &^ 15
	padded := make([]byte, padLen)
	copy(padded, password)

	result := make([]byte, padLen)
	prev := authenticator

	for i := 0; i < padLen; i += 16 {
		h := md5.New()
		h.Write(secret)
		h.Write(prev)
		hash := h.Sum(nil)

		for j := 0; j < 16; j++ {
			result[i+j] = padded[i+j] ^ hash[j]
		}
		prev = result[i : i+16]
	}

	return result
}

func (p *Provider) extractAttributes(resp *radius.Packet) map[string]string {
	attrs := make(map[string]string)

	for _, avp := range resp.Attributes {
		if m, ok := p.tier1Index[byte(avp.Type)]; ok {
			if val := m.decode(avp.Attribute); val != "" {
				attrs[m.internal] = val
			}
		}

		if avp.Type == 26 {
			p.extractVSA(avp.Attribute, attrs)
		}
	}

	return attrs
}

func (p *Provider) extractVSA(raw radius.Attribute, attrs map[string]string) {
	if len(raw) < 7 {
		return
	}

	vendorID := binary.BigEndian.Uint32(raw[0:4])
	offset := 4

	for offset+2 <= len(raw) {
		vsaType := raw[offset]
		vsaLen := int(raw[offset+1])
		if vsaLen < 2 || offset+vsaLen > len(raw) {
			break
		}
		vsaData := raw[offset+2 : offset+vsaLen]
		key := vendorKey{vendorID: vendorID, vendorType: vsaType}

		if m, ok := p.tier2Index[key]; ok {
			if val := m.decode(vsaData); val != "" {
				attrs[m.internal] = val
			}
		}

		for _, cm := range p.tier3 {
			if cm.vendorID == vendorID && cm.vendorType == vsaType {
				val := string(vsaData)
				if cm.extract != nil {
					matches := cm.extract.FindStringSubmatch(val)
					if len(matches) >= 2 {
						attrs[cm.internal] = matches[1]
					}
				} else {
					attrs[cm.internal] = val
				}
			}
		}

		offset += vsaLen
	}
}

func encodeUint32(v uint32) radius.Attribute {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, v)
	return radius.Attribute(buf)
}

func serviceTypeForAccess(accessType string) uint32 {
	switch accessType {
	case "pppoe":
		return 2
	default:
		return 5
	}
}

func nasPortTypeValue(s string) uint32 {
	switch s {
	case "Async":
		return 0
	case "Sync":
		return 1
	case "ISDN":
		return 2
	case "ISDN-V120":
		return 3
	case "ISDN-V110":
		return 4
	case "Virtual":
		return 5
	case "PIAFS":
		return 6
	case "HDLC":
		return 7
	case "X.25":
		return 8
	case "X.75":
		return 9
	case "G.3Fax":
		return 10
	case "SDSL":
		return 11
	case "ADSL-CAP":
		return 12
	case "ADSL-DMT":
		return 13
	case "IDSL":
		return 14
	case "Ethernet":
		return 15
	case "xDSL":
		return 16
	case "Cable":
		return 17
	case "PPPoA":
		return 30
	case "PPPoEoA":
		return 31
	case "PPPoEoE":
		return 32
	case "PPPoEoVLAN":
		return 33
	case "PPPoEoQinQ":
		return 34
	default:
		return 5
	}
}
