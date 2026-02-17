package allocator

import (
	"net"

	"github.com/veesix-networks/osvbng/pkg/aaa"
)

type Context struct {
	SessionID string
	MAC       net.HardwareAddr
	SVLAN     uint16
	CVLAN     uint16

	VRF             string
	SubscriberGroup string
	ServiceGroup    string
	ProfileName     string

	IPv4Address net.IP
	IPv4Netmask net.IPMask
	IPv4Gateway net.IP
	IPv6Address net.IP
	IPv6Prefix  *net.IPNet

	DNSv4 []net.IP
	DNSv6 []net.IP

	PoolOverride     string
	IANAPoolOverride string
	PDPoolOverride   string
}

func NewContext(sessionID string, mac net.HardwareAddr, svlan, cvlan uint16, vrf, serviceGroup, profileName string, aaaAttrs map[string]interface{}) *Context {
	ctx := &Context{
		SessionID:    sessionID,
		MAC:          mac,
		SVLAN:        svlan,
		CVLAN:        cvlan,
		VRF:          vrf,
		ServiceGroup: serviceGroup,
		ProfileName:  profileName,
	}

	if v, ok := aaaAttrs[aaa.AttrIPv4Address]; ok {
		if s, ok := v.(string); ok {
			ctx.IPv4Address = net.ParseIP(s)
		}
	}
	if v, ok := aaaAttrs[aaa.AttrIPv4Netmask]; ok {
		if s, ok := v.(string); ok {
			if mask := net.ParseIP(s); mask != nil {
				ctx.IPv4Netmask = net.IPMask(mask.To4())
			}
		}
	}
	if v, ok := aaaAttrs[aaa.AttrIPv4Gateway]; ok {
		if s, ok := v.(string); ok {
			ctx.IPv4Gateway = net.ParseIP(s)
		}
	}
	if v, ok := aaaAttrs[aaa.AttrDNSPrimary]; ok {
		if s, ok := v.(string); ok {
			if ip := net.ParseIP(s); ip != nil {
				ctx.DNSv4 = append(ctx.DNSv4, ip)
			}
		}
	}
	if v, ok := aaaAttrs[aaa.AttrDNSSecondary]; ok {
		if s, ok := v.(string); ok {
			if ip := net.ParseIP(s); ip != nil {
				ctx.DNSv4 = append(ctx.DNSv4, ip)
			}
		}
	}
	if v, ok := aaaAttrs[aaa.AttrIPv6Address]; ok {
		if s, ok := v.(string); ok {
			ctx.IPv6Address = net.ParseIP(s)
		}
	}
	if v, ok := aaaAttrs[aaa.AttrIPv6Prefix]; ok {
		if s, ok := v.(string); ok {
			if _, ipnet, err := net.ParseCIDR(s); err == nil {
				ctx.IPv6Prefix = ipnet
			}
		}
	}
	if v, ok := aaaAttrs[aaa.AttrIPv6DNSPrimary]; ok {
		if s, ok := v.(string); ok {
			if ip := net.ParseIP(s); ip != nil {
				ctx.DNSv6 = append(ctx.DNSv6, ip)
			}
		}
	}
	if v, ok := aaaAttrs[aaa.AttrIPv6DNSSecondary]; ok {
		if s, ok := v.(string); ok {
			if ip := net.ParseIP(s); ip != nil {
				ctx.DNSv6 = append(ctx.DNSv6, ip)
			}
		}
	}
	if v, ok := aaaAttrs[aaa.AttrPool]; ok {
		if s, ok := v.(string); ok {
			ctx.PoolOverride = s
		}
	}
	if v, ok := aaaAttrs[aaa.AttrIANAPool]; ok {
		if s, ok := v.(string); ok {
			ctx.IANAPoolOverride = s
		}
	}
	if v, ok := aaaAttrs[aaa.AttrPDPool]; ok {
		if s, ok := v.(string); ok {
			ctx.PDPoolOverride = s
		}
	}

	return ctx
}
