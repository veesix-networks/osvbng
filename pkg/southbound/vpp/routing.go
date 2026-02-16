package vpp

import (
	"fmt"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/fib_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip_types"
	"net"
)

func (v *VPP) AddLocalRoute(prefix string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	_, ipNet, err := net.ParseCIDR(prefix)
	if err != nil {
		return fmt.Errorf("invalid prefix: %w", err)
	}

	prefixLen, _ := ipNet.Mask.Size()

	ipPrefix := ip_types.Prefix{
		Address: ip_types.Address{
			Af: ip_types.ADDRESS_IP4,
			Un: ip_types.AddressUnionIP4(ip_types.IP4Address{
				ipNet.IP.To4()[0], ipNet.IP.To4()[1], ipNet.IP.To4()[2], ipNet.IP.To4()[3],
			}),
		},
		Len: uint8(prefixLen),
	}

	req := &ip.IPRouteAddDel{
		IsAdd: true,
		Route: ip.IPRoute{
			TableID: 0,
			Prefix:  ipPrefix,
			NPaths:  1,
			Paths: []fib_types.FibPath{
				{
					SwIfIndex: ^uint32(0),
					Type:      fib_types.FIB_API_PATH_TYPE_LOCAL,
				},
			},
		},
	}

	reply := &ip.IPRouteAddDelReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("add local route: %w", err)
	}

	v.logger.Debug("Added local route", "prefix", prefix)
	return nil
}
