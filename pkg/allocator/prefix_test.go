package allocator

import (
	"errors"
	"net"
	"net/netip"
	"testing"
)

func newTestPrefix(network string, prefixLen int) *PrefixAllocator {
	return NewPrefixAllocator(netip.MustParsePrefix(network), prefixLen)
}

func TestPrefixAllocatorNew(t *testing.T) {
	a := newTestPrefix("fd00::/48", 56)
	if a == nil {
		t.Fatal("expected non-nil allocator")
	}
}

func TestPrefixAllocatorNewNegativeDelegBits(t *testing.T) {
	a := NewPrefixAllocator(netip.MustParsePrefix("fd00::/56"), 48)
	if a != nil {
		t.Fatal("expected nil for prefixLength < network bits")
	}
}

func TestPrefixAllocatorNewTooManyDelegBits(t *testing.T) {
	a := NewPrefixAllocator(netip.MustParsePrefix("fd00::/64"), 128)
	if a != nil {
		t.Fatal("expected nil for delegBits > 63")
	}
}

func TestPrefixAllocatorNewSameLength(t *testing.T) {
	a := newTestPrefix("fd00:abcd::/48", 48)
	if a == nil {
		t.Fatal("expected non-nil allocator for same length")
	}
	pfx, err := a.Allocate("s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ones, _ := pfx.Mask.Size()
	if ones != 48 {
		t.Fatalf("prefix length = %d, want 48", ones)
	}
	_, err = a.Allocate("s2")
	if !errors.Is(err, ErrPoolExhausted) {
		t.Fatalf("got %v, want ErrPoolExhausted", err)
	}
}

func TestPrefixAllocateFirst(t *testing.T) {
	a := newTestPrefix("fd00::/48", 56)
	pfx, err := a.Allocate("s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "fd00::/56"
	if pfx.String() != want {
		t.Fatalf("got %v, want %s", pfx, want)
	}
}

func TestPrefixAllocateSequential(t *testing.T) {
	a := newTestPrefix("fd00::/48", 56)
	pfx1, _ := a.Allocate("s1")
	pfx2, _ := a.Allocate("s2")
	if pfx1.String() != "fd00::/56" {
		t.Fatalf("first got %v, want fd00::/56", pfx1)
	}
	if pfx2.String() != "fd00:0:0:100::/56" {
		t.Fatalf("second got %v, want fd00:0:0:100::/56", pfx2)
	}
}

func TestPrefixAllocateExhausted(t *testing.T) {
	a := newTestPrefix("fd00::/126", 127)
	a.Allocate("s1")
	a.Allocate("s2")
	_, err := a.Allocate("s3")
	if !errors.Is(err, ErrPoolExhausted) {
		t.Fatalf("got %v, want ErrPoolExhausted", err)
	}
}

func TestPrefixAllocateLargePool(t *testing.T) {
	a := newTestPrefix("fd00::/48", 56)
	seen := make(map[string]bool)
	for i := 0; i < 256; i++ {
		pfx, err := a.Allocate("s")
		if err != nil {
			t.Fatalf("allocation %d failed: %v", i, err)
		}
		s := pfx.String()
		if seen[s] {
			t.Fatalf("duplicate prefix at allocation %d: %s", i, s)
		}
		seen[s] = true
		ones, _ := pfx.Mask.Size()
		if ones != 56 {
			t.Fatalf("prefix length = %d, want 56", ones)
		}
	}
	_, err := a.Allocate("s")
	if !errors.Is(err, ErrPoolExhausted) {
		t.Fatalf("got %v, want ErrPoolExhausted after 256 allocations", err)
	}
}

func TestPrefixReleaseAndReallocate(t *testing.T) {
	a := newTestPrefix("fd00::/126", 127)
	pfx1, _ := a.Allocate("s1")
	a.Allocate("s2")
	a.Release(pfx1)
	pfx3, err := a.Allocate("s3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pfx3.String() != pfx1.String() {
		t.Fatalf("got %v, want %v", pfx3, pfx1)
	}
}

func TestPrefixReleaseNil(t *testing.T) {
	a := newTestPrefix("fd00::/48", 56)
	a.Release(nil)
}

func TestPrefixReleaseWrongLength(t *testing.T) {
	a := newTestPrefix("fd00::/48", 56)
	a.Allocate("s1")
	wrong := &net.IPNet{
		IP:   net.ParseIP("fd00::"),
		Mask: net.CIDRMask(64, 128),
	}
	a.Release(wrong)
	pfx, _ := a.Allocate("s2")
	if pfx.String() != "fd00:0:0:100::/56" {
		t.Fatalf("expected second prefix, got %v (wrong-length release should be no-op)", pfx)
	}
}

func TestPrefixReserveSameSession(t *testing.T) {
	a := newTestPrefix("fd00::/48", 56)
	pfx, _ := a.Allocate("s1")
	err := a.Reserve(pfx, "s1")
	if err != nil {
		t.Fatalf("re-reserve same session should be nil, got %v", err)
	}
}

func TestPrefixReserveDifferentSession(t *testing.T) {
	a := newTestPrefix("fd00::/48", 56)
	pfx, _ := a.Allocate("s1")
	err := a.Reserve(pfx, "s2")
	if !errors.Is(err, ErrAlreadyReserved) {
		t.Fatalf("got %v, want ErrAlreadyReserved", err)
	}
}

func TestPrefixContains(t *testing.T) {
	a := newTestPrefix("fd00::/48", 56)

	valid := &net.IPNet{
		IP:   net.ParseIP("fd00::"),
		Mask: net.CIDRMask(56, 128),
	}
	if !a.Contains(valid) {
		t.Fatal("expected Contains to return true for valid prefix")
	}

	outside := &net.IPNet{
		IP:   net.ParseIP("fd01::"),
		Mask: net.CIDRMask(56, 128),
	}
	if a.Contains(outside) {
		t.Fatal("expected Contains to return false for outside prefix")
	}

	if a.Contains(nil) {
		t.Fatal("expected Contains to return false for nil")
	}
}
