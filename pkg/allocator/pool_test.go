package allocator

import (
	"errors"
	"net"
	"net/netip"
	"testing"
)

func newTestPool(start, end string, exclude ...string) *PoolAllocator {
	var excl []netip.Addr
	for _, e := range exclude {
		excl = append(excl, netip.MustParseAddr(e))
	}
	return NewPoolAllocator(netip.MustParseAddr(start), netip.MustParseAddr(end), excl)
}

func TestPoolAllocateFirst(t *testing.T) {
	p := newTestPool("10.0.0.1", "10.0.0.10")
	ip, err := p.Allocate("s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ip.Equal(net.ParseIP("10.0.0.1")) {
		t.Fatalf("got %v, want 10.0.0.1", ip)
	}
}

func TestPoolAllocateSequential(t *testing.T) {
	p := newTestPool("10.0.0.1", "10.0.0.10")
	ip1, _ := p.Allocate("s1")
	ip2, _ := p.Allocate("s2")
	if !ip1.Equal(net.ParseIP("10.0.0.1")) {
		t.Fatalf("first got %v, want 10.0.0.1", ip1)
	}
	if !ip2.Equal(net.ParseIP("10.0.0.2")) {
		t.Fatalf("second got %v, want 10.0.0.2", ip2)
	}
}

func TestPoolAllocateExhausted(t *testing.T) {
	p := newTestPool("10.0.0.1", "10.0.0.2")
	p.Allocate("s1")
	p.Allocate("s2")
	_, err := p.Allocate("s3")
	if !errors.Is(err, ErrPoolExhausted) {
		t.Fatalf("got %v, want ErrPoolExhausted", err)
	}
}

func TestPoolAllocateSingleAddress(t *testing.T) {
	p := newTestPool("10.0.0.5", "10.0.0.5")
	ip, err := p.Allocate("s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ip.Equal(net.ParseIP("10.0.0.5")) {
		t.Fatalf("got %v, want 10.0.0.5", ip)
	}
	_, err = p.Allocate("s2")
	if !errors.Is(err, ErrPoolExhausted) {
		t.Fatalf("got %v, want ErrPoolExhausted", err)
	}
}

func TestPoolReleaseAndReallocate(t *testing.T) {
	p := newTestPool("10.0.0.1", "10.0.0.2")
	ip1, _ := p.Allocate("s1")
	p.Allocate("s2")
	p.Release(ip1)
	ip3, err := p.Allocate("s3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ip3.Equal(ip1) {
		t.Fatalf("got %v, want %v", ip3, ip1)
	}
}

func TestPoolReleaseUnknownAddress(t *testing.T) {
	p := newTestPool("10.0.0.1", "10.0.0.5")
	err := p.Release(net.ParseIP("10.0.1.1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Available() != 5 {
		t.Fatalf("available = %d, want 5", p.Available())
	}
}

func TestPoolReleaseNilIP(t *testing.T) {
	p := newTestPool("10.0.0.1", "10.0.0.5")
	err := p.Release(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPoolReserveFreshAddress(t *testing.T) {
	p := newTestPool("10.0.0.1", "10.0.0.5")
	before := p.Available()
	err := p.Reserve(net.ParseIP("10.0.0.3"), "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	after := p.Available()
	if after != before-1 {
		t.Fatalf("available = %d, want %d", after, before-1)
	}
}

func TestPoolReserveSameSession(t *testing.T) {
	p := newTestPool("10.0.0.1", "10.0.0.5")
	p.Reserve(net.ParseIP("10.0.0.3"), "s1")
	err := p.Reserve(net.ParseIP("10.0.0.3"), "s1")
	if err != nil {
		t.Fatalf("re-reserve same session should be nil, got %v", err)
	}
}

func TestPoolReserveDifferentSession(t *testing.T) {
	p := newTestPool("10.0.0.1", "10.0.0.5")
	p.Reserve(net.ParseIP("10.0.0.3"), "s1")
	err := p.Reserve(net.ParseIP("10.0.0.3"), "s2")
	if !errors.Is(err, ErrAlreadyReserved) {
		t.Fatalf("got %v, want ErrAlreadyReserved", err)
	}
}

func TestPoolReserveOutOfRange(t *testing.T) {
	p := newTestPool("10.0.0.1", "10.0.0.5")
	err := p.Reserve(net.ParseIP("10.0.1.1"), "s1")
	if err != nil {
		t.Fatalf("out-of-range reserve should be no-op, got %v", err)
	}
	if p.Available() != 5 {
		t.Fatalf("available = %d, want 5", p.Available())
	}
}

func TestPoolContainsInRange(t *testing.T) {
	p := newTestPool("10.0.0.1", "10.0.0.10")
	if !p.Contains(net.ParseIP("10.0.0.5")) {
		t.Fatal("expected Contains to return true for in-range address")
	}
}

func TestPoolContainsBoundaries(t *testing.T) {
	p := newTestPool("10.0.0.1", "10.0.0.10")
	if !p.Contains(net.ParseIP("10.0.0.1")) {
		t.Fatal("expected Contains to return true for rangeStart")
	}
	if !p.Contains(net.ParseIP("10.0.0.10")) {
		t.Fatal("expected Contains to return true for rangeEnd")
	}
}

func TestPoolContainsOutOfRange(t *testing.T) {
	p := newTestPool("10.0.0.1", "10.0.0.10")
	if p.Contains(net.ParseIP("10.0.0.0")) {
		t.Fatal("expected Contains to return false for address below range")
	}
	if p.Contains(net.ParseIP("10.0.0.11")) {
		t.Fatal("expected Contains to return false for address above range")
	}
}

func TestPoolContainsNilIP(t *testing.T) {
	p := newTestPool("10.0.0.1", "10.0.0.10")
	if p.Contains(nil) {
		t.Fatal("expected Contains to return false for nil IP")
	}
}

func TestPoolExcludeAddresses(t *testing.T) {
	p := newTestPool("10.0.0.1", "10.0.0.5", "10.0.0.2", "10.0.0.4")
	if p.Available() != 3 {
		t.Fatalf("available = %d, want 3", p.Available())
	}
	ip1, _ := p.Allocate("s1")
	ip2, _ := p.Allocate("s2")
	ip3, _ := p.Allocate("s3")
	if !ip1.Equal(net.ParseIP("10.0.0.1")) {
		t.Fatalf("first got %v, want 10.0.0.1", ip1)
	}
	if !ip2.Equal(net.ParseIP("10.0.0.3")) {
		t.Fatalf("second got %v, want 10.0.0.3", ip2)
	}
	if !ip3.Equal(net.ParseIP("10.0.0.5")) {
		t.Fatalf("third got %v, want 10.0.0.5", ip3)
	}
	_, err := p.Allocate("s4")
	if !errors.Is(err, ErrPoolExhausted) {
		t.Fatalf("got %v, want ErrPoolExhausted", err)
	}
}
