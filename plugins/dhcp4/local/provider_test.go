// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package local

import (
	"net"
	"testing"
	"time"
)

func TestReserveIP_SameAddressInTwoPools(t *testing.T) {
	p := &Provider{
		leases:         map[string]*Lease{},
		leasesByPoolIP: map[string]*Lease{},
	}
	ip := net.ParseIP("100.64.0.2").To4()

	if err := p.reserveIP(ip, "02:00:00:00:00:01", "a1", "default/customer-a-pool", time.Hour); err != nil {
		t.Fatalf("first pool: %v", err)
	}
	if err := p.reserveIP(ip, "02:00:00:00:00:02", "b1", "default/customer-b-pool", time.Hour); err != nil {
		t.Fatalf("the same address in another pool is another subscriber's lease: %v", err)
	}
	if err := p.reserveIP(ip, "02:00:00:00:00:03", "a2", "default/customer-a-pool", time.Hour); err == nil {
		t.Fatalf("a second subscriber in the same pool must not take a live lease")
	}
	if got := len(p.leasesByPoolIP); got != 2 {
		t.Fatalf("expected two leases, got %d", got)
	}
}

func TestReserveIP_RenewalKeepsTheLease(t *testing.T) {
	p := &Provider{
		leases:         map[string]*Lease{},
		leasesByPoolIP: map[string]*Lease{},
	}
	ip := net.ParseIP("100.64.0.2").To4()
	if err := p.reserveIP(ip, "02:00:00:00:00:01", "a1", "default/customer-a-pool", time.Hour); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := p.reserveIP(ip, "02:00:00:00:00:01", "a1", "default/customer-a-pool", time.Hour); err != nil {
		t.Fatalf("renewal by the holder must succeed: %v", err)
	}
	if got := len(p.leasesByPoolIP); got != 1 {
		t.Fatalf("expected one lease, got %d", got)
	}
}
