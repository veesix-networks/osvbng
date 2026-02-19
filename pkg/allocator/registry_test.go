package allocator

import (
	"errors"
	"net"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/config/ip"
)

func makeV4Profile(gateway string, pools ...ip.IPv4Pool) *ip.IPv4Profile {
	return &ip.IPv4Profile{
		Gateway: gateway,
		Pools:   pools,
	}
}

func makeV6Profile(iana []ip.IANAPool, pd []ip.PDPool) *ip.IPv6Profile {
	return &ip.IPv6Profile{
		IANAPools: iana,
		PDPools:   pd,
	}
}

func TestNewRegistryNilProfiles(t *testing.T) {
	r := newRegistry(nil, nil)
	if r == nil {
		t.Fatal("expected non-nil registry")
	}
	if len(r.allocators) != 0 || len(r.ianaAllocators) != 0 || len(r.pdAllocators) != 0 {
		t.Fatal("expected empty allocator maps")
	}
}

func TestInitGlobalRegistryAndGet(t *testing.T) {
	r := InitGlobalRegistry(nil, nil)
	g := GetGlobalRegistry()
	if r != g {
		t.Fatal("expected same pointer from Init and Get")
	}
}

func TestRegistryAllocateFromProfile(t *testing.T) {
	profiles := map[string]*ip.IPv4Profile{
		"prof1": makeV4Profile("10.0.0.1", ip.IPv4Pool{
			Name:       "pool1",
			Network:    "10.0.0.0/24",
			RangeStart: "10.0.0.2",
			RangeEnd:   "10.0.0.10",
		}),
	}
	r := newRegistry(profiles, nil)
	allocated, key, err := r.AllocateFromProfile("prof1", "", "", "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allocated.Equal(net.ParseIP("10.0.0.2")) {
		t.Fatalf("got %v, want 10.0.0.2", allocated)
	}
	if key != "prof1/pool1" {
		t.Fatalf("key = %q, want %q", key, "prof1/pool1")
	}
}

func TestRegistryAllocateCompositeKeyIsolation(t *testing.T) {
	profiles := map[string]*ip.IPv4Profile{
		"profA": makeV4Profile("10.0.0.1", ip.IPv4Pool{
			Name:       "shared",
			Network:    "10.0.0.0/24",
			RangeStart: "10.0.0.2",
			RangeEnd:   "10.0.0.2",
		}),
		"profB": makeV4Profile("10.0.1.1", ip.IPv4Pool{
			Name:       "shared",
			Network:    "10.0.1.0/24",
			RangeStart: "10.0.1.2",
			RangeEnd:   "10.0.1.2",
		}),
	}
	r := newRegistry(profiles, nil)

	ipA, keyA, err := r.AllocateFromProfile("profA", "", "", "s1")
	if err != nil {
		t.Fatalf("profA allocation failed: %v", err)
	}
	ipB, keyB, err := r.AllocateFromProfile("profB", "", "", "s2")
	if err != nil {
		t.Fatalf("profB allocation failed: %v", err)
	}

	if keyA == keyB {
		t.Fatalf("composite keys should differ: %q vs %q", keyA, keyB)
	}
	if ipA.Equal(ipB) {
		t.Fatal("IPs from different profiles should differ")
	}
}

func TestRegistryAllocatePoolPriority(t *testing.T) {
	profiles := map[string]*ip.IPv4Profile{
		"prof1": makeV4Profile("", ip.IPv4Pool{
			Name:       "high",
			Network:    "10.0.1.0/24",
			RangeStart: "10.0.1.1",
			RangeEnd:   "10.0.1.1",
			Priority:   10,
		}, ip.IPv4Pool{
			Name:       "low",
			Network:    "10.0.0.0/24",
			RangeStart: "10.0.0.1",
			RangeEnd:   "10.0.0.1",
			Priority:   1,
		}),
	}
	r := newRegistry(profiles, nil)

	ip1, key1, _ := r.AllocateFromProfile("prof1", "", "", "s1")
	if key1 != "prof1/low" {
		t.Fatalf("first allocation key = %q, want prof1/low (lower priority first)", key1)
	}
	if !ip1.Equal(net.ParseIP("10.0.0.1")) {
		t.Fatalf("got %v, want 10.0.0.1", ip1)
	}

	ip2, key2, _ := r.AllocateFromProfile("prof1", "", "", "s2")
	if key2 != "prof1/high" {
		t.Fatalf("spill key = %q, want prof1/high", key2)
	}
	if !ip2.Equal(net.ParseIP("10.0.1.1")) {
		t.Fatalf("got %v, want 10.0.1.1", ip2)
	}
}

func TestRegistryAllocatePoolOverride(t *testing.T) {
	profiles := map[string]*ip.IPv4Profile{
		"prof1": makeV4Profile("", ip.IPv4Pool{
			Name:       "default",
			Network:    "10.0.0.0/24",
			RangeStart: "10.0.0.1",
			RangeEnd:   "10.0.0.1",
			Priority:   1,
		}, ip.IPv4Pool{
			Name:       "special",
			Network:    "10.0.1.0/24",
			RangeStart: "10.0.1.1",
			RangeEnd:   "10.0.1.1",
			Priority:   10,
		}),
	}
	r := newRegistry(profiles, nil)

	allocated, key, err := r.AllocateFromProfile("prof1", "special", "", "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "prof1/special" {
		t.Fatalf("key = %q, want prof1/special", key)
	}
	if !allocated.Equal(net.ParseIP("10.0.1.1")) {
		t.Fatalf("got %v, want 10.0.1.1", allocated)
	}
}

func TestRegistryAllocatePoolOverrideNotFound(t *testing.T) {
	profiles := map[string]*ip.IPv4Profile{
		"prof1": makeV4Profile("", ip.IPv4Pool{
			Name:       "default",
			Network:    "10.0.0.0/24",
			RangeStart: "10.0.0.1",
			RangeEnd:   "10.0.0.1",
			Priority:   1,
		}),
	}
	r := newRegistry(profiles, nil)

	allocated, key, err := r.AllocateFromProfile("prof1", "nonexistent", "", "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "prof1/default" {
		t.Fatalf("key = %q, want prof1/default (fallback)", key)
	}
	if !allocated.Equal(net.ParseIP("10.0.0.1")) {
		t.Fatalf("got %v, want 10.0.0.1", allocated)
	}
}

func TestRegistryAllocateProfileNotFound(t *testing.T) {
	r := newRegistry(nil, nil)
	_, _, err := r.AllocateFromProfile("nosuch", "", "", "s1")
	if !errors.Is(err, ErrPoolExhausted) {
		t.Fatalf("got %v, want ErrPoolExhausted", err)
	}
}

func TestRegistryAllocateExhausted(t *testing.T) {
	profiles := map[string]*ip.IPv4Profile{
		"prof1": makeV4Profile("", ip.IPv4Pool{
			Name:       "tiny",
			Network:    "10.0.0.0/30",
			RangeStart: "10.0.0.1",
			RangeEnd:   "10.0.0.1",
		}),
	}
	r := newRegistry(profiles, nil)
	r.AllocateFromProfile("prof1", "", "", "s1")
	_, _, err := r.AllocateFromProfile("prof1", "", "", "s2")
	if !errors.Is(err, ErrPoolExhausted) {
		t.Fatalf("got %v, want ErrPoolExhausted", err)
	}
}

func TestRegistryNilReceiver(t *testing.T) {
	var r *Registry
	_, _, err := r.AllocateFromProfile("prof1", "", "", "s1")
	if !errors.Is(err, ErrPoolExhausted) {
		t.Fatalf("got %v, want ErrPoolExhausted", err)
	}
}

func TestRegistryRelease(t *testing.T) {
	profiles := map[string]*ip.IPv4Profile{
		"prof1": makeV4Profile("", ip.IPv4Pool{
			Name:       "pool1",
			Network:    "10.0.0.0/24",
			RangeStart: "10.0.0.1",
			RangeEnd:   "10.0.0.1",
		}),
	}
	r := newRegistry(profiles, nil)
	ip1, key, _ := r.AllocateFromProfile("prof1", "", "", "s1")
	r.Release(key, ip1)
	ip2, _, err := r.AllocateFromProfile("prof1", "", "", "s2")
	if err != nil {
		t.Fatalf("unexpected error after release: %v", err)
	}
	if !ip2.Equal(ip1) {
		t.Fatalf("got %v, want %v after release", ip2, ip1)
	}
}

func TestRegistryReleaseIP(t *testing.T) {
	profiles := map[string]*ip.IPv4Profile{
		"prof1": makeV4Profile("", ip.IPv4Pool{
			Name:       "pool1",
			Network:    "10.0.0.0/24",
			RangeStart: "10.0.0.1",
			RangeEnd:   "10.0.0.1",
		}),
	}
	r := newRegistry(profiles, nil)
	ip1, _, _ := r.AllocateFromProfile("prof1", "", "", "s1")
	r.ReleaseIP(ip1)
	ip2, _, err := r.AllocateFromProfile("prof1", "", "", "s2")
	if err != nil {
		t.Fatalf("unexpected error after ReleaseIP: %v", err)
	}
	if !ip2.Equal(ip1) {
		t.Fatalf("got %v, want %v after ReleaseIP", ip2, ip1)
	}
}

func TestRegistryReserveIP(t *testing.T) {
	profiles := map[string]*ip.IPv4Profile{
		"prof1": makeV4Profile("", ip.IPv4Pool{
			Name:       "pool1",
			Network:    "10.0.0.0/24",
			RangeStart: "10.0.0.1",
			RangeEnd:   "10.0.0.2",
		}),
	}
	r := newRegistry(profiles, nil)
	r.ReserveIP(net.ParseIP("10.0.0.1"), "s1")
	allocated, _, _ := r.AllocateFromProfile("prof1", "", "", "s2")
	if !allocated.Equal(net.ParseIP("10.0.0.2")) {
		t.Fatalf("got %v, want 10.0.0.2 (10.0.0.1 should be reserved)", allocated)
	}
}

func TestRegistryAllocateIANA(t *testing.T) {
	profiles := map[string]*ip.IPv6Profile{
		"prof1": makeV6Profile([]ip.IANAPool{{
			Name:       "iana1",
			Network:    "2001:db8::/64",
			RangeStart: "2001:db8::1",
			RangeEnd:   "2001:db8::10",
		}}, nil),
	}
	r := newRegistry(nil, profiles)
	allocated, key, err := r.AllocateIANAFromProfile("prof1", "", "", "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allocated.Equal(net.ParseIP("2001:db8::1")) {
		t.Fatalf("got %v, want 2001:db8::1", allocated)
	}
	if key != "prof1/iana1" {
		t.Fatalf("key = %q, want %q", key, "prof1/iana1")
	}
}

func TestRegistryAllocateIANAPoolOverride(t *testing.T) {
	profiles := map[string]*ip.IPv6Profile{
		"prof1": makeV6Profile([]ip.IANAPool{
			{
				Name:       "default",
				Network:    "2001:db8::/64",
				RangeStart: "2001:db8::1",
				RangeEnd:   "2001:db8::1",
			},
			{
				Name:       "special",
				Network:    "2001:db8:1::/64",
				RangeStart: "2001:db8:1::1",
				RangeEnd:   "2001:db8:1::1",
			},
		}, nil),
	}
	r := newRegistry(nil, profiles)
	allocated, key, err := r.AllocateIANAFromProfile("prof1", "special", "", "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "prof1/special" {
		t.Fatalf("key = %q, want prof1/special", key)
	}
	if !allocated.Equal(net.ParseIP("2001:db8:1::1")) {
		t.Fatalf("got %v, want 2001:db8:1::1", allocated)
	}
}

func TestRegistryReleaseIANA(t *testing.T) {
	profiles := map[string]*ip.IPv6Profile{
		"prof1": makeV6Profile([]ip.IANAPool{{
			Name:       "iana1",
			Network:    "2001:db8::/64",
			RangeStart: "2001:db8::1",
			RangeEnd:   "2001:db8::1",
		}}, nil),
	}
	r := newRegistry(nil, profiles)
	ip1, key, _ := r.AllocateIANAFromProfile("prof1", "", "", "s1")

	r.ReleaseIANA(key, ip1)
	ip2, _, err := r.AllocateIANAFromProfile("prof1", "", "", "s2")
	if err != nil {
		t.Fatalf("unexpected error after ReleaseIANA: %v", err)
	}
	if !ip2.Equal(ip1) {
		t.Fatalf("got %v, want %v", ip2, ip1)
	}

	r.ReleaseIANAByIP(ip2)
	ip3, _, err := r.AllocateIANAFromProfile("prof1", "", "", "s3")
	if err != nil {
		t.Fatalf("unexpected error after ReleaseIANAByIP: %v", err)
	}
	if !ip3.Equal(ip1) {
		t.Fatalf("got %v, want %v", ip3, ip1)
	}
}

func TestRegistryReserveIANA(t *testing.T) {
	profiles := map[string]*ip.IPv6Profile{
		"prof1": makeV6Profile([]ip.IANAPool{{
			Name:       "iana1",
			Network:    "2001:db8::/64",
			RangeStart: "2001:db8::1",
			RangeEnd:   "2001:db8::2",
		}}, nil),
	}
	r := newRegistry(nil, profiles)
	r.ReserveIANA(net.ParseIP("2001:db8::1"), "s1")
	allocated, _, _ := r.AllocateIANAFromProfile("prof1", "", "", "s2")
	if !allocated.Equal(net.ParseIP("2001:db8::2")) {
		t.Fatalf("got %v, want 2001:db8::2 (::1 should be reserved)", allocated)
	}
}

func TestRegistryAllocatePD(t *testing.T) {
	profiles := map[string]*ip.IPv6Profile{
		"prof1": makeV6Profile(nil, []ip.PDPool{{
			Name:         "pd1",
			Network:      "2001:db8::/48",
			PrefixLength: 56,
		}}),
	}
	r := newRegistry(nil, profiles)
	pfx, key, err := r.AllocatePDFromProfile("prof1", "", "", "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pfx.String() != "2001:db8::/56" {
		t.Fatalf("got %v, want 2001:db8::/56", pfx)
	}
	if key != "prof1/pd1" {
		t.Fatalf("key = %q, want %q", key, "prof1/pd1")
	}
}

func TestRegistryAllocatePDPoolOverride(t *testing.T) {
	profiles := map[string]*ip.IPv6Profile{
		"prof1": makeV6Profile(nil, []ip.PDPool{
			{
				Name:         "default",
				Network:      "2001:db8::/48",
				PrefixLength: 56,
			},
			{
				Name:         "special",
				Network:      "2001:db8:1::/48",
				PrefixLength: 56,
			},
		}),
	}
	r := newRegistry(nil, profiles)
	pfx, key, err := r.AllocatePDFromProfile("prof1", "special", "", "s1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "prof1/special" {
		t.Fatalf("key = %q, want prof1/special", key)
	}
	if pfx.String() != "2001:db8:1::/56" {
		t.Fatalf("got %v, want 2001:db8:1::/56", pfx)
	}
}

func TestRegistryReleasePD(t *testing.T) {
	profiles := map[string]*ip.IPv6Profile{
		"prof1": makeV6Profile(nil, []ip.PDPool{{
			Name:         "pd1",
			Network:      "2001:db8::/126",
			PrefixLength: 127,
		}}),
	}
	r := newRegistry(nil, profiles)
	pfx1, key, _ := r.AllocatePDFromProfile("prof1", "", "", "s1")
	pfx2, _, _ := r.AllocatePDFromProfile("prof1", "", "", "s2")

	r.ReleasePD(key, pfx1)
	pfx3, _, err := r.AllocatePDFromProfile("prof1", "", "", "s3")
	if err != nil {
		t.Fatalf("unexpected error after ReleasePD: %v", err)
	}
	if pfx3.String() != pfx1.String() {
		t.Fatalf("got %v, want %v", pfx3, pfx1)
	}

	r.ReleasePDByPrefix(pfx2)
	pfx4, _, err := r.AllocatePDFromProfile("prof1", "", "", "s4")
	if err != nil {
		t.Fatalf("unexpected error after ReleasePDByPrefix: %v", err)
	}
	if pfx4.String() != pfx2.String() {
		t.Fatalf("got %v, want %v", pfx4, pfx2)
	}
}

func TestRegistryReservePD(t *testing.T) {
	profiles := map[string]*ip.IPv6Profile{
		"prof1": makeV6Profile(nil, []ip.PDPool{{
			Name:         "pd1",
			Network:      "2001:db8::/126",
			PrefixLength: 127,
		}}),
	}
	r := newRegistry(nil, profiles)

	reserved := &net.IPNet{
		IP:   net.ParseIP("2001:db8::"),
		Mask: net.CIDRMask(127, 128),
	}
	r.ReservePD(reserved, "s1")

	pfx, _, _ := r.AllocatePDFromProfile("prof1", "", "", "s2")
	if pfx.String() != "2001:db8::2/127" {
		t.Fatalf("got %v, want 2001:db8::2/127 (first prefix should be reserved)", pfx)
	}
}

func TestRegistryGatewayAndExcludeRanges(t *testing.T) {
	profiles := map[string]*ip.IPv4Profile{
		"prof1": makeV4Profile("10.0.0.1", ip.IPv4Pool{
			Name:       "pool1",
			Network:    "10.0.0.0/24",
			RangeStart: "10.0.0.1",
			RangeEnd:   "10.0.0.5",
			Exclude:    []string{"10.0.0.3-10.0.0.4"},
		}),
	}
	r := newRegistry(profiles, nil)

	var allocated []string
	for i := 0; i < 3; i++ {
		ip, _, err := r.AllocateFromProfile("prof1", "", "", "s")
		if err != nil {
			break
		}
		allocated = append(allocated, ip.String())
	}

	// 10.0.0.1 = gateway (excluded), 10.0.0.3-4 = exclude range
	// Available: 10.0.0.2, 10.0.0.5
	if len(allocated) != 2 {
		t.Fatalf("allocated %d addresses %v, want 2", len(allocated), allocated)
	}
	if allocated[0] != "10.0.0.2" {
		t.Fatalf("first = %q, want 10.0.0.2", allocated[0])
	}
	if allocated[1] != "10.0.0.5" {
		t.Fatalf("second = %q, want 10.0.0.5", allocated[1])
	}
}

func TestRegistryAllocateIANAVRFIsolation(t *testing.T) {
	profiles := map[string]*ip.IPv6Profile{
		"prof1": makeV6Profile([]ip.IANAPool{
			{
				Name:       "default-iana",
				Network:    "2001:db8::/64",
				RangeStart: "2001:db8::1",
				RangeEnd:   "2001:db8::1",
			},
			{
				Name:       "vrf-iana",
				Network:    "2001:db8:1::/64",
				RangeStart: "2001:db8:1::1",
				RangeEnd:   "2001:db8:1::1",
				VRF:        "CUSTOMER-A",
			},
		}, nil),
	}
	r := newRegistry(nil, profiles)

	t.Run("default subscriber skips VRF pool", func(t *testing.T) {
		ip1, key, err := r.AllocateIANAFromProfile("prof1", "", "", "s1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key != "prof1/default-iana" {
			t.Fatalf("key = %q, want prof1/default-iana", key)
		}
		if !ip1.Equal(net.ParseIP("2001:db8::1")) {
			t.Fatalf("got %v, want 2001:db8::1", ip1)
		}

		_, _, err = r.AllocateIANAFromProfile("prof1", "", "", "s2")
		if !errors.Is(err, ErrPoolExhausted) {
			t.Fatalf("got %v, want ErrPoolExhausted (should not overflow into VRF pool)", err)
		}
	})

	t.Run("VRF subscriber skips default pool", func(t *testing.T) {
		ip1, key, err := r.AllocateIANAFromProfile("prof1", "", "CUSTOMER-A", "s3")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key != "prof1/vrf-iana" {
			t.Fatalf("key = %q, want prof1/vrf-iana", key)
		}
		if !ip1.Equal(net.ParseIP("2001:db8:1::1")) {
			t.Fatalf("got %v, want 2001:db8:1::1", ip1)
		}
	})

	t.Run("pool override bypasses VRF check", func(t *testing.T) {
		r2 := newRegistry(nil, profiles)
		ip1, key, err := r2.AllocateIANAFromProfile("prof1", "vrf-iana", "", "s4")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key != "prof1/vrf-iana" {
			t.Fatalf("key = %q, want prof1/vrf-iana", key)
		}
		if !ip1.Equal(net.ParseIP("2001:db8:1::1")) {
			t.Fatalf("got %v, want 2001:db8:1::1", ip1)
		}
	})
}

func TestRegistryAllocatePDVRFIsolation(t *testing.T) {
	profiles := map[string]*ip.IPv6Profile{
		"prof1": makeV6Profile(nil, []ip.PDPool{
			{
				Name:         "default-pd",
				Network:      "2001:db8::/126",
				PrefixLength: 127,
			},
			{
				Name:         "vrf-pd",
				Network:      "2001:db8:1::/126",
				PrefixLength: 127,
				VRF:          "CUSTOMER-A",
			},
		}),
	}
	r := newRegistry(nil, profiles)

	t.Run("default subscriber skips VRF pool", func(t *testing.T) {
		pfx, key, err := r.AllocatePDFromProfile("prof1", "", "", "s1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key != "prof1/default-pd" {
			t.Fatalf("key = %q, want prof1/default-pd", key)
		}
		if pfx.String() != "2001:db8::/127" {
			t.Fatalf("got %v, want 2001:db8::/127", pfx)
		}

		r.AllocatePDFromProfile("prof1", "", "", "s1b")
		_, _, err = r.AllocatePDFromProfile("prof1", "", "", "s2")
		if !errors.Is(err, ErrPoolExhausted) {
			t.Fatalf("got %v, want ErrPoolExhausted (should not overflow into VRF pool)", err)
		}
	})

	t.Run("VRF subscriber skips default pool", func(t *testing.T) {
		pfx, key, err := r.AllocatePDFromProfile("prof1", "", "CUSTOMER-A", "s3")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key != "prof1/vrf-pd" {
			t.Fatalf("key = %q, want prof1/vrf-pd", key)
		}
		if pfx.String() != "2001:db8:1::/127" {
			t.Fatalf("got %v, want 2001:db8:1::/127", pfx)
		}
	})

	t.Run("pool override bypasses VRF check", func(t *testing.T) {
		r2 := newRegistry(nil, profiles)
		pfx, key, err := r2.AllocatePDFromProfile("prof1", "vrf-pd", "", "s4")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key != "prof1/vrf-pd" {
			t.Fatalf("key = %q, want prof1/vrf-pd", key)
		}
		if pfx.String() != "2001:db8:1::/127" {
			t.Fatalf("got %v, want 2001:db8:1::/127", pfx)
		}
	})
}

func TestRegistryAllocateVRFIsolation(t *testing.T) {
	profiles := map[string]*ip.IPv4Profile{
		"prof1": makeV4Profile("10.0.0.1", ip.IPv4Pool{
			Name:       "default-pool",
			Network:    "10.0.0.0/24",
			RangeStart: "10.0.0.2",
			RangeEnd:   "10.0.0.2",
			Priority:   1,
		}, ip.IPv4Pool{
			Name:       "vrf-pool",
			Network:    "192.168.1.0/24",
			RangeStart: "192.168.1.2",
			RangeEnd:   "192.168.1.2",
			VRF:        "CUSTOMER-A",
			Priority:   10,
		}),
	}
	r := newRegistry(profiles, nil)

	t.Run("default subscriber skips VRF pool", func(t *testing.T) {
		ip1, key, err := r.AllocateFromProfile("prof1", "", "", "s1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key != "prof1/default-pool" {
			t.Fatalf("key = %q, want prof1/default-pool", key)
		}
		if !ip1.Equal(net.ParseIP("10.0.0.2")) {
			t.Fatalf("got %v, want 10.0.0.2", ip1)
		}

		_, _, err = r.AllocateFromProfile("prof1", "", "", "s2")
		if !errors.Is(err, ErrPoolExhausted) {
			t.Fatalf("got %v, want ErrPoolExhausted (should not overflow into VRF pool)", err)
		}
	})

	t.Run("VRF subscriber skips default pool", func(t *testing.T) {
		ip1, key, err := r.AllocateFromProfile("prof1", "", "CUSTOMER-A", "s3")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key != "prof1/vrf-pool" {
			t.Fatalf("key = %q, want prof1/vrf-pool", key)
		}
		if !ip1.Equal(net.ParseIP("192.168.1.2")) {
			t.Fatalf("got %v, want 192.168.1.2", ip1)
		}
	})

	t.Run("pool override bypasses VRF check", func(t *testing.T) {
		r2 := newRegistry(profiles, nil)
		ip1, key, err := r2.AllocateFromProfile("prof1", "vrf-pool", "", "s4")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if key != "prof1/vrf-pool" {
			t.Fatalf("key = %q, want prof1/vrf-pool", key)
		}
		if !ip1.Equal(net.ParseIP("192.168.1.2")) {
			t.Fatalf("got %v, want 192.168.1.2", ip1)
		}
	})
}
