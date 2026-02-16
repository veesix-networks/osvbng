package vpp

import (
	"fmt"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/fib_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/mfib_types"
	"net"
)

func (v *VPP) AddMfibLocalReceiveAllInterfaces(group net.IP, tableID uint32) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	var grpAddr ip_types.AddressUnion
	var af ip_types.AddressFamily
	var proto fib_types.FibPathNhProto
	var grpLen uint16

	if ip4 := group.To4(); ip4 != nil {
		var addr ip_types.IP4Address
		copy(addr[:], ip4)
		grpAddr.SetIP4(addr)
		af = ip_types.ADDRESS_IP4
		proto = fib_types.FIB_API_PATH_NH_PROTO_IP4
		grpLen = 32
	} else {
		var addr ip_types.IP6Address
		copy(addr[:], group.To16())
		grpAddr.SetIP6(addr)
		af = ip_types.ADDRESS_IP6
		proto = fib_types.FIB_API_PATH_NH_PROTO_IP6
		grpLen = 128
	}

	req := &ip.IPMrouteAddDel{
		IsAdd:       true,
		IsMultipath: true,
		Route: ip.IPMroute{
			TableID:    tableID,
			EntryFlags: mfib_types.MFIB_API_ENTRY_FLAG_ACCEPT_ALL_ITF,
			Prefix: ip_types.Mprefix{
				Af:               af,
				GrpAddressLength: grpLen,
				GrpAddress:       grpAddr,
			},
			NPaths: 1,
			Paths: []mfib_types.MfibPath{
				{
					ItfFlags: mfib_types.MFIB_API_ITF_FLAG_FORWARD,
					Path: fib_types.FibPath{
						SwIfIndex: ^uint32(0),
						Proto:     proto,
						Type:      fib_types.FIB_API_PATH_TYPE_LOCAL,
					},
				},
			},
		},
	}

	reply := &ip.IPMrouteAddDelReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("add mfib local receive: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("add mfib local receive failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Added MFIB local receive (all interfaces)", "group", group, "table", tableID)
	return nil
}


func (v *VPP) DumpMroutes() ([]southbound.MrouteInfo, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, err
	}
	defer ch.Close()

	req := &ip.IPMrouteDump{
		Table: ip.IPTable{
			TableID: 0,
			IsIP6:   true,
		},
	}

	stream := ch.SendMultiRequest(req)
	var result []southbound.MrouteInfo

	for {
		reply := &ip.IPMrouteDetails{}
		stop, err := stream.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("dump mroutes: %w", err)
		}

		var grpIP, srcIP net.IP
		if reply.Route.Prefix.Af == ip_types.ADDRESS_IP6 {
			grp6 := reply.Route.Prefix.GrpAddress.GetIP6()
			src6 := reply.Route.Prefix.SrcAddress.GetIP6()
			grpIP = net.IP(grp6[:])
			srcIP = net.IP(src6[:])
		} else {
			grp4 := reply.Route.Prefix.GrpAddress.GetIP4()
			src4 := reply.Route.Prefix.SrcAddress.GetIP4()
			grpIP = net.IP(grp4[:])
			srcIP = net.IP(src4[:])
		}

		info := southbound.MrouteInfo{
			TableID:    reply.Route.TableID,
			GrpAddress: grpIP,
			SrcAddress: srcIP,
			IsIPv6:     reply.Route.Prefix.Af == ip_types.ADDRESS_IP6,
		}
		result = append(result, info)
	}

	v.logger.Debug("Dumped mroutes", "count", len(result))
	return result, nil
}

