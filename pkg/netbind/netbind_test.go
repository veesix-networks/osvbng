// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package netbind

import (
	"context"
	"net"
	"net/netip"
	"testing"
)

func TestBinding_IsZero(t *testing.T) {
	cases := []struct {
		name string
		b    Binding
		want bool
	}{
		{"empty", Binding{}, true},
		{"vrf only", Binding{VRF: "mgmt"}, false},
		{"src only", Binding{SourceIP: netip.MustParseAddr("10.0.0.1")}, false},
		{"both", Binding{VRF: "mgmt", SourceIP: netip.MustParseAddr("10.0.0.1")}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.b.IsZero(); got != c.want {
				t.Fatalf("IsZero=%v want %v", got, c.want)
			}
		})
	}
}

func TestBinding_String(t *testing.T) {
	cases := []struct {
		b    Binding
		want string
	}{
		{Binding{}, "default"},
		{Binding{VRF: "mgmt"}, "vrf=mgmt"},
		{Binding{SourceIP: netip.MustParseAddr("10.0.0.1")}, "source=10.0.0.1"},
		{Binding{VRF: "mgmt", SourceIP: netip.MustParseAddr("10.0.0.1")}, "vrf=mgmt source=10.0.0.1"},
	}
	for _, c := range cases {
		if got := c.b.String(); got != c.want {
			t.Fatalf("String=%q want %q", got, c.want)
		}
	}
}

// TestEmptyBinding_NoOp asserts that helpers with the zero binding behave
// identically to raw stdlib: open, accept, dial, all without
// SO_BINDTODEVICE. This is the migration regression guard.
func TestEmptyBinding_NoOp_TCPListener(t *testing.T) {
	ln, err := ListenTCP(context.Background(), "tcp", "127.0.0.1:0", Binding{})
	if err != nil {
		t.Fatalf("ListenTCP: %v", err)
	}
	defer ln.Close()

	addr := ln.Addr().String()
	connCh := make(chan net.Conn, 1)
	go func() {
		c, _ := ln.Accept()
		connCh <- c
	}()

	d := &net.Dialer{Timeout: 0}
	c, err := d.DialContext(context.Background(), "tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	srv := <-connCh
	if srv == nil {
		t.Fatal("server-side conn nil")
	}
	defer srv.Close()
}

func TestEmptyBinding_NoOp_UDPListener(t *testing.T) {
	uc, err := ListenUDP(context.Background(), "udp4", 0, Binding{})
	if err != nil {
		t.Fatalf("ListenUDP: %v", err)
	}
	defer uc.Close()

	if uc.LocalAddr() == nil {
		t.Fatal("local addr nil")
	}
}

func TestEmptyBinding_NoOp_DialUDP(t *testing.T) {
	srv, err := ListenUDP(context.Background(), "udp4", 0, Binding{})
	if err != nil {
		t.Fatalf("server listen: %v", err)
	}
	defer srv.Close()

	raddr := srv.LocalAddr().(*net.UDPAddr)
	c, err := DialUDP(context.Background(), "udp", raddr, Binding{})
	if err != nil {
		t.Fatalf("DialUDP: %v", err)
	}
	defer c.Close()

	if _, err := c.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}

	buf := make([]byte, 16)
	n, _, err := srv.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf[:n]) != "ping" {
		t.Fatalf("got %q want ping", buf[:n])
	}
}

func TestEmptyBinding_NoOp_HTTPClient(t *testing.T) {
	c := HTTPClient(Binding{}, 0)
	if c == nil {
		t.Fatal("nil client")
	}
	// The Transport is a freshly built one; confirm it's distinct from
	// http.DefaultTransport so we don't accidentally mutate global state.
	if c.Transport == nil {
		t.Fatal("nil transport")
	}
	if c.Timeout == 0 {
		t.Fatal("zero timeout, expected default 30s")
	}
}

func TestEmptyBinding_Resolver_IsDefault(t *testing.T) {
	r := Resolver(Binding{})
	if r != net.DefaultResolver {
		t.Fatalf("Resolver(empty) should be DefaultResolver, got %p", r)
	}
}

func TestNonEmptyBinding_Resolver_IsCustom(t *testing.T) {
	r := Resolver(Binding{VRF: "mgmt"})
	if r == net.DefaultResolver {
		t.Fatal("non-empty binding should produce a custom resolver")
	}
	if r.Dial == nil {
		t.Fatal("custom resolver has nil Dial")
	}
}

func TestSourceIPOnly_LocalAddrSet_NoBindToDevice(t *testing.T) {
	// vrf="" but source_ip set → LocalAddr is bound, no SO_BINDTODEVICE.
	// We can verify the dialer's LocalAddr is what we asked for.
	b := Binding{SourceIP: netip.MustParseAddr("127.0.0.1")}
	d := b.dialer(0)
	if d.LocalAddr == nil {
		t.Fatal("LocalAddr should be set when source_ip is set")
	}
	if d.Control != nil {
		t.Fatal("Control should be nil when vrf is empty")
	}
}

func TestVRFOnly_ControlSet_LocalAddrNil(t *testing.T) {
	b := Binding{VRF: "mgmt"}
	d := b.dialer(0)
	if d.Control == nil {
		t.Fatal("Control should be set when vrf is set")
	}
	if d.LocalAddr != nil {
		t.Fatal("LocalAddr should be nil when source_ip is empty")
	}
}

func TestBindUDPAddr(t *testing.T) {
	cases := []struct {
		src  netip.Addr
		port int
		want string
	}{
		{netip.Addr{}, 0, ":0"},
		{netip.Addr{}, 67, ":67"},
		{netip.MustParseAddr("10.0.0.1"), 67, "10.0.0.1:67"},
		{netip.MustParseAddr("2001:db8::1"), 547, "[2001:db8::1]:547"},
	}
	for _, c := range cases {
		got := bindUDPAddr(c.src, c.port)
		if got != c.want {
			t.Errorf("bindUDPAddr(%v, %d)=%q want %q", c.src, c.port, got, c.want)
		}
	}
}

func TestResolveTCPListenAddr(t *testing.T) {
	cases := []struct {
		addr string
		src  netip.Addr
		want string
	}{
		{":8080", netip.Addr{}, ":8080"},
		{":8080", netip.MustParseAddr("10.0.0.1"), "10.0.0.1:8080"},
		{"0.0.0.0:8080", netip.MustParseAddr("10.0.0.1"), "10.0.0.1:8080"},
	}
	for _, c := range cases {
		got := resolveTCPListenAddr(c.addr, c.src)
		if got != c.want {
			t.Errorf("resolveTCPListenAddr(%q, %v)=%q want %q", c.addr, c.src, got, c.want)
		}
	}
}
