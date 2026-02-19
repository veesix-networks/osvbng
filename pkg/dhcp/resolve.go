package dhcp

import (
	"net"
	"time"

	"github.com/veesix-networks/osvbng/pkg/allocator"
	"github.com/veesix-networks/osvbng/pkg/config/ip"
)

func ResolveV4(ctx *allocator.Context, profile *ip.IPv4Profile) *ResolvedDHCPv4 {
	var poolName string
	if ctx.IPv4Address == nil {
		registry := allocator.GetGlobalRegistry()
		if registry == nil {
			return nil
		}
		allocated, pn, err := registry.AllocateFromProfile(ctx.ProfileName, ctx.PoolOverride, ctx.VRF, ctx.SessionID)
		if err != nil {
			return nil
		}
		ctx.IPv4Address = allocated
		poolName = pn
	} else {
		if registry := allocator.GetGlobalRegistry(); registry != nil {
			if err := registry.ReserveIP(ctx.IPv4Address, ctx.SessionID); err != nil {
				return nil
			}
		}
	}

	resolved := &ResolvedDHCPv4{
		YourIP:    ctx.IPv4Address,
		LeaseTime: time.Duration(profile.GetLeaseTime()) * time.Second,
		PoolName:  poolName,
	}

	pool := findPoolForIP(ctx.IPv4Address, profile)

	if ctx.IPv4Gateway != nil {
		resolved.Router = ctx.IPv4Gateway
	} else if pool != nil && pool.Gateway != "" {
		if gw := net.ParseIP(pool.Gateway); gw != nil {
			resolved.Router = gw
		}
	} else if gw := net.ParseIP(profile.Gateway); gw != nil {
		resolved.Router = gw
	}

	if sid := net.ParseIP(profile.GetServerID()); sid != nil {
		resolved.ServerID = sid
	} else {
		resolved.ServerID = resolved.Router
	}

	if len(ctx.DNSv4) > 0 {
		resolved.DNS = ctx.DNSv4
	} else {
		for _, s := range profile.DNS {
			if dnsIP := net.ParseIP(s); dnsIP != nil {
				resolved.DNS = append(resolved.DNS, dnsIP)
			}
		}
	}

	switch profile.GetAddressModel() {
	case "unnumbered-ptp":
		resolved.Netmask = net.CIDRMask(32, 32)
		if resolved.Router != nil {
			resolved.ClasslessRoutes = []ClasslessRoute{
				{
					Destination: &net.IPNet{
						IP:   net.IPv4zero,
						Mask: net.CIDRMask(0, 32),
					},
					NextHop: resolved.Router,
				},
			}
		}
	default:
		if ctx.IPv4Netmask != nil {
			resolved.Netmask = ctx.IPv4Netmask
		} else if pool != nil {
			if _, poolNet, err := net.ParseCIDR(pool.Network); err == nil {
				resolved.Netmask = poolNet.Mask
			}
		}
	}

	return resolved
}

func ResolveV6(ctx *allocator.Context, profile *ip.IPv6Profile) *ResolvedDHCPv6 {
	registry := allocator.GetGlobalRegistry()

	var ianaPoolName string
	if ctx.IPv6Address == nil {
		if registry != nil {
			allocated, pn, err := registry.AllocateIANAFromProfile(ctx.ProfileName, ctx.IANAPoolOverride, ctx.VRF, ctx.SessionID)
			if err == nil {
				ctx.IPv6Address = allocated
				ianaPoolName = pn
			}
		}
	} else if registry != nil {
		if err := registry.ReserveIANA(ctx.IPv6Address, ctx.SessionID); err != nil {
			return nil
		}
	}

	var pdPoolName string
	if ctx.IPv6Prefix == nil {
		if registry != nil {
			allocated, pn, err := registry.AllocatePDFromProfile(ctx.ProfileName, ctx.PDPoolOverride, ctx.VRF, ctx.SessionID)
			if err == nil {
				ctx.IPv6Prefix = allocated
				pdPoolName = pn
			}
		}
	} else if registry != nil {
		if err := registry.ReservePD(ctx.IPv6Prefix, ctx.SessionID); err != nil {
			return nil
		}
	}

	if ctx.IPv6Address == nil && ctx.IPv6Prefix == nil {
		return nil
	}

	resolved := &ResolvedDHCPv6{
		IANAPoolName: ianaPoolName,
		PDPoolName:   pdPoolName,
	}

	if ctx.IPv6Address != nil {
		resolved.IANAAddress = ctx.IPv6Address
		resolved.IANAPreferredTime = profile.GetPreferredTime()
		resolved.IANAValidTime = profile.GetValidTime()
		if pool := findIANAPoolForAddr(ctx.IPv6Address, profile); pool != nil {
			if pool.PreferredTime > 0 {
				resolved.IANAPreferredTime = pool.PreferredTime
			}
			if pool.ValidTime > 0 {
				resolved.IANAValidTime = pool.ValidTime
			}
		}
	}

	if ctx.IPv6Prefix != nil {
		resolved.PDPrefix = ctx.IPv6Prefix
		resolved.PDPreferredTime = profile.GetPreferredTime()
		resolved.PDValidTime = profile.GetValidTime()
		if pool := findPDPoolForPrefix(ctx.IPv6Prefix, profile); pool != nil {
			if pool.PreferredTime > 0 {
				resolved.PDPreferredTime = pool.PreferredTime
			}
			if pool.ValidTime > 0 {
				resolved.PDValidTime = pool.ValidTime
			}
		}
	}

	if len(ctx.DNSv6) > 0 {
		resolved.DNS = ctx.DNSv6
	} else {
		for _, s := range profile.DNS {
			if dnsIP := net.ParseIP(s); dnsIP != nil {
				resolved.DNS = append(resolved.DNS, dnsIP)
			}
		}
	}

	return resolved
}

func findIANAPoolForAddr(addr net.IP, profile *ip.IPv6Profile) *ip.IANAPool {
	for i := range profile.IANAPools {
		_, poolNet, err := net.ParseCIDR(profile.IANAPools[i].Network)
		if err != nil {
			continue
		}
		if poolNet.Contains(addr) {
			return &profile.IANAPools[i]
		}
	}
	return nil
}

func findPDPoolForPrefix(prefix *net.IPNet, profile *ip.IPv6Profile) *ip.PDPool {
	for i := range profile.PDPools {
		_, poolNet, err := net.ParseCIDR(profile.PDPools[i].Network)
		if err != nil {
			continue
		}
		if poolNet.Contains(prefix.IP) {
			return &profile.PDPools[i]
		}
	}
	return nil
}

func findPoolForIP(clientIP net.IP, profile *ip.IPv4Profile) *ip.IPv4Pool {
	for i := range profile.Pools {
		_, poolNet, err := net.ParseCIDR(profile.Pools[i].Network)
		if err != nil {
			continue
		}
		if poolNet.Contains(clientIP) {
			return &profile.Pools[i]
		}
	}
	return nil
}
