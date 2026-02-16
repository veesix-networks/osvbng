package vpp

import (
	"errors"
	"fmt"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/fib_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip6_nd"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/mfib_types"
	"go.fd.io/govpp/api"
)

func (v *VPP) EnableIPv6(ifaceName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	req := &ip.SwInterfaceIP6EnableDisable{
		SwIfIndex: interface_types.InterfaceIndex(idx),
		Enable:    true,
	}

	reply := &ip.SwInterfaceIP6EnableDisableReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		var vppErr api.VPPApiError
		if errors.As(err, &vppErr) && vppErr == -81 {
			v.logger.Debug("IPv6 already enabled on interface", "interface", ifaceName)
			return nil
		}
		return fmt.Errorf("enable ipv6: %w", err)
	}

	v.logger.Debug("Enabled IPv6 on interface", "interface", ifaceName)
	return nil
}


func (v *VPP) EnableDHCPv6Multicast(ifaceName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	// DHCPv6 all-relay-agents-and-servers multicast: ff02::1:2
	var grpAddr ip_types.AddressUnion
	grpAddr.SetIP6(ip_types.IP6Address{0xff, 0x02, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x01, 0, 0x02})

	req := &ip.IPMrouteAddDel{
		IsAdd: true,
		Route: ip.IPMroute{
			TableID:    0,
			EntryFlags: mfib_types.MFIB_API_ENTRY_FLAG_ACCEPT_ALL_ITF,
			Prefix: ip_types.Mprefix{
				Af:               ip_types.ADDRESS_IP6,
				GrpAddressLength: 128,
				GrpAddress:       grpAddr,
			},
			NPaths: 1,
			Paths: []mfib_types.MfibPath{
				{
					ItfFlags: mfib_types.MFIB_API_ITF_FLAG_FORWARD,
					Path: fib_types.FibPath{
						SwIfIndex: uint32(idx),
						Proto:     fib_types.FIB_API_PATH_NH_PROTO_IP6,
						Type:      fib_types.FIB_API_PATH_TYPE_LOCAL,
					},
				},
			},
		},
	}

	reply := &ip.IPMrouteAddDelReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("add dhcpv6 mcast route: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("add dhcpv6 mcast route failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Enabled DHCPv6 multicast on interface", "interface", ifaceName)
	return nil
}


func (v *VPP) ConfigureIPv6RA(ifaceName string, config southbound.IPv6RAConfig) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	lifetime := config.RouterLifetime
	if lifetime > 9000 {
		lifetime = 9000
	}

	var managed, other uint8
	if config.Managed {
		managed = 1
	}
	if config.Other {
		other = 1
	}

	req := &ip6_nd.SwInterfaceIP6ndRaConfig{
		SwIfIndex:       interface_types.InterfaceIndex(idx),
		Suppress:        0,       // 0 = send RAs, 1 = suppress RAs
		Managed:         managed, // M flag: use DHCPv6 for address config
		Other:           other,   // O flag: use DHCPv6 for other config
		LlOption:        1,       // 1 = include source link-layer address option
		SendUnicast:     1,       // 1 = send unicast RA in response to RS
		Cease:           0,       // 1 = cease sending RAs
		IsNo:            false,   // true = disable/remove config
		DefaultRouter:   1,       // 1 = advertise as default router
		MaxInterval:     config.MaxInterval,
		MinInterval:     config.MinInterval,
		Lifetime:        lifetime,
		InitialCount:    3,  // number of initial RAs to send rapidly
		InitialInterval: 16, // interval between initial RAs (seconds)
	}

	reply := &ip6_nd.SwInterfaceIP6ndRaConfigReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("configure ipv6 ra: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("configure ipv6 ra failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Configured IPv6 RA", "interface", ifaceName,
		"managed", config.Managed, "other", config.Other, "lifetime", lifetime)
	return nil
}


func (v *VPP) isIPv6EnabledByIndex(swIfIndex uint32) bool {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return false
	}
	defer ch.Close()

	req := &ip.SwInterfaceIP6GetLinkLocalAddress{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
	}

	reply := &ip.SwInterfaceIP6GetLinkLocalAddressReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return false
	}

	return reply.Retval == 0
}


func (v *VPP) DumpIPv6Enabled() ([]uint32, error) {
	interfaces, err := v.DumpInterfaces()
	if err != nil {
		return nil, fmt.Errorf("dump interfaces: %w", err)
	}

	var result []uint32
	for _, iface := range interfaces {
		if v.isIPv6EnabledByIndex(iface.SwIfIndex) {
			result = append(result, iface.SwIfIndex)
		}
	}

	v.logger.Debug("Dumped IPv6 enabled interfaces", "count", len(result))
	return result, nil
}


func (v *VPP) DumpIPv6RA() ([]southbound.IPv6RAInfo, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, err
	}
	defer ch.Close()

	req := &ip6_nd.SwInterfaceIP6ndRaDump{
		SwIfIndex: interface_types.InterfaceIndex(^uint32(0)),
	}

	stream := ch.SendMultiRequest(req)
	var result []southbound.IPv6RAInfo

	for {
		reply := &ip6_nd.SwInterfaceIP6ndRaDetails{}
		stop, err := stream.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("dump ipv6 ra: %w", err)
		}

		info := southbound.IPv6RAInfo{
			SwIfIndex:          uint32(reply.SwIfIndex),
			Managed:            reply.AdvManagedFlag,
			Other:              reply.AdvOtherFlag,
			RouterLifetimeSecs: reply.AdvRouterLifetime,
			MaxIntervalSecs:    reply.MaxRadvInterval,
			MinIntervalSecs:    reply.MinRadvInterval,
			SendRadv:           reply.SendRadv,
		}
		result = append(result, info)
	}

	v.logger.Debug("Dumped IPv6 RA configs", "count", len(result))
	return result, nil
}


