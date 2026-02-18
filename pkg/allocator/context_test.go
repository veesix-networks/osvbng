package allocator

import (
	"net"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/aaa"
)

func TestNewContextBasicFields(t *testing.T) {
	mac, _ := net.ParseMAC("aa:bb:cc:dd:ee:ff")
	ctx := NewContext("sess1", mac, 100, 200, "vrf-1", "svc-grp", "prof-1", nil)
	if ctx.SessionID != "sess1" {
		t.Fatalf("SessionID = %q, want %q", ctx.SessionID, "sess1")
	}
	if ctx.SVLAN != 100 || ctx.CVLAN != 200 {
		t.Fatalf("SVLAN/CVLAN = %d/%d, want 100/200", ctx.SVLAN, ctx.CVLAN)
	}
	if ctx.VRF != "vrf-1" {
		t.Fatalf("VRF = %q, want %q", ctx.VRF, "vrf-1")
	}
	if ctx.ServiceGroup != "svc-grp" {
		t.Fatalf("ServiceGroup = %q, want %q", ctx.ServiceGroup, "svc-grp")
	}
	if ctx.ProfileName != "prof-1" {
		t.Fatalf("ProfileName = %q, want %q", ctx.ProfileName, "prof-1")
	}
	if ctx.IPv4Address != nil {
		t.Fatalf("IPv4Address = %v, want nil", ctx.IPv4Address)
	}
	if ctx.IPv4Gateway != nil {
		t.Fatalf("IPv4Gateway = %v, want nil", ctx.IPv4Gateway)
	}
	if ctx.IPv6Address != nil {
		t.Fatalf("IPv6Address = %v, want nil", ctx.IPv6Address)
	}
}

func TestNewContextIPv4Address(t *testing.T) {
	attrs := map[string]interface{}{aaa.AttrIPv4Address: "10.0.0.5"}
	ctx := NewContext("s", nil, 0, 0, "", "", "", attrs)
	if !ctx.IPv4Address.Equal(net.ParseIP("10.0.0.5")) {
		t.Fatalf("IPv4Address = %v, want 10.0.0.5", ctx.IPv4Address)
	}

	t.Run("non-string value ignored", func(t *testing.T) {
		attrs := map[string]interface{}{aaa.AttrIPv4Address: 12345}
		ctx := NewContext("s", nil, 0, 0, "", "", "", attrs)
		if ctx.IPv4Address != nil {
			t.Fatalf("IPv4Address = %v, want nil for non-string", ctx.IPv4Address)
		}
	})

	t.Run("invalid IP ignored", func(t *testing.T) {
		attrs := map[string]interface{}{aaa.AttrIPv4Address: "not-an-ip"}
		ctx := NewContext("s", nil, 0, 0, "", "", "", attrs)
		if ctx.IPv4Address != nil {
			t.Fatalf("IPv4Address = %v, want nil for invalid IP", ctx.IPv4Address)
		}
	})
}

func TestNewContextIPv4Netmask(t *testing.T) {
	attrs := map[string]interface{}{aaa.AttrIPv4Netmask: "255.255.255.0"}
	ctx := NewContext("s", nil, 0, 0, "", "", "", attrs)
	want := net.IPMask(net.ParseIP("255.255.255.0").To4())
	if ctx.IPv4Netmask.String() != want.String() {
		t.Fatalf("IPv4Netmask = %v, want %v", ctx.IPv4Netmask, want)
	}
}

func TestNewContextIPv4Gateway(t *testing.T) {
	attrs := map[string]interface{}{aaa.AttrIPv4Gateway: "10.0.0.1"}
	ctx := NewContext("s", nil, 0, 0, "", "", "", attrs)
	if !ctx.IPv4Gateway.Equal(net.ParseIP("10.0.0.1")) {
		t.Fatalf("IPv4Gateway = %v, want 10.0.0.1", ctx.IPv4Gateway)
	}
}

func TestNewContextDNSv4(t *testing.T) {
	t.Run("both primary and secondary", func(t *testing.T) {
		attrs := map[string]interface{}{
			aaa.AttrDNSPrimary:   "8.8.8.8",
			aaa.AttrDNSSecondary: "8.8.4.4",
		}
		ctx := NewContext("s", nil, 0, 0, "", "", "", attrs)
		if len(ctx.DNSv4) != 2 {
			t.Fatalf("len(DNSv4) = %d, want 2", len(ctx.DNSv4))
		}
		if !ctx.DNSv4[0].Equal(net.ParseIP("8.8.8.8")) {
			t.Fatalf("DNSv4[0] = %v, want 8.8.8.8", ctx.DNSv4[0])
		}
		if !ctx.DNSv4[1].Equal(net.ParseIP("8.8.4.4")) {
			t.Fatalf("DNSv4[1] = %v, want 8.8.4.4", ctx.DNSv4[1])
		}
	})

	t.Run("primary only", func(t *testing.T) {
		attrs := map[string]interface{}{aaa.AttrDNSPrimary: "8.8.8.8"}
		ctx := NewContext("s", nil, 0, 0, "", "", "", attrs)
		if len(ctx.DNSv4) != 1 {
			t.Fatalf("len(DNSv4) = %d, want 1", len(ctx.DNSv4))
		}
	})
}

func TestNewContextIPv6Fields(t *testing.T) {
	attrs := map[string]interface{}{
		aaa.AttrIPv6Address: "2001:db8::1",
		aaa.AttrIPv6Prefix:  "2001:db8:1::/48",
	}
	ctx := NewContext("s", nil, 0, 0, "", "", "", attrs)
	if !ctx.IPv6Address.Equal(net.ParseIP("2001:db8::1")) {
		t.Fatalf("IPv6Address = %v, want 2001:db8::1", ctx.IPv6Address)
	}
	if ctx.IPv6Prefix == nil {
		t.Fatal("IPv6Prefix = nil, want non-nil")
	}
	if ctx.IPv6Prefix.String() != "2001:db8:1::/48" {
		t.Fatalf("IPv6Prefix = %v, want 2001:db8:1::/48", ctx.IPv6Prefix)
	}
}

func TestNewContextDNSv6(t *testing.T) {
	attrs := map[string]interface{}{
		aaa.AttrIPv6DNSPrimary:   "2001:4860:4860::8888",
		aaa.AttrIPv6DNSSecondary: "2001:4860:4860::8844",
	}
	ctx := NewContext("s", nil, 0, 0, "", "", "", attrs)
	if len(ctx.DNSv6) != 2 {
		t.Fatalf("len(DNSv6) = %d, want 2", len(ctx.DNSv6))
	}
	if !ctx.DNSv6[0].Equal(net.ParseIP("2001:4860:4860::8888")) {
		t.Fatalf("DNSv6[0] = %v, want 2001:4860:4860::8888", ctx.DNSv6[0])
	}
}

func TestNewContextPoolOverrides(t *testing.T) {
	attrs := map[string]interface{}{
		aaa.AttrPool:     "pool-v4",
		aaa.AttrIANAPool: "pool-iana",
		aaa.AttrPDPool:   "pool-pd",
	}
	ctx := NewContext("s", nil, 0, 0, "", "", "", attrs)
	if ctx.PoolOverride != "pool-v4" {
		t.Fatalf("PoolOverride = %q, want %q", ctx.PoolOverride, "pool-v4")
	}
	if ctx.IANAPoolOverride != "pool-iana" {
		t.Fatalf("IANAPoolOverride = %q, want %q", ctx.IANAPoolOverride, "pool-iana")
	}
	if ctx.PDPoolOverride != "pool-pd" {
		t.Fatalf("PDPoolOverride = %q, want %q", ctx.PDPoolOverride, "pool-pd")
	}

	t.Run("non-string pool ignored", func(t *testing.T) {
		attrs := map[string]interface{}{aaa.AttrPool: 42}
		ctx := NewContext("s", nil, 0, 0, "", "", "", attrs)
		if ctx.PoolOverride != "" {
			t.Fatalf("PoolOverride = %q, want empty for non-string", ctx.PoolOverride)
		}
	})
}
