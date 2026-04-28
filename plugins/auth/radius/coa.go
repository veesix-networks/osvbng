// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package radius

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/veesix-networks/osvbng/internal/subscriber"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/netbind"
	"layeh.com/radius"
)

const (
	CoANamespace = "subscriber.auth.radius.coa"

	coaWorkerCount = 4
	coaQueueSize   = 64

	codeCoARequest        = radius.Code(43)
	codeCoAACK            = radius.Code(44)
	codeCoANAK            = radius.Code(45)
	codeDisconnectRequest = radius.Code(40)
	codeDisconnectACK     = radius.Code(41)
	codeDisconnectNAK     = radius.Code(42)

	errorCauseResidualRemoved  = 201
	errorCauseUnsupportedAttr  = 401
	errorCauseMissingAttr      = 402
	errorCauseNASIDMismatch    = 403
	errorCauseInvalidRequest   = 404
	errorCauseSessionNotFound  = 503
	errorCauseResourcesUnavail = 506
	errorCauseRequestInitiated = 507

	attrTypeUserName          = 1
	attrTypeServiceType       = 6
	attrTypeFramedIPAddress   = 8
	attrTypeNASIdentifier     = 32
	attrTypeProxyState        = 33
	attrTypeAcctSessionID     = 44
	attrTypeEventTimestamp    = 55
	attrTypeMessageAuth       = 80
	attrTypeErrorCause        = 101
	attrTypeFramedIPv6Address = 168

	defaultMutationTimeout = 10 * time.Second
	coaWorkerTimeout       = 5 * time.Second
)

func init() {
	component.Register(CoANamespace, NewCoAComponent)
}

type coaClient struct {
	network *net.IPNet
	secret  []byte
	key     string
}

type coaRequest struct {
	raw    []byte
	packet *radius.Packet
	src    *net.UDPAddr
	client *coaClient
}

type coaMutationWaiter struct {
	ch       chan events.SubscriberMutationResultEvent
	expected int
}

type CoAComponent struct {
	*component.Base

	logger   *logger.Logger
	eventBus events.Bus

	conn    *net.UDPConn
	clients []coaClient
	workCh  chan *coaRequest
	stats   *CoAStats
	wg      sync.WaitGroup

	mutationResultSub events.Subscription
	waiters           sync.Map
}

func NewCoAComponent(deps component.Dependencies) (component.Component, error) {
	provider := GetProvider()
	if provider == nil {
		return nil, nil
	}

	cfg := provider.cfg
	if len(cfg.CoAClients) == 0 {
		return nil, nil
	}

	clients, err := buildCoAClients(cfg.CoAClients)
	if err != nil {
		return nil, fmt.Errorf("build coa clients: %w", err)
	}

	return &CoAComponent{
		Base:     component.NewBase(CoANamespace),
		logger:   logger.Get("radius.coa"),
		eventBus: deps.EventBus,
		clients:  clients,
		workCh:   make(chan *coaRequest, coaQueueSize),
		stats:    NewCoAStats(),
	}, nil
}

func (c *CoAComponent) Start(ctx context.Context) error {
	c.StartContext(ctx)

	provider := GetProvider()
	if provider == nil {
		return fmt.Errorf("radius provider not available")
	}

	bind, err := provider.cfg.CoAListener.Resolve(netbind.FamilyV4)
	if err != nil {
		return fmt.Errorf("coa listener binding: %w", err)
	}
	conn, err := netbind.ListenUDP(ctx, "udp4", provider.cfg.CoAListener.Port, bind)
	if err != nil {
		return fmt.Errorf("listen udp4 :%d: %w", provider.cfg.CoAListener.Port, err)
	}
	c.conn = conn

	c.mutationResultSub = c.eventBus.Subscribe(
		events.TopicSubscriberMutationResult,
		c.handleMutationResult,
	)

	for i := 0; i < coaWorkerCount; i++ {
		c.wg.Add(1)
		go c.worker()
	}

	c.wg.Add(1)
	go c.readLoop()

	c.logger.Info("CoA listener started",
		"port", provider.cfg.CoAListener.Port,
		"binding", bind,
		"clients", len(c.clients))
	return nil
}

func (c *CoAComponent) Stop(ctx context.Context) error {
	c.logger.Info("Stopping CoA listener")
	c.mutationResultSub.Unsubscribe()
	c.StopContext()
	if c.conn != nil {
		c.conn.Close()
	}
	c.wg.Wait()
	return nil
}

func (c *CoAComponent) GetStats() *CoAStats {
	return c.stats
}

func buildCoAClients(cfgs []CoAClientConfig) ([]coaClient, error) {
	clients := make([]coaClient, 0, len(cfgs))
	for _, cfg := range cfgs {
		var ipNet *net.IPNet
		if ip := net.ParseIP(cfg.Host); ip != nil {
			if ip4 := ip.To4(); ip4 != nil {
				ipNet = &net.IPNet{IP: ip4, Mask: net.CIDRMask(32, 32)}
			} else {
				ipNet = &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)}
			}
		} else {
			_, parsed, err := net.ParseCIDR(cfg.Host)
			if err != nil {
				return nil, fmt.Errorf("invalid host %q: %w", cfg.Host, err)
			}
			ipNet = parsed
		}
		clients = append(clients, coaClient{
			network: ipNet,
			secret:  []byte(cfg.Secret),
			key:     cfg.Host,
		})
	}
	return clients, nil
}

func (c *CoAComponent) findClient(src net.IP) *coaClient {
	for i := range c.clients {
		if c.clients[i].network.Contains(src) {
			return &c.clients[i]
		}
	}
	return nil
}

func (c *CoAComponent) readLoop() {
	defer c.wg.Done()
	buf := make([]byte, radius.MaxPacketLength)

	for {
		select {
		case <-c.Ctx.Done():
			return
		default:
		}

		c.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, src, err := c.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			if c.Ctx.Err() != nil {
				return
			}
			c.logger.Debug("CoA read error", "error", err)
			continue
		}

		raw := make([]byte, n)
		copy(raw, buf[:n])

		client := c.findClient(src.IP)
		if client == nil {
			c.stats.IncrUnknownClient()
			continue
		}

		packet, err := radius.Parse(raw, client.secret)
		if err != nil {
			c.stats.IncrInvalidAuth(client.key)
			continue
		}

		if !validateMessageAuthenticator(raw, client.secret) {
			c.stats.IncrInvalidAuth(client.key)
			continue
		}

		req := &coaRequest{
			raw:    raw,
			packet: packet,
			src:    src,
			client: client,
		}

		select {
		case c.workCh <- req:
		default:
			c.stats.IncrOverflow(client.key)
			c.sendResponse(src, client.secret, packet, codeCoANAK, errorCauseResourcesUnavail, raw)
		}
	}
}

func (c *CoAComponent) worker() {
	defer c.wg.Done()
	for {
		select {
		case <-c.Ctx.Done():
			return
		case req := <-c.workCh:
			c.handleRequest(req)
		}
	}
}

func (c *CoAComponent) handleRequest(req *coaRequest) {
	switch req.packet.Code {
	case codeCoARequest:
		c.stats.IncrCoARequest(req.client.key)
		c.handleCoARequest(req)
	case codeDisconnectRequest:
		c.stats.IncrDisconnectRequest(req.client.key)
		c.handleDisconnectRequest(req)
	default:
		c.logger.Debug("Unknown CoA request code", "code", req.packet.Code)
	}
}

func (c *CoAComponent) handleCoARequest(req *coaRequest) {
	provider := GetProvider()
	if provider == nil {
		c.stats.IncrCoANAK(req.client.key)
		c.sendResponse(req.src, req.client.secret, req.packet, codeCoANAK, errorCauseResourcesUnavail, req.raw)
		return
	}

	if hasServiceType(req.packet, 8) {
		c.stats.IncrCoANAK(req.client.key)
		c.sendResponse(req.src, req.client.secret, req.packet, codeCoANAK, errorCauseRequestInitiated, req.raw)
		return
	}

	if provider.cfg.CoAReplayWindow > 0 {
		if ts := getEventTimestamp(req.packet); ts > 0 {
			age := time.Now().Unix() - int64(ts)
			if age > provider.cfg.CoAReplayWindow || age < -provider.cfg.CoAReplayWindow {
				c.stats.IncrInvalidAuth(req.client.key)
				return
			}
		}
	}

	target, errCause := resolveCoATarget(req.packet)
	if errCause != 0 {
		c.stats.IncrCoANAK(req.client.key)
		c.sendResponse(req.src, req.client.secret, req.packet, codeCoANAK, errCause, req.raw)
		return
	}

	if err := validateNASIdentifier(req.packet, provider.cfg.NASIdentifier); err != nil {
		c.stats.IncrCoANAK(req.client.key)
		c.sendResponse(req.src, req.client.secret, req.packet, codeCoANAK, errorCauseNASIDMismatch, req.raw)
		return
	}

	attrs := provider.extractAttributes(req.packet)
	attrs = stripNonMutableAttrs(attrs)

	if len(attrs) == 0 {
		c.stats.IncrCoANAK(req.client.key)
		c.sendResponse(req.src, req.client.secret, req.packet, codeCoANAK, errorCauseMissingAttr, req.raw)
		return
	}

	result, err := c.mutateViaEventBus(target, attrs)
	if err != nil {
		c.stats.IncrCoANAK(req.client.key)
		c.sendResponse(req.src, req.client.secret, req.packet, codeCoANAK, errorCauseResourcesUnavail, req.raw)
		return
	}

	if !result.Ok {
		ec := result.ErrorCause
		if ec == 0 {
			ec = errorCauseResourcesUnavail
		}
		c.stats.IncrCoANAK(req.client.key)
		if ec == errorCauseSessionNotFound {
			c.stats.IncrSessionNotFound(req.client.key)
		}
		c.sendResponse(req.src, req.client.secret, req.packet, codeCoANAK, ec, req.raw)
		return
	}

	c.stats.IncrCoAACK(req.client.key)
	c.sendResponse(req.src, req.client.secret, req.packet, codeCoAACK, 0, req.raw)
}

func (c *CoAComponent) handleDisconnectRequest(req *coaRequest) {
	provider := GetProvider()
	if provider == nil {
		c.stats.IncrDisconnectNAK(req.client.key)
		c.sendResponse(req.src, req.client.secret, req.packet, codeDisconnectNAK, errorCauseResourcesUnavail, req.raw)
		return
	}

	if hasNonIdentificationAttrs(req.packet) {
		c.stats.IncrDisconnectNAK(req.client.key)
		c.sendResponse(req.src, req.client.secret, req.packet, codeDisconnectNAK, errorCauseInvalidRequest, req.raw)
		return
	}

	target, errCause := resolveCoATarget(req.packet)
	if errCause != 0 {
		c.stats.IncrDisconnectNAK(req.client.key)
		c.sendResponse(req.src, req.client.secret, req.packet, codeDisconnectNAK, errCause, req.raw)
		return
	}

	if err := validateNASIdentifier(req.packet, provider.cfg.NASIdentifier); err != nil {
		c.stats.IncrDisconnectNAK(req.client.key)
		c.sendResponse(req.src, req.client.secret, req.packet, codeDisconnectNAK, errorCauseNASIDMismatch, req.raw)
		return
	}

	c.stats.IncrDisconnectACK(req.client.key)
	c.sendResponse(req.src, req.client.secret, req.packet, codeDisconnectACK, errorCauseResidualRemoved, req.raw)

	c.eventBus.Publish(events.TopicSubscriberTerminate, events.Event{
		Source:    CoANamespace,
		Timestamp: time.Now(),
		Data: &events.SubscriberTerminateEvent{
			SessionID:     target.SessionID,
			AcctSessionID: target.AcctSessionID,
			Username:      target.Username,
			FramedIPv4:    target.FramedIPv4,
			FramedIPv6:    target.FramedIPv6,
			Reason:        "radius-disconnect",
		},
	})
}

func (c *CoAComponent) mutateViaEventBus(target subscriber.Target, attrs map[string]string) (*events.SubscriberMutationResultEvent, error) {
	requestID := uuid.NewString()
	waiter := &coaMutationWaiter{
		ch:       make(chan events.SubscriberMutationResultEvent, 1),
		expected: 1,
	}
	c.waiters.Store(requestID, waiter)
	defer c.waiters.Delete(requestID)

	c.eventBus.Publish(events.TopicSubscriberMutation, events.Event{
		Source:    CoANamespace,
		Timestamp: time.Now(),
		Data: &events.SubscriberMutationEvent{
			RequestID:      requestID,
			SessionID:      target.SessionID,
			AcctSessionID:  target.AcctSessionID,
			Username:       target.Username,
			FramedIPv4:     target.FramedIPv4,
			FramedIPv6:     target.FramedIPv6,
			AttributeDelta: attrs,
		},
	})

	timer := time.NewTimer(defaultMutationTimeout)
	defer timer.Stop()

	select {
	case result := <-waiter.ch:
		return &result, nil
	case <-timer.C:
		return &events.SubscriberMutationResultEvent{
			Ok:         false,
			Error:      "mutation timeout",
			ErrorCause: errorCauseResourcesUnavail,
		}, nil
	case <-c.Ctx.Done():
		return nil, fmt.Errorf("context cancelled")
	}
}

func (c *CoAComponent) handleMutationResult(ev events.Event) {
	data, ok := ev.Data.(*events.SubscriberMutationResultEvent)
	if !ok {
		return
	}

	val, ok := c.waiters.Load(data.RequestID)
	if !ok {
		return
	}
	waiter := val.(*coaMutationWaiter)

	select {
	case waiter.ch <- *data:
	default:
	}
}

func resolveCoATarget(packet *radius.Packet) (subscriber.Target, int) {
	var acctSessID, framedIPv4, username, framedIPv6 string

	for _, avp := range packet.Attributes {
		switch avp.Type {
		case attrTypeAcctSessionID:
			if acctSessID == "" {
				acctSessID = string(avp.Attribute)
			}
		case attrTypeFramedIPAddress:
			if framedIPv4 == "" && len(avp.Attribute) == 4 {
				framedIPv4 = net.IP(avp.Attribute).String()
			}
		case attrTypeUserName:
			if username == "" {
				username = string(avp.Attribute)
			}
		case attrTypeFramedIPv6Address:
			if framedIPv6 == "" && len(avp.Attribute) == 16 {
				framedIPv6 = net.IP(avp.Attribute).String()
			}
		}
	}

	switch {
	case acctSessID != "":
		return subscriber.Target{AcctSessionID: acctSessID}, 0
	case framedIPv4 != "":
		return subscriber.Target{FramedIPv4: framedIPv4}, 0
	case username != "":
		return subscriber.Target{Username: username}, 0
	case framedIPv6 != "":
		return subscriber.Target{FramedIPv6: framedIPv6}, 0
	default:
		return subscriber.Target{}, errorCauseMissingAttr
	}
}

func validateNASIdentifier(packet *radius.Packet, expected string) error {
	for _, avp := range packet.Attributes {
		if avp.Type == attrTypeNASIdentifier {
			if expected != "" && string(avp.Attribute) != expected {
				return fmt.Errorf("NAS-Identifier mismatch")
			}
			return nil
		}
	}
	return nil
}

func stripNonMutableAttrs(attrs map[string]string) map[string]string {
	delete(attrs, "ipv4_address")
	delete(attrs, "ipv4_netmask")
	delete(attrs, "ipv6_address")
	delete(attrs, "ipv6_prefix")
	delete(attrs, "ipv6_wan_prefix")
	delete(attrs, "pool")
	delete(attrs, "iana_pool")
	delete(attrs, "pd_pool")
	delete(attrs, "vrf")
	delete(attrs, "unnumbered")
	delete(attrs, "urpf")
	delete(attrs, "routed_prefix")
	delete(attrs, "username")
	delete(attrs, "password")
	delete(attrs, "dns_primary")
	delete(attrs, "dns_secondary")
	return attrs
}

func hasServiceType(packet *radius.Packet, value uint32) bool {
	for _, avp := range packet.Attributes {
		if avp.Type == attrTypeServiceType && len(avp.Attribute) == 4 {
			if binary.BigEndian.Uint32(avp.Attribute) == value {
				return true
			}
		}
	}
	return false
}

func getEventTimestamp(packet *radius.Packet) uint32 {
	for _, avp := range packet.Attributes {
		if avp.Type == attrTypeEventTimestamp && len(avp.Attribute) == 4 {
			return binary.BigEndian.Uint32(avp.Attribute)
		}
	}
	return 0
}

var identificationAttrTypes = map[radius.Type]bool{
	attrTypeUserName:          true,
	attrTypeFramedIPAddress:   true,
	attrTypeAcctSessionID:     true,
	attrTypeFramedIPv6Address: true,
	attrTypeNASIdentifier:     true,
	4:                         true,
	5:                         true,
	31:                        true,
	61:                        true,
	87:                        true,
	attrTypeEventTimestamp:    true,
	attrTypeMessageAuth:       true,
	attrTypeProxyState:        true,
	attrTypeErrorCause:        true,
}

func hasNonIdentificationAttrs(packet *radius.Packet) bool {
	for _, avp := range packet.Attributes {
		if !identificationAttrTypes[avp.Type] {
			return true
		}
	}
	return false
}

func validateMessageAuthenticator(raw []byte, secret []byte) bool {
	offset := findAttr80(raw)
	if offset < 0 {
		return true
	}

	saved := make([]byte, 16)
	copy(saved, raw[offset:offset+16])

	for i := 0; i < 16; i++ {
		raw[offset+i] = 0
	}

	h := hmac.New(md5.New, secret)
	h.Write(raw)
	computed := h.Sum(nil)

	copy(raw[offset:offset+16], saved)

	return hmac.Equal(saved, computed)
}

func (c *CoAComponent) sendResponse(dst *net.UDPAddr, secret []byte, request *radius.Packet, code radius.Code, errorCause int, requestRaw []byte) {
	resp := radius.New(code, secret)
	resp.Identifier = request.Identifier

	for _, avp := range request.Attributes {
		if avp.Type == attrTypeProxyState {
			resp.Add(attrTypeProxyState, avp.Attribute)
		}
	}

	if errorCause > 0 {
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, uint32(errorCause))
		resp.Add(attrTypeErrorCause, radius.Attribute(buf))
	}

	requestHasMA := findAttr80(requestRaw) >= 0
	if requestHasMA {
		resp.Add(attrTypeMessageAuth, make(radius.Attribute, 16))
	}

	encoded, err := resp.Encode()
	if err != nil {
		c.logger.Warn("Failed to encode CoA response", "error", err)
		return
	}

	// Response Authenticator: MD5(Code+ID+Length+RequestAuth+Attributes+Secret)
	copy(encoded[4:20], requestRaw[4:20])
	rh := md5.New()
	rh.Write(encoded)
	rh.Write(secret)
	copy(encoded[4:20], rh.Sum(nil))

	// Message-Authenticator: HMAC-MD5(packet with ResponseAuth set and MA zeroed, Secret)
	if requestHasMA {
		if maOffset := findAttr80(encoded); maOffset >= 0 {
			for i := 0; i < 16; i++ {
				encoded[maOffset+i] = 0
			}
			mac := hmac.New(md5.New, secret)
			mac.Write(encoded)
			copy(encoded[maOffset:maOffset+16], mac.Sum(nil))
		}
	}

	c.conn.WriteToUDP(encoded, dst)
}
