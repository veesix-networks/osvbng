// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

// Package netbind opens sockets bound to a Linux VRF master device,
// optionally pinned to a source IP. A zero Binding is a no-op.
package netbind

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"google.golang.org/grpc"
)

type Family int

const (
	FamilyV4 Family = 4
	FamilyV6 Family = 6
)

func (f Family) String() string {
	switch f {
	case FamilyV4:
		return "ipv4"
	case FamilyV6:
		return "ipv6"
	default:
		return "unknown"
	}
}

// Binding is the runtime VRF + source-IP pair for one socket.
type Binding struct {
	VRF      string
	SourceIP netip.Addr
}

func (b Binding) IsZero() bool {
	return b.VRF == "" && !b.SourceIP.IsValid()
}

func (b Binding) String() string {
	switch {
	case b.IsZero():
		return "default"
	case b.VRF != "" && b.SourceIP.IsValid():
		return fmt.Sprintf("vrf=%s source=%s", b.VRF, b.SourceIP)
	case b.VRF != "":
		return fmt.Sprintf("vrf=%s", b.VRF)
	default:
		return fmt.Sprintf("source=%s", b.SourceIP)
	}
}

func (b Binding) dialer(timeout time.Duration) *net.Dialer {
	d := &net.Dialer{
		Timeout:  timeout,
		Resolver: Resolver(b),
		Control:  bindControl(b),
	}
	if b.SourceIP.IsValid() {
		d.LocalAddr = sourceTCPAddr(b.SourceIP)
	}
	return d
}

func (b Binding) listenConfig() *net.ListenConfig {
	return &net.ListenConfig{Control: bindControl(b)}
}

func ListenUDP(ctx context.Context, network string, port int, b Binding) (*net.UDPConn, error) {
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("netbind: invalid port %d", port)
	}

	pc, err := b.listenConfig().ListenPacket(ctx, network, bindUDPAddr(b.SourceIP, port))
	if err != nil {
		return nil, fmt.Errorf("netbind: listen %s %s: %w", network, b, err)
	}

	uc, ok := pc.(*net.UDPConn)
	if !ok {
		_ = pc.Close()
		return nil, fmt.Errorf("netbind: ListenPacket returned %T", pc)
	}
	return uc, nil
}

func ListenTCP(ctx context.Context, network, addr string, b Binding) (net.Listener, error) {
	ln, err := b.listenConfig().Listen(ctx, network, resolveTCPListenAddr(addr, b.SourceIP))
	if err != nil {
		return nil, fmt.Errorf("netbind: listen %s %s: %w", network, b, err)
	}
	return ln, nil
}

func DialUDP(ctx context.Context, network string, raddr *net.UDPAddr, b Binding) (*net.UDPConn, error) {
	c, err := b.dialer(0).DialContext(ctx, network, raddr.String())
	if err != nil {
		return nil, fmt.Errorf("netbind: dial %s %s: %w", network, b, err)
	}
	uc, ok := c.(*net.UDPConn)
	if !ok {
		_ = c.Close()
		return nil, fmt.Errorf("netbind: DialContext returned %T", c)
	}
	return uc, nil
}

func HTTPClient(b Binding, timeout time.Duration) *http.Client {
	d := b.dialer(timeout)

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           d.DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}

	clientTimeout := timeout
	if clientTimeout == 0 {
		clientTimeout = 30 * time.Second
	}
	return &http.Client{
		Transport: transport,
		Timeout:   clientTimeout,
	}
}

// GRPCDialOpts returns dialer options for VRF binding only. Caller
// supplies its own grpc.WithTransportCredentials.
func GRPCDialOpts(b Binding) []grpc.DialOption {
	d := b.dialer(0)
	return []grpc.DialOption{
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return d.DialContext(ctx, "tcp", addr)
		}),
	}
}

func bindUDPAddr(src netip.Addr, port int) string {
	host := ""
	if src.IsValid() {
		host = src.String()
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func resolveTCPListenAddr(addr string, src netip.Addr) string {
	if !src.IsValid() {
		return addr
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return net.JoinHostPort(src.String(), port)
}

func sourceTCPAddr(src netip.Addr) net.Addr {
	if !src.IsValid() {
		return nil
	}
	return &net.TCPAddr{IP: src.AsSlice()}
}
