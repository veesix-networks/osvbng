package dhcp

import (
	"net"
	"testing"
	"time"

	"github.com/veesix-networks/osvbng/pkg/allocator"
	"github.com/veesix-networks/osvbng/pkg/config/ip"
)

func initRegistry(t *testing.T, v4 map[string]*ip.IPv4Profile, v6 map[string]*ip.IPv6Profile) {
	t.Helper()
	allocator.InitGlobalRegistry(v4, v6)
	t.Cleanup(allocator.ResetGlobalRegistry)
}

func TestResolveV4AllocateFromPool(t *testing.T) {
	v4 := map[string]*ip.IPv4Profile{
		"prof1": {
			Gateway: "10.0.0.1",
			Pools: []ip.IPv4Pool{{
				Name:       "pool1",
				Network:    "10.0.0.0/24",
				RangeStart: "10.0.0.2",
				RangeEnd:   "10.0.0.10",
			}},
		},
	}
	initRegistry(t, v4, nil)

	ctx := &allocator.Context{SessionID: "s1", ProfileName: "prof1"}
	profile := v4["prof1"]
	res := ResolveV4(ctx, profile)
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if !res.YourIP.Equal(net.ParseIP("10.0.0.2")) {
		t.Fatalf("YourIP = %v, want 10.0.0.2", res.YourIP)
	}
	if res.PoolName != "prof1/pool1" {
		t.Fatalf("PoolName = %q, want %q", res.PoolName, "prof1/pool1")
	}
}

func TestResolveV4ReserveAAAIP(t *testing.T) {
	v4 := map[string]*ip.IPv4Profile{
		"prof1": {
			Gateway: "10.0.0.1",
			Pools: []ip.IPv4Pool{{
				Name:       "pool1",
				Network:    "10.0.0.0/24",
				RangeStart: "10.0.0.2",
				RangeEnd:   "10.0.0.10",
			}},
		},
	}
	initRegistry(t, v4, nil)

	ctx := &allocator.Context{
		SessionID:   "s1",
		ProfileName: "prof1",
		IPv4Address: net.ParseIP("10.0.0.5"),
	}
	res := ResolveV4(ctx, v4["prof1"])
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if !res.YourIP.Equal(net.ParseIP("10.0.0.5")) {
		t.Fatalf("YourIP = %v, want 10.0.0.5", res.YourIP)
	}
	if res.PoolName != "" {
		t.Fatalf("PoolName = %q, want empty for reserved IP", res.PoolName)
	}
}

func TestResolveV4GatewayPrecedence(t *testing.T) {
	t.Run("ctx gateway wins over pool and profile", func(t *testing.T) {
		v4 := map[string]*ip.IPv4Profile{
			"prof1": {
				Gateway: "10.0.0.1",
				Pools: []ip.IPv4Pool{{
					Name:       "pool1",
					Network:    "10.0.0.0/24",
					RangeStart: "10.0.0.2",
					RangeEnd:   "10.0.0.10",
					Gateway:    "10.0.0.100",
				}},
			},
		}
		initRegistry(t, v4, nil)

		ctx := &allocator.Context{
			SessionID:   "s1",
			ProfileName: "prof1",
			IPv4Gateway: net.ParseIP("10.0.0.254"),
		}
		res := ResolveV4(ctx, v4["prof1"])
		if !res.Router.Equal(net.ParseIP("10.0.0.254")) {
			t.Fatalf("Router = %v, want 10.0.0.254 (ctx gateway)", res.Router)
		}
	})

	t.Run("pool gateway wins over profile", func(t *testing.T) {
		v4 := map[string]*ip.IPv4Profile{
			"prof1": {
				Gateway: "10.0.0.1",
				Pools: []ip.IPv4Pool{{
					Name:       "pool1",
					Network:    "192.168.1.0/24",
					RangeStart: "192.168.1.2",
					RangeEnd:   "192.168.1.10",
					Gateway:    "192.168.1.1",
				}},
			},
		}
		initRegistry(t, v4, nil)

		ctx := &allocator.Context{SessionID: "s1", ProfileName: "prof1"}
		res := ResolveV4(ctx, v4["prof1"])
		if !res.Router.Equal(net.ParseIP("192.168.1.1")) {
			t.Fatalf("Router = %v, want 192.168.1.1 (pool gateway)", res.Router)
		}
	})

	t.Run("profile gateway used when pool has none", func(t *testing.T) {
		v4 := map[string]*ip.IPv4Profile{
			"prof1": {
				Gateway: "10.0.0.1",
				Pools: []ip.IPv4Pool{{
					Name:       "pool1",
					Network:    "10.0.0.0/24",
					RangeStart: "10.0.0.2",
					RangeEnd:   "10.0.0.10",
				}},
			},
		}
		initRegistry(t, v4, nil)

		ctx := &allocator.Context{SessionID: "s1", ProfileName: "prof1"}
		res := ResolveV4(ctx, v4["prof1"])
		if !res.Router.Equal(net.ParseIP("10.0.0.1")) {
			t.Fatalf("Router = %v, want 10.0.0.1 (profile gateway)", res.Router)
		}
	})
}

func TestResolveV4ServerID(t *testing.T) {
	t.Run("from profile DHCP options", func(t *testing.T) {
		v4 := map[string]*ip.IPv4Profile{
			"prof1": {
				Gateway: "10.0.0.1",
				Pools: []ip.IPv4Pool{{
					Name:       "pool1",
					Network:    "10.0.0.0/24",
					RangeStart: "10.0.0.2",
					RangeEnd:   "10.0.0.10",
				}},
				DHCP: &ip.IPv4DHCPOptions{ServerID: "10.0.0.100"},
			},
		}
		initRegistry(t, v4, nil)

		ctx := &allocator.Context{SessionID: "s1", ProfileName: "prof1"}
		res := ResolveV4(ctx, v4["prof1"])
		if !res.ServerID.Equal(net.ParseIP("10.0.0.100")) {
			t.Fatalf("ServerID = %v, want 10.0.0.100", res.ServerID)
		}
	})

	t.Run("fallback to router", func(t *testing.T) {
		v4 := map[string]*ip.IPv4Profile{
			"prof1": {
				Gateway: "10.0.0.1",
				Pools: []ip.IPv4Pool{{
					Name:       "pool1",
					Network:    "10.0.0.0/24",
					RangeStart: "10.0.0.2",
					RangeEnd:   "10.0.0.10",
				}},
			},
		}
		initRegistry(t, v4, nil)

		ctx := &allocator.Context{SessionID: "s1", ProfileName: "prof1"}
		res := ResolveV4(ctx, v4["prof1"])
		if !res.ServerID.Equal(net.ParseIP("10.0.0.1")) {
			t.Fatalf("ServerID = %v, want 10.0.0.1 (fallback to router)", res.ServerID)
		}
	})
}

func TestResolveV4DNSPrecedence(t *testing.T) {
	v4 := map[string]*ip.IPv4Profile{
		"prof1": {
			Gateway: "10.0.0.1",
			DNS:     []string{"1.1.1.1"},
			Pools: []ip.IPv4Pool{{
				Name:       "pool1",
				Network:    "10.0.0.0/24",
				RangeStart: "10.0.0.2",
				RangeEnd:   "10.0.0.10",
			}},
		},
	}
	initRegistry(t, v4, nil)

	ctx := &allocator.Context{
		SessionID:   "s1",
		ProfileName: "prof1",
		DNSv4:       []net.IP{net.ParseIP("8.8.8.8")},
	}
	res := ResolveV4(ctx, v4["prof1"])
	if len(res.DNS) != 1 || !res.DNS[0].Equal(net.ParseIP("8.8.8.8")) {
		t.Fatalf("DNS = %v, want [8.8.8.8] (ctx overrides profile)", res.DNS)
	}
}

func TestResolveV4UnnumberedPTP(t *testing.T) {
	v4 := map[string]*ip.IPv4Profile{
		"prof1": {
			Gateway: "10.0.0.1",
			Pools: []ip.IPv4Pool{{
				Name:       "pool1",
				Network:    "10.0.0.0/24",
				RangeStart: "10.0.0.2",
				RangeEnd:   "10.0.0.10",
			}},
			DHCP: &ip.IPv4DHCPOptions{AddressModel: "unnumbered-ptp"},
		},
	}
	initRegistry(t, v4, nil)

	ctx := &allocator.Context{SessionID: "s1", ProfileName: "prof1"}
	res := ResolveV4(ctx, v4["prof1"])
	ones, bits := res.Netmask.Size()
	if ones != 32 || bits != 32 {
		t.Fatalf("Netmask = /%d (bits=%d), want /32", ones, bits)
	}
	if len(res.ClasslessRoutes) != 1 {
		t.Fatalf("ClasslessRoutes len = %d, want 1", len(res.ClasslessRoutes))
	}
	route := res.ClasslessRoutes[0]
	if route.Destination.String() != "0.0.0.0/0" {
		t.Fatalf("route dest = %v, want 0.0.0.0/0", route.Destination)
	}
	if !route.NextHop.Equal(net.ParseIP("10.0.0.1")) {
		t.Fatalf("route nexthop = %v, want 10.0.0.1", route.NextHop)
	}
}

func TestResolveV4DefaultNetmask(t *testing.T) {
	t.Run("from context", func(t *testing.T) {
		v4 := map[string]*ip.IPv4Profile{
			"prof1": {
				Gateway: "10.0.0.1",
				Pools: []ip.IPv4Pool{{
					Name:       "pool1",
					Network:    "10.0.0.0/24",
					RangeStart: "10.0.0.2",
					RangeEnd:   "10.0.0.10",
				}},
			},
		}
		initRegistry(t, v4, nil)

		ctx := &allocator.Context{
			SessionID:   "s1",
			ProfileName: "prof1",
			IPv4Netmask: net.CIDRMask(16, 32),
		}
		res := ResolveV4(ctx, v4["prof1"])
		ones, _ := res.Netmask.Size()
		if ones != 16 {
			t.Fatalf("Netmask = /%d, want /16 from context", ones)
		}
	})

	t.Run("fallback to pool CIDR", func(t *testing.T) {
		v4 := map[string]*ip.IPv4Profile{
			"prof1": {
				Gateway: "10.0.0.1",
				Pools: []ip.IPv4Pool{{
					Name:       "pool1",
					Network:    "10.0.0.0/24",
					RangeStart: "10.0.0.2",
					RangeEnd:   "10.0.0.10",
				}},
			},
		}
		initRegistry(t, v4, nil)

		ctx := &allocator.Context{SessionID: "s1", ProfileName: "prof1"}
		res := ResolveV4(ctx, v4["prof1"])
		ones, _ := res.Netmask.Size()
		if ones != 24 {
			t.Fatalf("Netmask = /%d, want /24 from pool CIDR", ones)
		}
	})
}

func TestResolveV4NilRegistry(t *testing.T) {
	allocator.ResetGlobalRegistry()
	t.Cleanup(allocator.ResetGlobalRegistry)

	ctx := &allocator.Context{SessionID: "s1", ProfileName: "prof1"}
	profile := &ip.IPv4Profile{Gateway: "10.0.0.1"}
	res := ResolveV4(ctx, profile)
	if res != nil {
		t.Fatalf("expected nil result with nil registry, got %+v", res)
	}
}

func TestResolveV6AllocateIANAAndPD(t *testing.T) {
	v6 := map[string]*ip.IPv6Profile{
		"prof1": makeTestV6Profile(),
	}
	initRegistry(t, nil, v6)

	ctx := &allocator.Context{SessionID: "s1", ProfileName: "prof1"}
	res := ResolveV6(ctx, v6["prof1"])
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if res.IANAAddress == nil {
		t.Fatal("expected IANA address")
	}
	if res.PDPrefix == nil {
		t.Fatal("expected PD prefix")
	}
	if res.IANAPoolName == "" {
		t.Fatal("expected non-empty IANAPoolName")
	}
	if res.PDPoolName == "" {
		t.Fatal("expected non-empty PDPoolName")
	}
	if res.IANAPreferredTime != 3600 {
		t.Fatalf("IANAPreferredTime = %d, want 3600 (default)", res.IANAPreferredTime)
	}
	if res.IANAValidTime != 7200 {
		t.Fatalf("IANAValidTime = %d, want 7200 (default)", res.IANAValidTime)
	}
}

func TestResolveV6ReserveAAAAddresses(t *testing.T) {
	v6 := map[string]*ip.IPv6Profile{
		"prof1": makeTestV6Profile(),
	}
	initRegistry(t, nil, v6)

	ctx := &allocator.Context{
		SessionID:   "s1",
		ProfileName: "prof1",
		IPv6Address: net.ParseIP("2001:db8::5"),
		IPv6Prefix: &net.IPNet{
			IP:   net.ParseIP("2001:db8:1::"),
			Mask: net.CIDRMask(56, 128),
		},
	}
	res := ResolveV6(ctx, v6["prof1"])
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if !res.IANAAddress.Equal(net.ParseIP("2001:db8::5")) {
		t.Fatalf("IANAAddress = %v, want 2001:db8::5", res.IANAAddress)
	}
	if res.IANAPoolName != "" {
		t.Fatalf("IANAPoolName = %q, want empty for reserved", res.IANAPoolName)
	}
	if res.PDPoolName != "" {
		t.Fatalf("PDPoolName = %q, want empty for reserved", res.PDPoolName)
	}
}

func TestResolveV6IANAOnlyNoPD(t *testing.T) {
	v6 := map[string]*ip.IPv6Profile{
		"prof1": {
			IANAPools: []ip.IANAPool{{
				Name:       "iana1",
				Network:    "2001:db8::/64",
				RangeStart: "2001:db8::1",
				RangeEnd:   "2001:db8::10",
			}},
		},
	}
	initRegistry(t, nil, v6)

	ctx := &allocator.Context{SessionID: "s1", ProfileName: "prof1"}
	res := ResolveV6(ctx, v6["prof1"])
	if res == nil {
		t.Fatal("expected non-nil result (IANA only)")
	}
	if res.IANAAddress == nil {
		t.Fatal("expected IANA address")
	}
	if res.PDPrefix != nil {
		t.Fatalf("PDPrefix = %v, want nil", res.PDPrefix)
	}
}

func TestResolveV6NeitherAllocated(t *testing.T) {
	initRegistry(t, nil, nil)

	ctx := &allocator.Context{SessionID: "s1", ProfileName: "prof1"}
	profile := &ip.IPv6Profile{}
	res := ResolveV6(ctx, profile)
	if res != nil {
		t.Fatalf("expected nil result, got %+v", res)
	}
}

func TestResolveV6DNSPrecedence(t *testing.T) {
	v6 := map[string]*ip.IPv6Profile{
		"prof1": {
			IANAPools: []ip.IANAPool{{
				Name:       "iana1",
				Network:    "2001:db8::/64",
				RangeStart: "2001:db8::1",
				RangeEnd:   "2001:db8::10",
			}},
			DNS: []string{"2001:4860:4860::8888"},
		},
	}
	initRegistry(t, nil, v6)

	ctx := &allocator.Context{
		SessionID:   "s1",
		ProfileName: "prof1",
		DNSv6:       []net.IP{net.ParseIP("fd00::53")},
	}
	res := ResolveV6(ctx, v6["prof1"])
	if len(res.DNS) != 1 || !res.DNS[0].Equal(net.ParseIP("fd00::53")) {
		t.Fatalf("DNS = %v, want [fd00::53] (ctx overrides profile)", res.DNS)
	}
}

func TestResolveV6PoolTimingOverrides(t *testing.T) {
	v6 := map[string]*ip.IPv6Profile{
		"prof1": {
			IANAPools: []ip.IANAPool{{
				Name:          "iana1",
				Network:       "2001:db8::/64",
				RangeStart:    "2001:db8::1",
				RangeEnd:      "2001:db8::10",
				PreferredTime: 600,
				ValidTime:     1200,
			}},
			PDPools: []ip.PDPool{{
				Name:          "pd1",
				Network:       "2001:db8:1::/48",
				PrefixLength:  56,
				PreferredTime: 900,
				ValidTime:     1800,
			}},
		},
	}
	initRegistry(t, nil, v6)

	ctx := &allocator.Context{SessionID: "s1", ProfileName: "prof1"}
	res := ResolveV6(ctx, v6["prof1"])
	if res.IANAPreferredTime != 600 {
		t.Fatalf("IANAPreferredTime = %d, want 600 (pool override)", res.IANAPreferredTime)
	}
	if res.IANAValidTime != 1200 {
		t.Fatalf("IANAValidTime = %d, want 1200 (pool override)", res.IANAValidTime)
	}
	if res.PDPreferredTime != 900 {
		t.Fatalf("PDPreferredTime = %d, want 900 (pool override)", res.PDPreferredTime)
	}
	if res.PDValidTime != 1800 {
		t.Fatalf("PDValidTime = %d, want 1800 (pool override)", res.PDValidTime)
	}
}

func TestResolveV4LeaseTime(t *testing.T) {
	v4 := map[string]*ip.IPv4Profile{
		"prof1": {
			Gateway: "10.0.0.1",
			Pools: []ip.IPv4Pool{{
				Name:       "pool1",
				Network:    "10.0.0.0/24",
				RangeStart: "10.0.0.2",
				RangeEnd:   "10.0.0.10",
			}},
			DHCP: &ip.IPv4DHCPOptions{LeaseTime: 7200},
		},
	}
	initRegistry(t, v4, nil)

	ctx := &allocator.Context{SessionID: "s1", ProfileName: "prof1"}
	res := ResolveV4(ctx, v4["prof1"])
	if res.LeaseTime != 7200*time.Second {
		t.Fatalf("LeaseTime = %v, want %v", res.LeaseTime, 7200*time.Second)
	}
}

func makeTestV6Profile() *ip.IPv6Profile {
	return &ip.IPv6Profile{
		IANAPools: []ip.IANAPool{{
			Name:       "iana1",
			Network:    "2001:db8::/64",
			RangeStart: "2001:db8::1",
			RangeEnd:   "2001:db8::10",
		}},
		PDPools: []ip.PDPool{{
			Name:         "pd1",
			Network:      "2001:db8:1::/48",
			PrefixLength: 56,
		}},
	}
}
