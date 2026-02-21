package vpp

import (
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip_types"
	"net"
)

func (v *VPP) toAddress(ip net.IP) (ip_types.Address, error) {
	var addr ip_types.Address
	if ip.To4() != nil {
		addr = ip_types.Address{
			Af: ip_types.ADDRESS_IP4,
			Un: ip_types.AddressUnionIP4(ip_types.IP4Address{
				ip.To4()[0], ip.To4()[1], ip.To4()[2], ip.To4()[3],
			}),
		}
	} else {
		var ip6 ip_types.IP6Address
		copy(ip6[:], ip.To16())
		addr = ip_types.Address{
			Af: ip_types.ADDRESS_IP6,
			Un: ip_types.AddressUnionIP6(ip6),
		}
	}
	return addr, nil
}
