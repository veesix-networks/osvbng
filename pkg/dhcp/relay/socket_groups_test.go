// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package relay

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/netbind"
)

func newTestClient(t *testing.T, opener socketOpener) *Client {
	t.Helper()
	c := newClient()
	c.opener = opener
	return c
}

func loopbackOpener(t *testing.T) socketOpener {
	t.Helper()
	return func(ctx context.Context, network string, port int, b netbind.Binding) (*net.UDPConn, error) {
		return net.ListenUDP(network, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	}
}

func bk(family Family, vrf, src string, port int) bindingKey {
	return bindingKey{Family: family, VRF: vrf, SourceIP: src, LocalPort: port}
}

func TestSocketGroups_SameAddrDifferentVRFs(t *testing.T) {
	c := newTestClient(t, loopbackOpener(t))
	defer c.Close()

	addr := &net.UDPAddr{IP: net.IPv4(10, 50, 0, 1), Port: 67}

	s1 := c.getOrCreateServer(FamilyV4, addr, 0, netbind.Binding{VRF: "MGMT-VRF"})
	s2 := c.getOrCreateServer(FamilyV4, addr, 0, netbind.Binding{VRF: "OOB-VRF"})
	if s1 == s2 {
		t.Fatal("same addr in different VRFs must not collapse")
	}

	g1, err := c.groupFor(s1.bindingKey(), s1.Binding)
	if err != nil {
		t.Fatalf("groupFor MGMT: %v", err)
	}
	g2, err := c.groupFor(s2.bindingKey(), s2.Binding)
	if err != nil {
		t.Fatalf("groupFor OOB: %v", err)
	}
	if g1 == g2 {
		t.Fatal("expected two distinct groups")
	}
	if c.SocketGroupCount() != 2 {
		t.Errorf("SocketGroupCount=%d want 2", c.SocketGroupCount())
	}
}

func TestSocketGroups_EmptyBinding_SingleSocket(t *testing.T) {
	c := newTestClient(t, loopbackOpener(t))
	defer c.Close()

	addrA := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 67}
	addrB := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 2), Port: 67}

	sA := c.getOrCreateServer(FamilyV4, addrA, 0, netbind.Binding{})
	sB := c.getOrCreateServer(FamilyV4, addrB, 0, netbind.Binding{})

	gA, _ := c.groupFor(sA.bindingKey(), sA.Binding)
	gB, _ := c.groupFor(sB.bindingKey(), sB.Binding)
	if gA != gB {
		t.Fatal("empty-binding servers must share one group")
	}
	if c.SocketGroupCount() != 1 {
		t.Errorf("SocketGroupCount=%d want 1", c.SocketGroupCount())
	}
}

func TestSocketGroups_DifferentSourceIPs(t *testing.T) {
	c := newTestClient(t, loopbackOpener(t))
	defer c.Close()

	addr := &net.UDPAddr{IP: net.IPv4(10, 0, 0, 1), Port: 67}

	bA := netbind.Binding{VRF: "MGMT-VRF", SourceIP: netip.MustParseAddr("10.99.0.2")}
	bB := netbind.Binding{VRF: "MGMT-VRF", SourceIP: netip.MustParseAddr("10.99.0.99")}

	sA := c.getOrCreateServer(FamilyV4, addr, 0, bA)
	sB := c.getOrCreateServer(FamilyV4, addr, 0, bB)
	if sA == sB {
		t.Fatal("different source IPs must produce distinct Server entries")
	}
	gA, _ := c.groupFor(sA.bindingKey(), sA.Binding)
	gB, _ := c.groupFor(sB.bindingKey(), sB.Binding)
	if gA == gB {
		t.Fatal("different source IPs must produce distinct groups")
	}
}

func TestIsFatalSocketErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"closed", net.ErrClosed, true},
		{"ENODEV", syscall.ENODEV, true},
		{"ENETDOWN", syscall.ENETDOWN, true},
		{"EADDRNOTAVAIL", syscall.EADDRNOTAVAIL, true},
		{"ENETUNREACH", syscall.ENETUNREACH, true},
		{"EAGAIN", syscall.EAGAIN, false},
		{"timeout", &timeoutErr{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isFatalSocketErr(c.err); got != c.want {
				t.Errorf("isFatalSocketErr(%v)=%v want %v", c.err, got, c.want)
			}
		})
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestSocketGroups_ReopenAfterClose(t *testing.T) {
	c := newTestClient(t, loopbackOpener(t))
	defer c.Close()

	key := bk(FamilyV4, "MGMT-VRF", "", 67)
	bind := netbind.Binding{VRF: "MGMT-VRF"}

	g1, err := c.groupFor(key, bind)
	if err != nil {
		t.Fatalf("first groupFor: %v", err)
	}
	g1.markClosed("test", errors.New("simulated VRF removal"))

	if c.SocketGroupCount() != 0 {
		t.Errorf("SocketGroupCount after markClosed=%d want 0", c.SocketGroupCount())
	}

	g2, err := c.groupFor(key, bind)
	if err != nil {
		t.Fatalf("second groupFor (reopen): %v", err)
	}
	if g2 == g1 {
		t.Fatal("reopen must allocate a fresh group")
	}
}

// Counts opener calls because a bounded-backoff regression would show
// up as a reopen storm: the opener counter would explode instead of
// staying pinned at 1.
func TestSocketGroups_NoBusyLoopOnTransientErr(t *testing.T) {
	var openerCalls atomic.Int32
	opener := func(ctx context.Context, network string, port int, b netbind.Binding) (*net.UDPConn, error) {
		openerCalls.Add(1)
		return net.ListenUDP(network, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	}

	c := newTestClient(t, opener)
	defer c.Close()

	key := bk(FamilyV4, "", "", 67)
	if _, err := c.groupFor(key, netbind.Binding{}); err != nil {
		t.Fatalf("groupFor: %v", err)
	}

	time.Sleep(2 * readBackoffOnUnknownErr)
	if calls := openerCalls.Load(); calls != 1 {
		t.Errorf("openerCalls=%d want 1", calls)
	}
}

func TestGetClient_DefaultOpener(t *testing.T) {
	logger.Get(logger.IPoERelay)
	c := GetClient()
	if c == nil {
		t.Fatal("GetClient returned nil")
	}
	if c.opener == nil {
		t.Error("default client has no opener wired")
	}
}
