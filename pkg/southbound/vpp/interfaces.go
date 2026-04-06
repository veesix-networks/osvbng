package vpp

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/af_packet"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/bond"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ethernet_types"
	vppinterfaces "github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/lcp"
	"github.com/vishvananda/netlink"
	"go.fd.io/govpp/api"
)

func (v *VPP) resolveRxMode() interface_types.RxMode {
	switch v.rxMode {
	case "polling":
		return interface_types.RX_MODE_API_POLLING
	case "adaptive":
		return interface_types.RX_MODE_API_ADAPTIVE
	default:
		return interface_types.RX_MODE_API_INTERRUPT
	}
}

func (v *VPP) CreateSubinterface(params *southbound.SubinterfaceParams) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	subIfName := fmt.Sprintf("%s.%d", params.ParentIface, params.SubID)

	if iface := v.ifMgr.GetByName(subIfName); iface != nil {
		v.logger.Debug("Sub-interface already exists", "interface", subIfName, "sw_if_index", iface.SwIfIndex)
		return nil
	}

	parentIdx, err := v.GetInterfaceIndex(params.ParentIface)
	if err != nil {
		return fmt.Errorf("parent interface %s not found: %w", params.ParentIface, err)
	}

	flags := v.computeSubIfFlags(params)

	req := &vppinterfaces.CreateSubif{
		SwIfIndex:   interface_types.InterfaceIndex(parentIdx),
		SubID:       uint32(params.SubID),
		SubIfFlags:  flags,
		OuterVlanID: params.OuterVLAN,
	}
	if params.InnerVLAN != nil {
		req.InnerVlanID = *params.InnerVLAN
	}

	reply := &vppinterfaces.CreateSubifReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "-56") {
			v.logger.Debug("Sub-interface already exists in VPP", "interface", subIfName)
			if err := v.LoadInterfaces(); err != nil {
				return fmt.Errorf("reload interfaces: %w", err)
			}
			return nil
		}
		return fmt.Errorf("create sub-interface: %w", err)
	}

	var parentMAC []byte
	var parentMTU uint32
	if parent := v.ifMgr.Get(uint32(parentIdx)); parent != nil {
		parentMAC = parent.MAC
		parentMTU = parent.MTU
	}

	if parentMTU > 0 {
		tagOverhead := uint32(4)
		if params.InnerVLAN != nil || params.InnerVLANAny {
			tagOverhead = 8
		}
		mtu := parentMTU + tagOverhead
		mtuReq := &vppinterfaces.SwInterfaceSetMtu{
			SwIfIndex: reply.SwIfIndex,
			Mtu:       []uint32{mtu, 0, 0, 0},
		}
		mtuReply := &vppinterfaces.SwInterfaceSetMtuReply{}
		if err := ch.SendRequest(mtuReq).ReceiveReply(mtuReply); err != nil {
			v.logger.Warn("Failed to set sub-interface MTU", "interface", subIfName, "mtu", mtu, "error", err)
		}
	}

	ifEntry := &ifmgr.Interface{
		SwIfIndex:    uint32(reply.SwIfIndex),
		SupSwIfIndex: uint32(parentIdx),
		Name:         subIfName,
		Type:         ifmgr.IfTypeSub,
		OuterVlanID:  params.OuterVLAN,
		MAC:          parentMAC,
	}
	if params.InnerVLAN != nil {
		ifEntry.InnerVlanID = *params.InnerVLAN
		ifEntry.SubNumberOfTags = 2
	} else {
		ifEntry.SubNumberOfTags = 1
	}
	v.ifMgr.Add(ifEntry)

	v.logger.Info("Created sub-interface", "interface", subIfName, "sw_if_index", reply.SwIfIndex, "flags", flags)
	return nil
}

func (v *VPP) CreateLCPPair(ifName string) error {
	if err := v.createLCPPair(ifName, ifName, lcp.LCP_API_ITF_HOST_TAP); err != nil {
		return fmt.Errorf("create LCP pair for %s: %w", ifName, err)
	}
	if err := v.waitForLink(ifName, 10*time.Second); err != nil {
		return fmt.Errorf("wait for LCP link %s: %w", ifName, err)
	}
	return nil
}

func (v *VPP) HasLCPPair(ifName string) bool {
	_, _, err := v.findLink(ifName)
	return err == nil
}

func (v *VPP) BindInterfaceToVRF(vppIfName, vrfName string, hasLCP bool) error {
	return v.bindInterfaceToVRF(vppIfName, vppIfName, vrfName, hasLCP)
}

func (v *VPP) computeSubIfFlags(params *southbound.SubinterfaceParams) interface_types.SubIfFlags {
	tpid := params.VLANTpid
	if tpid == "" {
		if params.InnerVLAN != nil || params.InnerVLANAny {
			tpid = "dot1ad"
		} else {
			tpid = "dot1q"
		}
	}

	isDot1ad := tpid == "dot1ad"

	if params.InnerVLANAny && isDot1ad {
		return interface_types.SUB_IF_API_FLAG_ONE_TAG |
			interface_types.SUB_IF_API_FLAG_TWO_TAGS |
			interface_types.SUB_IF_API_FLAG_INNER_VLAN_ID_ANY |
			interface_types.SUB_IF_API_FLAG_DOT1AD
	}

	if params.InnerVLANAny {
		return interface_types.SUB_IF_API_FLAG_ONE_TAG |
			interface_types.SUB_IF_API_FLAG_TWO_TAGS |
			interface_types.SUB_IF_API_FLAG_INNER_VLAN_ID_ANY
	}

	if params.InnerVLAN != nil && isDot1ad {
		return interface_types.SUB_IF_API_FLAG_TWO_TAGS |
			interface_types.SUB_IF_API_FLAG_EXACT_MATCH |
			interface_types.SUB_IF_API_FLAG_DOT1AD
	}

	if params.InnerVLAN != nil {
		return interface_types.SUB_IF_API_FLAG_TWO_TAGS |
			interface_types.SUB_IF_API_FLAG_EXACT_MATCH
	}

	if isDot1ad {
		return interface_types.SUB_IF_API_FLAG_ONE_TAG |
			interface_types.SUB_IF_API_FLAG_EXACT_MATCH |
			interface_types.SUB_IF_API_FLAG_DOT1AD
	}

	return interface_types.SUB_IF_API_FLAG_ONE_TAG |
		interface_types.SUB_IF_API_FLAG_EXACT_MATCH
}

func (v *VPP) DeleteInterface(name string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	iface := v.ifMgr.GetByName(name)
	if iface == nil {
		return nil
	}

	if iface.DevType == "bond" {
		req := &bond.BondDelete{
			SwIfIndex: interface_types.InterfaceIndex(iface.SwIfIndex),
		}
		reply := &bond.BondDeleteReply{}
		if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
			return fmt.Errorf("delete bond interface: %w", err)
		}
		v.ifMgr.Remove(iface.SwIfIndex)
		v.logger.Debug("Deleted bond interface", "interface", name)
		return nil
	}

	req := &vppinterfaces.DeleteSubif{
		SwIfIndex: interface_types.InterfaceIndex(iface.SwIfIndex),
	}

	reply := &vppinterfaces.DeleteSubifReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("delete interface: %w", err)
	}

	v.ifMgr.Remove(iface.SwIfIndex)
	v.logger.Debug("Deleted interface", "interface", name)
	return nil
}

func (v *VPP) GetInterfaceIndex(name string) (int, error) {
	if iface := v.ifMgr.GetByName(name); iface != nil {
		v.logger.Debug("Using cached interface index", "name", name, "sw_if_index", iface.SwIfIndex)
		return int(iface.SwIfIndex), nil
	}

	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return 0, err
	}
	defer ch.Close()

	req := &vppinterfaces.SwInterfaceDump{
		NameFilterValid: true,
		NameFilter:      name,
	}

	v.logger.Debug("Looking up interface index", "name", name)
	stream := ch.SendMultiRequest(req)
	for {
		reply := &vppinterfaces.SwInterfaceDetails{}
		stop, err := stream.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("dump interface: %w", err)
		}

		v.logger.Debug("Checking interface from VPP", "requested_name", name, "vpp_interface_name", reply.InterfaceName, "sw_if_index", reply.SwIfIndex)
		if reply.InterfaceName == name || reply.InterfaceName == "host-"+name {
			v.logger.Debug("Found matching interface", "requested_name", name, "matched_name", reply.InterfaceName, "sw_if_index", reply.SwIfIndex)
			ifaceName := strings.TrimRight(reply.InterfaceName, "\x00")
			v.ifMgr.Add(&ifmgr.Interface{
				SwIfIndex:       uint32(reply.SwIfIndex),
				SupSwIfIndex:    reply.SupSwIfIndex,
				Name:            ifaceName,
				DevType:         strings.TrimRight(reply.InterfaceDevType, "\x00"),
				Type:            ifmgr.IfType(reply.Type),
				AdminUp:         reply.Flags&interface_types.IF_STATUS_API_FLAG_ADMIN_UP != 0,
				LinkUp:          reply.Flags&interface_types.IF_STATUS_API_FLAG_LINK_UP != 0,
				MTU:             reply.Mtu[0],
				MAC:             reply.L2Address[:],
				SubID:           reply.SubID,
				SubNumberOfTags: reply.SubNumberOfTags,
				OuterVlanID:     reply.SubOuterVlanID,
				InnerVlanID:     reply.SubInnerVlanID,
			})
			return int(reply.SwIfIndex), nil
		}
	}

	return 0, fmt.Errorf("interface %s not found", name)
}

func (v *VPP) SetInterfacePromiscuous(ifaceName string, on bool) error {
	link, err := netlink.LinkByName(ifaceName)
	if err != nil {
		return fmt.Errorf("find linux interface %s: %w", ifaceName, err)
	}
	if on {
		if err := netlink.SetPromiscOn(link); err != nil {
			return fmt.Errorf("set promiscuous on %s: %w", ifaceName, err)
		}
	} else {
		if err := netlink.SetPromiscOff(link); err != nil {
			return fmt.Errorf("set promiscuous off %s: %w", ifaceName, err)
		}
	}

	v.logger.Debug("Set interface promiscuous", "interface", ifaceName, "on", on)
	return nil
}

func (v *VPP) SetInterfaceMAC(swIfIndex interface_types.InterfaceIndex, mac net.HardwareAddr) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	var macAddr ethernet_types.MacAddress
	copy(macAddr[:], mac)

	req := &vppinterfaces.SwInterfaceSetMacAddress{
		SwIfIndex:  swIfIndex,
		MacAddress: macAddr,
	}

	reply := &vppinterfaces.SwInterfaceSetMacAddressReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("set mac address: %w", err)
	}

	v.logger.Debug("Set interface MAC", "sw_if_index", swIfIndex, "mac", mac.String())
	return nil
}

func (v *VPP) SetUnnumbered(ifaceName, loopbackName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	iface := v.ifMgr.GetByName(ifaceName)
	if iface == nil {
		if _, err := v.GetInterfaceIndex(ifaceName); err != nil {
			return fmt.Errorf("interface %s not found: %w", ifaceName, err)
		}
		iface = v.ifMgr.GetByName(ifaceName)
	}

	loopback := v.ifMgr.GetByName(loopbackName)
	if loopback == nil {
		if _, err := v.GetInterfaceIndex(loopbackName); err != nil {
			return fmt.Errorf("loopback %s not found: %w", loopbackName, err)
		}
		loopback = v.ifMgr.GetByName(loopbackName)
	}

	req := &vppinterfaces.SwInterfaceSetUnnumbered{
		SwIfIndex:           interface_types.InterfaceIndex(loopback.SwIfIndex),
		UnnumberedSwIfIndex: interface_types.InterfaceIndex(iface.SwIfIndex),
		IsAdd:               true,
	}

	reply := &vppinterfaces.SwInterfaceSetUnnumberedReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("set unnumbered: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("set unnumbered failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Set interface unnumbered", "interface", ifaceName, "loopback", loopbackName)
	return nil
}

func (v *VPP) SetUnnumberedAsync(swIfIndex uint32, loopbackName string, callback func(error)) {
	loopback := v.ifMgr.GetByName(loopbackName)
	if loopback == nil {
		callback(fmt.Errorf("loopback %s not found in ifMgr cache", loopbackName))
		return
	}

	req := &vppinterfaces.SwInterfaceSetUnnumbered{
		SwIfIndex:           interface_types.InterfaceIndex(loopback.SwIfIndex),
		UnnumberedSwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		IsAdd:               true,
	}

	v.asyncWorker.SendAsync(req, func(reply api.Message, err error) {
		if err != nil {
			callback(err)
			return
		}
		r, ok := reply.(*vppinterfaces.SwInterfaceSetUnnumberedReply)
		if !ok {
			callback(fmt.Errorf("unexpected reply type: %T", reply))
			return
		}
		if r.Retval != 0 {
			callback(fmt.Errorf("set unnumbered failed: retval=%d", r.Retval))
			return
		}
		v.logger.Debug("Set interface unnumbered (async)", "sw_if_index", swIfIndex, "loopback", loopbackName)
		callback(nil)
	})
}

func (v *VPP) getInterfaceAddresses(swIfIndex interface_types.InterfaceIndex) (map[string]bool, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, err
	}
	defer ch.Close()

	req := &ip.IPAddressDump{
		SwIfIndex: swIfIndex,
	}

	stream := ch.SendMultiRequest(req)
	addrs := make(map[string]bool)

	for {
		reply := &ip.IPAddressDetails{}
		stop, err := stream.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("get addresses: %w", err)
		}

		var ipStr string
		if reply.Prefix.Address.Af == ip_types.ADDRESS_IP4 {
			ip4 := reply.Prefix.Address.Un.GetIP4()
			ipStr = fmt.Sprintf("%d.%d.%d.%d/%d", ip4[0], ip4[1], ip4[2], ip4[3], reply.Prefix.Len)
		} else {
			ip6 := reply.Prefix.Address.Un.GetIP6()
			ipStr = fmt.Sprintf("%s/%d", net.IP(ip6[:]).String(), reply.Prefix.Len)
		}
		addrs[ipStr] = true
	}

	return addrs, nil
}


func (v *VPP) DumpInterfaces() ([]southbound.InterfaceInfo, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, err
	}
	defer ch.Close()

	req := &vppinterfaces.SwInterfaceDump{
		SwIfIndex: interface_types.InterfaceIndex(^uint32(0)),
	}

	stream := ch.SendMultiRequest(req)
	var result []southbound.InterfaceInfo

	for {
		reply := &vppinterfaces.SwInterfaceDetails{}
		stop, err := stream.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("dump interfaces: %w", err)
		}

		info := southbound.InterfaceInfo{
			SwIfIndex:    uint32(reply.SwIfIndex),
			Name:         strings.TrimRight(reply.InterfaceName, "\x00"),
			AdminUp:      reply.Flags&interface_types.IF_STATUS_API_FLAG_ADMIN_UP != 0,
			LinkUp:       reply.Flags&interface_types.IF_STATUS_API_FLAG_LINK_UP != 0,
			MTU:          reply.Mtu[0],
			OuterVlanID:  reply.SubOuterVlanID,
			InnerVlanID:  reply.SubInnerVlanID,
			SupSwIfIndex: reply.SupSwIfIndex,
		}
		result = append(result, info)
	}

	v.logger.Debug("Dumped interfaces", "count", len(result))
	return result, nil
}

func (v *VPP) LoadInterfaces() error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	req := &vppinterfaces.SwInterfaceDump{
		SwIfIndex: interface_types.InterfaceIndex(^uint32(0)),
	}

	stream := ch.SendMultiRequest(req)
	v.ifMgr.Clear()

	for {
		reply := &vppinterfaces.SwInterfaceDetails{}
		stop, err := stream.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return fmt.Errorf("load interfaces: %w", err)
		}

		ifaceName := strings.TrimRight(reply.InterfaceName, "\x00")

		v.ifMgr.Add(&ifmgr.Interface{
			SwIfIndex:       uint32(reply.SwIfIndex),
			SupSwIfIndex:    reply.SupSwIfIndex,
			Name:            ifaceName,
			DevType:         strings.TrimRight(reply.InterfaceDevType, "\x00"),
			Type:            ifmgr.IfType(reply.Type),
			AdminUp:         reply.Flags&interface_types.IF_STATUS_API_FLAG_ADMIN_UP != 0,
			LinkUp:          reply.Flags&interface_types.IF_STATUS_API_FLAG_LINK_UP != 0,
			MTU:             reply.Mtu[0],
			LinkSpeed:       reply.LinkSpeed,
			MAC:             reply.L2Address[:],
			SubID:           reply.SubID,
			SubNumberOfTags: reply.SubNumberOfTags,
			OuterVlanID:     reply.SubOuterVlanID,
			InnerVlanID:     reply.SubInnerVlanID,
			Tag:             strings.TrimRight(reply.Tag, "\x00"),
		})
	}

	v.logger.Debug("Loaded interfaces into ifMgr", "count", len(v.ifMgr.List()))
	return nil
}

func (v *VPP) LoadIPState() error {
	ifaces := v.ifMgr.List()
	totalAddrs := 0

	for _, iface := range ifaces {
		addrs, err := v.dumpIPAddressesForInterface(iface.SwIfIndex)
		if err != nil {
			v.logger.Warn("Failed to dump IPs for interface", "sw_if_index", iface.SwIfIndex, "name", iface.Name, "error", err)
			continue
		}

		for _, info := range addrs {
			ipAddr, _, err := net.ParseCIDR(info.Address)
			if err != nil {
				v.logger.Warn("Failed to parse IP from dump", "address", info.Address, "error", err)
				continue
			}

			if info.IsIPv6 {
				v.ifMgr.AddIPv6Address(info.SwIfIndex, ipAddr)
			} else {
				v.ifMgr.AddIPv4Address(info.SwIfIndex, ipAddr)
			}
			totalAddrs++
		}

		if len(addrs) > 0 {
			tableID, err := v.GetFIBIDForInterface(iface.SwIfIndex)
			if err != nil {
				v.logger.Warn("Failed to get FIB table for interface", "sw_if_index", iface.SwIfIndex, "error", err)
				continue
			}
			v.ifMgr.SetFIBTableID(iface.SwIfIndex, tableID)
		}
	}

	v.logger.Info("Loaded IP state into ifMgr", "addresses", totalAddrs, "interfaces", len(ifaces))
	return nil
}

func (v *VPP) dumpIPAddressesForInterface(swIfIndex uint32) ([]southbound.IPAddressInfo, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, err
	}
	defer ch.Close()

	var result []southbound.IPAddressInfo

	reqV4 := &ip.IPAddressDump{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		IsIPv6:    false,
	}
	streamV4 := ch.SendMultiRequest(reqV4)
	for {
		reply := &ip.IPAddressDetails{}
		stop, err := streamV4.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("dump ipv4 addresses: %w", err)
		}
		addr := fmt.Sprintf("%s/%d", reply.Prefix.Address.Un.GetIP4().String(), reply.Prefix.Len)
		result = append(result, southbound.IPAddressInfo{
			SwIfIndex: uint32(reply.SwIfIndex),
			Address:   addr,
			IsIPv6:    false,
		})
	}

	reqV6 := &ip.IPAddressDump{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		IsIPv6:    true,
	}
	streamV6 := ch.SendMultiRequest(reqV6)
	for {
		reply := &ip.IPAddressDetails{}
		stop, err := streamV6.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("dump ipv6 addresses: %w", err)
		}
		addr := fmt.Sprintf("%s/%d", reply.Prefix.Address.Un.GetIP6().String(), reply.Prefix.Len)
		result = append(result, southbound.IPAddressInfo{
			SwIfIndex: uint32(reply.SwIfIndex),
			Address:   addr,
			IsIPv6:    true,
		})
	}

	return result, nil
}

func (v *VPP) GetIfMgr() *ifmgr.Manager {
	return v.ifMgr
}

func (v *VPP) DumpIPAddresses() ([]southbound.IPAddressInfo, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, err
	}
	defer ch.Close()

	var result []southbound.IPAddressInfo
	reqV4 := &ip.IPAddressDump{
		SwIfIndex: interface_types.InterfaceIndex(^uint32(0)),
		IsIPv6:    false,
	}

	streamV4 := ch.SendMultiRequest(reqV4)
	for {
		reply := &ip.IPAddressDetails{}
		stop, err := streamV4.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("dump ipv4 addresses: %w", err)
		}

		addr := fmt.Sprintf("%s/%d",
			reply.Prefix.Address.Un.GetIP4().String(),
			reply.Prefix.Len)
		result = append(result, southbound.IPAddressInfo{
			SwIfIndex: uint32(reply.SwIfIndex),
			Address:   addr,
			IsIPv6:    false,
		})
	}

	reqV6 := &ip.IPAddressDump{
		SwIfIndex: interface_types.InterfaceIndex(^uint32(0)),
		IsIPv6:    true,
	}

	streamV6 := ch.SendMultiRequest(reqV6)
	for {
		reply := &ip.IPAddressDetails{}
		stop, err := streamV6.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("dump ipv6 addresses: %w", err)
		}

		addr := fmt.Sprintf("%s/%d",
			reply.Prefix.Address.Un.GetIP6().String(),
			reply.Prefix.Len)
		result = append(result, southbound.IPAddressInfo{
			SwIfIndex: uint32(reply.SwIfIndex),
			Address:   addr,
			IsIPv6:    true,
		})
	}

	v.logger.Debug("Dumped IP addresses", "count", len(result))
	return result, nil
}

func (v *VPP) DumpUnnumbered() ([]southbound.UnnumberedInfo, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, err
	}
	defer ch.Close()

	req := &ip.IPUnnumberedDump{
		SwIfIndex: interface_types.InterfaceIndex(^uint32(0)),
	}

	stream := ch.SendMultiRequest(req)
	var result []southbound.UnnumberedInfo

	for {
		reply := &ip.IPUnnumberedDetails{}
		stop, err := stream.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("dump unnumbered: %w", err)
		}

		info := southbound.UnnumberedInfo{
			SwIfIndex:   uint32(reply.SwIfIndex),
			IPSwIfIndex: uint32(reply.IPSwIfIndex),
		}
		result = append(result, info)
	}

	v.logger.Debug("Dumped unnumbered", "count", len(result))
	return result, nil
}

func (v *VPP) CreateInterface(cfg *interfaces.InterfaceConfig) error {
	ifType := inferInterfaceType(cfg)

	switch ifType {
	case "bond":
		return v.createBondInterface(cfg)
	case "loopback":
		return v.createLoopback(cfg)
	case "physical":
		return v.createPhysicalInterface(cfg)
	}

	return fmt.Errorf("unknown interface type for %s", cfg.Name)
}

func (v *VPP) createPhysicalInterface(cfg *interfaces.InterfaceConfig) error {
	// DPDK path: interface already exists in VPP (no AF_PACKET creation needed)
	if _, err := v.GetInterfaceIndex(cfg.Name); err == nil {
		v.logger.Info("Interface already exists in VPP, skipping creation", "interface", cfg.Name)
		if cfg.Enabled {
			if err := v.setInterfaceState(cfg.Name, true); err != nil {
				v.logger.Warn("Failed to set interface up", "interface", cfg.Name, "error", err)
			}
		}

		if cfg.LCP {
			if err := v.createLCPPair(cfg.Name, cfg.Name, lcp.LCP_API_ITF_HOST_TAP); err != nil {
				return fmt.Errorf("create LCP pair: %w", err)
			}
			if err := v.waitForLink(cfg.Name, 10*time.Second); err != nil {
				return fmt.Errorf("wait for LCP interface: %w", err)
			}
		}

		if cfg.Description != "" {
			v.SetInterfaceDescription(cfg.Name, cfg.Description)
		}

		if cfg.VRF != "" {
			if err := v.bindInterfaceToVRF(cfg.Name, cfg.Name, cfg.VRF, cfg.LCP); err != nil {
				return fmt.Errorf("bind to VRF: %w", err)
			}
		}

		return nil
	}

	// AF_PACKET path: create host-interface, then rename
	vppIfName, err := v.createVPPHostInterface(cfg.Name)
	if err != nil {
		if idx, lookupErr := v.GetInterfaceIndex("host-" + cfg.Name); lookupErr == nil {
			v.logger.Info("Host-interface already exists in VPP, skipping creation", "interface", cfg.Name)
			vppIfName = "host-" + cfg.Name
			_ = idx
		} else {
			return fmt.Errorf("create VPP host-interface: %w", err)
		}
	}

	// Rename VPP interface from "host-ethX" to "ethX"
	if err := v.renameVPPInterface(vppIfName, cfg.Name); err != nil {
		v.logger.Warn("Failed to rename VPP interface, continuing with original name",
			"old_name", vppIfName, "new_name", cfg.Name, "error", err)
	} else {
		vppIfName = cfg.Name
	}

	// Match VPP interface MTU to underlying Linux interface
	if hostMTU, err := v.getLinuxInterfaceMTU(cfg.Name); err == nil && hostMTU > 0 {
		if err := v.setVPPInterfaceHWMtu(vppIfName, uint16(hostMTU)); err != nil {
			v.logger.Warn("Failed to set VPP interface MTU", "interface", vppIfName, "mtu", hostMTU, "error", err)
		} else {
			v.logger.Info("Set VPP interface MTU to match host", "interface", vppIfName, "mtu", hostMTU)
		}
	}

	if cfg.Enabled {
		if err := v.setInterfaceState(vppIfName, true); err != nil {
			v.logger.Warn("Failed to set interface up", "interface", vppIfName, "error", err)
		}
	}

	if cfg.LCP {
		if err := v.createLCPPair(vppIfName, cfg.Name, lcp.LCP_API_ITF_HOST_TAP); err != nil {
			return fmt.Errorf("create LCP pair: %w", err)
		}
		if err := v.waitForLink(cfg.Name, 10*time.Second); err != nil {
			return fmt.Errorf("wait for LCP interface: %w", err)
		}
	}

	if cfg.Description != "" {
		v.SetInterfaceDescription(cfg.Name, cfg.Description)
	}

	if cfg.VRF != "" {
		if err := v.bindInterfaceToVRF(vppIfName, cfg.Name, cfg.VRF, cfg.LCP); err != nil {
			return fmt.Errorf("bind to VRF: %w", err)
		}
	}

	return nil
}

func (v *VPP) createBondInterface(cfg *interfaces.InterfaceConfig) error {
	if !v.useDPDK {
		v.logger.Warn("Bond config ignored in AF_PACKET mode, host OS manages the bond", "interface", cfg.Name)
		return v.createPhysicalInterface(cfg)
	}

	if err := cfg.Bond.Validate(); err != nil {
		return fmt.Errorf("invalid bond config for %s: %w", cfg.Name, err)
	}

	for _, member := range cfg.Bond.Members {
		iface := v.ifMgr.GetByName(member.Name)
		if iface == nil {
			return fmt.Errorf("bond member %q not found in VPP", member.Name)
		}
		if iface.Type == ifmgr.IfTypeSub {
			return fmt.Errorf("bond member %q is a subinterface", member.Name)
		}
		if iface.SupSwIfIndex != iface.SwIfIndex {
			return fmt.Errorf("bond member %q is a parented interface", member.Name)
		}
		if iface.DevType == "loopback" || iface.DevType == "host" {
			return fmt.Errorf("bond member %q has unsupported device type %q", member.Name, iface.DevType)
		}
	}

	mode := bond.BOND_API_MODE_LACP
	switch cfg.Bond.Mode {
	case "round-robin":
		mode = bond.BOND_API_MODE_ROUND_ROBIN
	case "active-backup":
		mode = bond.BOND_API_MODE_ACTIVE_BACKUP
	case "xor":
		mode = bond.BOND_API_MODE_XOR
	case "broadcast":
		mode = bond.BOND_API_MODE_BROADCAST
	case "lacp", "":
		mode = bond.BOND_API_MODE_LACP
	}

	lb := bond.BOND_API_LB_ALGO_L2
	switch cfg.Bond.LoadBalance {
	case "l23":
		lb = bond.BOND_API_LB_ALGO_L23
	case "l34":
		lb = bond.BOND_API_LB_ALGO_L34
	case "l2", "":
		lb = bond.BOND_API_LB_ALGO_L2
	}

	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	createReq := &bond.BondCreate2{
		Mode:      mode,
		Lb:        lb,
		EnableGso: cfg.Bond.GSO,
	}

	if cfg.Bond.MACAddress != "" {
		mac, _ := net.ParseMAC(cfg.Bond.MACAddress)
		createReq.UseCustomMac = true
		copy(createReq.MacAddress[:], mac)
	}

	createReply := &bond.BondCreate2Reply{}
	if err := ch.SendRequest(createReq).ReceiveReply(createReply); err != nil {
		return fmt.Errorf("create bond interface: %w", err)
	}

	bondSwIfIndex := createReply.SwIfIndex

	rollback := func() {
		delReq := &bond.BondDelete{SwIfIndex: bondSwIfIndex}
		delReply := &bond.BondDeleteReply{}
		if err := ch.SendRequest(delReq).ReceiveReply(delReply); err != nil {
			v.logger.Error("Failed to rollback bond creation", "sw_if_index", bondSwIfIndex, "error", err)
		} else {
			v.logger.Info("Rolled back bond creation", "sw_if_index", bondSwIfIndex)
		}
	}

	var addedMembers []interface_types.InterfaceIndex
	for _, member := range cfg.Bond.Members {
		memberIdx, _ := v.GetInterfaceIndex(member.Name)

		addReq := &bond.BondAddMember{
			SwIfIndex:     interface_types.InterfaceIndex(memberIdx),
			BondSwIfIndex: bondSwIfIndex,
			IsPassive:     member.Passive,
			IsLongTimeout: member.LongTimeout,
		}
		addReply := &bond.BondAddMemberReply{}
		if err := ch.SendRequest(addReq).ReceiveReply(addReply); err != nil {
			for _, idx := range addedMembers {
				detachReq := &bond.BondDetachMember{SwIfIndex: idx}
				detachReply := &bond.BondDetachMemberReply{}
				_ = ch.SendRequest(detachReq).ReceiveReply(detachReply)
			}
			rollback()
			return fmt.Errorf("add bond member %s: %w", member.Name, err)
		}
		addedMembers = append(addedMembers, interface_types.InterfaceIndex(memberIdx))
		v.logger.Debug("Added bond member", "member", member.Name, "bond_sw_if_index", bondSwIfIndex)
	}

	if err := v.LoadInterfaces(); err != nil {
		v.logger.Warn("Failed to reload interfaces after bond creation", "error", err)
	}

	vppIfName := cfg.Name
	if iface := v.ifMgr.Get(uint32(bondSwIfIndex)); iface != nil && iface.Name != cfg.Name {
		if err := v.renameVPPInterface(iface.Name, cfg.Name); err != nil {
			v.logger.Warn("Failed to rename bond interface, continuing with VPP name",
				"vpp_name", iface.Name, "config_name", cfg.Name, "error", err)
			vppIfName = iface.Name
		}
	}

	if cfg.Enabled {
		if err := v.setInterfaceState(vppIfName, true); err != nil {
			for _, idx := range addedMembers {
				detachReq := &bond.BondDetachMember{SwIfIndex: idx}
				detachReply := &bond.BondDetachMemberReply{}
				_ = ch.SendRequest(detachReq).ReceiveReply(detachReply)
			}
			rollback()
			return fmt.Errorf("set bond interface up: %w", err)
		}
	}

	if cfg.LCP {
		if err := v.createLCPPair(vppIfName, cfg.Name, lcp.LCP_API_ITF_HOST_TAP); err != nil {
			for _, idx := range addedMembers {
				detachReq := &bond.BondDetachMember{SwIfIndex: idx}
				detachReply := &bond.BondDetachMemberReply{}
				_ = ch.SendRequest(detachReq).ReceiveReply(detachReply)
			}
			rollback()
			return fmt.Errorf("create LCP pair for bond: %w", err)
		}
		if err := v.waitForLink(cfg.Name, 10*time.Second); err != nil {
			return fmt.Errorf("wait for bond LCP interface: %w", err)
		}
	}

	if cfg.Description != "" {
		v.SetInterfaceDescription(cfg.Name, cfg.Description)
	}

	if cfg.VRF != "" {
		if err := v.bindInterfaceToVRF(vppIfName, cfg.Name, cfg.VRF, cfg.LCP); err != nil {
			return fmt.Errorf("bind bond to VRF: %w", err)
		}
	}

	v.logger.Info("Created bond interface", "name", cfg.Name, "mode", cfg.Bond.Mode, "members", len(cfg.Bond.Members), "sw_if_index", bondSwIfIndex)
	return nil
}

func (v *VPP) createLoopback(cfg *interfaces.InterfaceConfig) error {
	// Check if loopback already exists in VPP
	if _, err := v.GetInterfaceIndex(cfg.Name); err == nil {
		v.logger.Info("Loopback already exists in VPP, skipping creation", "interface", cfg.Name)
		if cfg.Enabled {
			if err := v.setInterfaceState(cfg.Name, true); err != nil {
				v.logger.Warn("Failed to set interface up", "interface", cfg.Name, "error", err)
			}
		}
		return nil
	}

	vppIfName, swIfIndex, err := v.createVPPLoopback(cfg.Name)
	if err != nil {
		return fmt.Errorf("create VPP loopback: %w", err)
	}

	v.ifMgr.Add(&ifmgr.Interface{
		SwIfIndex:    swIfIndex,
		SupSwIfIndex: swIfIndex,
		Name:         vppIfName,
		DevType:      "loopback",
		Type:         ifmgr.IfTypeHardware,
	})

	if cfg.LCP {
		if err := v.createLCPPair(vppIfName, cfg.Name, lcp.LCP_API_ITF_HOST_TAP); err != nil {
			return fmt.Errorf("create LCP pair: %w", err)
		}
		if err := v.waitForLink(cfg.Name, 10*time.Second); err != nil {
			return fmt.Errorf("wait for LCP interface: %w", err)
		}

		if cfg.Description != "" {
			v.SetInterfaceDescription(cfg.Name, cfg.Description)
		}
	}

	if cfg.VRF != "" {
		if err := v.bindInterfaceToVRF(vppIfName, cfg.Name, cfg.VRF, cfg.LCP); err != nil {
			return fmt.Errorf("bind to VRF: %w", err)
		}
	}

	if cfg.Enabled {
		if err := v.setInterfaceState(vppIfName, true); err != nil {
			v.logger.Warn("Failed to set interface up", "interface", vppIfName, "error", err)
		}
	}

	return nil
}

func (v *VPP) SetInterfaceDescription(name, description string) error {
	idx, err := v.GetInterfaceIndex(name)
	if err == nil {
		ch, chErr := v.conn.NewAPIChannel()
		if chErr == nil {
			tagReq := &vppinterfaces.SwInterfaceTagAddDel{
				IsAdd:     true,
				SwIfIndex: interface_types.InterfaceIndex(idx),
				Tag:       description,
			}
			tagReply := &vppinterfaces.SwInterfaceTagAddDelReply{}
			if tagErr := ch.SendRequest(tagReq).ReceiveReply(tagReply); tagErr != nil {
				v.logger.Warn("Failed to set VPP interface tag", "interface", name, "error", tagErr)
			}
			if iface := v.ifMgr.GetByName(name); iface != nil {
				iface.Tag = description
			}
			ch.Close()
		}
	}

	link, h, err := v.findLink(name)
	if err != nil {
		v.logger.Debug("No LCP link for description alias", "interface", name)
		return nil
	}

	if h != nil {
		return h.LinkSetAlias(link, description)
	}
	return netlink.LinkSetAlias(link, description)
}

func (v *VPP) SetInterfaceMTU(name string, mtu int) error {
	idx, err := v.GetInterfaceIndex(name)
	if err == nil {
		// Set HW MTU first (ceiling), then SW MTU. SW MTU cannot exceed HW MTU.
		if err := v.setVPPInterfaceHWMtu(name, uint16(mtu)); err != nil {
			v.logger.Warn("Failed to set HW MTU", "interface", name, "mtu", mtu, "error", err)
		}

		ch, err := v.conn.NewAPIChannel()
		if err != nil {
			return fmt.Errorf("create API channel: %w", err)
		}
		defer ch.Close()

		req := &vppinterfaces.SwInterfaceSetMtu{
			SwIfIndex: interface_types.InterfaceIndex(idx),
			Mtu:       []uint32{uint32(mtu), 0, 0, 0},
		}
		reply := &vppinterfaces.SwInterfaceSetMtuReply{}
		if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
			return fmt.Errorf("set VPP SW MTU: %w", err)
		}
		if reply.Retval != 0 {
			return fmt.Errorf("set VPP SW MTU failed: retval=%d", reply.Retval)
		}

		if iface := v.ifMgr.GetByName(name); iface != nil {
			iface.MTU = uint32(mtu)
		}

		v.logger.Info("Set interface MTU", "interface", name, "mtu", mtu)
	}

	// Set MTU on LCP tap (dataplane ns) and host interface (default ns)
	link, h, linkErr := v.findLink(name)
	if linkErr != nil {
		if err != nil {
			return fmt.Errorf("interface %s not found in VPP or Linux: %w", name, linkErr)
		}
		return nil
	}
	if h != nil {
		if err := h.LinkSetMTU(link, mtu); err != nil {
			return fmt.Errorf("set LCP interface MTU: %w", err)
		}
		if hostLink, hostErr := netlink.LinkByName(name); hostErr == nil {
			if err := netlink.LinkSetMTU(hostLink, mtu); err != nil {
				v.logger.Warn("Failed to set host interface MTU", "interface", name, "mtu", mtu, "error", err)
			}
		}
		return nil
	}
	return netlink.LinkSetMTU(link, mtu)
}

func (v *VPP) SetInterfaceEnabled(name string, enabled bool) error {
	if _, err := v.GetInterfaceIndex(name); err != nil {
		return fmt.Errorf("VPP interface %s not found", name)
	}
	return v.setInterfaceState(name, enabled)
}

func (v *VPP) AddIPv4Address(ifName, address string) error {
	link, h, err := v.findLink(ifName)
	if err != nil {
		return fmt.Errorf("get interface %s: %w", ifName, err)
	}

	addr, err := netlink.ParseAddr(address)
	if err != nil {
		return fmt.Errorf("parse address %s: %w", address, err)
	}

	if err := v.addrAdd(h, link, addr); err != nil {
		if err.Error() == "file exists" {
			v.logger.Info("IPv4 address already exists", "interface", ifName, "address", address)
			return nil
		}
		return fmt.Errorf("add address: %w", err)
	}

	if swIfIndex, ok := v.ifMgr.GetSwIfIndex(ifName); ok {
		v.ifMgr.AddIPv4Address(swIfIndex, addr.IP)
	}

	v.logger.Info("Added IPv4 address", "interface", ifName, "address", address)
	return nil
}

func (v *VPP) DelIPv4Address(ifName, address string) error {
	link, h, err := v.findLink(ifName)
	if err != nil {
		return fmt.Errorf("get interface %s: %w", ifName, err)
	}

	addr, err := netlink.ParseAddr(address)
	if err != nil {
		return fmt.Errorf("parse address %s: %w", address, err)
	}

	if err := v.addrDel(h, link, addr); err != nil {
		return fmt.Errorf("del address: %w", err)
	}

	if swIfIndex, ok := v.ifMgr.GetSwIfIndex(ifName); ok {
		v.ifMgr.RemoveIPv4Address(swIfIndex, addr.IP)
	}

	v.logger.Info("Deleted IPv4 address", "interface", ifName, "address", address)
	return nil
}

func (v *VPP) AddIPv6Address(ifName, address string) error {
	link, h, err := v.findLink(ifName)
	if err != nil {
		return fmt.Errorf("get interface %s: %w", ifName, err)
	}

	addr, err := netlink.ParseAddr(address)
	if err != nil {
		return fmt.Errorf("parse address %s: %w", address, err)
	}

	if err := v.addrAdd(h, link, addr); err != nil {
		if err.Error() == "file exists" {
			v.logger.Info("IPv6 address already exists", "interface", ifName, "address", address)
			return nil
		}
		return fmt.Errorf("add address: %w", err)
	}

	if swIfIndex, ok := v.ifMgr.GetSwIfIndex(ifName); ok {
		v.ifMgr.AddIPv6Address(swIfIndex, addr.IP)
	}

	v.logger.Info("Added IPv6 address", "interface", ifName, "address", address)
	return nil
}

func (v *VPP) DelIPv6Address(ifName, address string) error {
	link, h, err := v.findLink(ifName)
	if err != nil {
		return fmt.Errorf("get interface %s: %w", ifName, err)
	}

	addr, err := netlink.ParseAddr(address)
	if err != nil {
		return fmt.Errorf("parse address %s: %w", address, err)
	}

	if err := v.addrDel(h, link, addr); err != nil {
		return fmt.Errorf("del address: %w", err)
	}

	if swIfIndex, ok := v.ifMgr.GetSwIfIndex(ifName); ok {
		v.ifMgr.RemoveIPv6Address(swIfIndex, addr.IP)
	}

	v.logger.Info("Deleted IPv6 address", "interface", ifName, "address", address)
	return nil
}

func (v *VPP) waitForLink(name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, _, err := v.findLink(name); err == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for LCP interface %q", name)
}

// Internal helpers

func (v *VPP) findLink(name string) (netlink.Link, *netlink.Handle, error) {
	if v.lcpNs != nil {
		if link, err := v.lcpNs.LinkByName(name); err == nil {
			return link, v.lcpNs, nil
		}
	}

	link, err := netlink.LinkByName(name)
	if err != nil {
		return nil, nil, fmt.Errorf("interface %q not found: %w", name, err)
	}
	return link, nil, nil
}

func (v *VPP) addrAdd(h *netlink.Handle, link netlink.Link, addr *netlink.Addr) error {
	if h != nil {
		return h.AddrAdd(link, addr)
	}
	return netlink.AddrAdd(link, addr)
}

func (v *VPP) addrDel(h *netlink.Handle, link netlink.Link, addr *netlink.Addr) error {
	if h != nil {
		return h.AddrDel(link, addr)
	}
	return netlink.AddrDel(link, addr)
}

func (v *VPP) renameVPPInterface(oldName, newName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(oldName)
	if err != nil {
		return fmt.Errorf("get interface index for %q: %w", oldName, err)
	}
	swIfIndex := interface_types.InterfaceIndex(idx)

	req := &vppinterfaces.SwInterfaceSetInterfaceName{
		SwIfIndex: swIfIndex,
		Name:      newName,
	}
	reply := &vppinterfaces.SwInterfaceSetInterfaceNameReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("rename interface: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("rename interface failed: retval=%d", reply.Retval)
	}

	v.ifMgr.Rename(oldName, newName)

	v.logger.Info("Renamed VPP interface", "old_name", oldName, "new_name", newName)
	return nil
}

func (v *VPP) createVPPHostInterface(linuxIface string) (string, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return "", fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	afReq := &af_packet.AfPacketCreateV2{
		HostIfName:      linuxIface,
		UseRandomHwAddr: false,
		Flags:           uint32(af_packet.AF_PACKET_API_FLAG_QDISC_BYPASS | af_packet.AF_PACKET_API_FLAG_CKSUM_GSO),
		NumRxQueues:     1,
	}

	var mac net.HardwareAddr
	if linuxIf, err := net.InterfaceByName(linuxIface); err == nil && len(linuxIf.HardwareAddr) >= 6 {
		mac = linuxIf.HardwareAddr
		copy(afReq.HwAddr[:], mac)
	}

	afReply := &af_packet.AfPacketCreateV2Reply{}
	if err := ch.SendRequest(afReq).ReceiveReply(afReply); err != nil {
		return "", fmt.Errorf("create host-interface: %w", err)
	}

	if afReply.Retval != 0 {
		return "", fmt.Errorf("create host-interface failed: retval=%d", afReply.Retval)
	}

	vppIfName := "host-" + linuxIface

	v.ifMgr.Add(&ifmgr.Interface{
		SwIfIndex:    uint32(afReply.SwIfIndex),
		SupSwIfIndex: uint32(afReply.SwIfIndex),
		Name:         vppIfName,
		DevType:      "af_packet",
		Type:         ifmgr.IfTypeHardware,
		MAC:          mac,
	})

	rxModeReq := &vppinterfaces.SwInterfaceSetRxMode{
		SwIfIndex: afReply.SwIfIndex,
		Mode:      v.resolveRxMode(),
	}
	rxModeReply := &vppinterfaces.SwInterfaceSetRxModeReply{}
	if err := ch.SendRequest(rxModeReq).ReceiveReply(rxModeReply); err != nil {
		v.logger.Warn("Failed to set RX mode", "interface", vppIfName, "mode", v.rxMode, "error", err)
	}

	v.logger.Info("Created VPP host-interface", "linux_iface", linuxIface, "vpp_iface", vppIfName, "sw_if_index", afReply.SwIfIndex)

	return vppIfName, nil
}

func (v *VPP) createVPPLoopback(name string) (string, uint32, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return "", 0, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	var instance uint32
	if _, err := fmt.Sscanf(name, "loop%d", &instance); err != nil {
		instance = 0
	}

	req := &vppinterfaces.CreateLoopbackInstance{
		MacAddress:   ethernet_types.MacAddress{},
		IsSpecified:  true,
		UserInstance: instance,
	}

	reply := &vppinterfaces.CreateLoopbackInstanceReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return "", 0, fmt.Errorf("create loopback: %w", err)
	}

	if reply.Retval != 0 {
		return "", 0, fmt.Errorf("create loopback failed: retval=%d", reply.Retval)
	}

	dumpReq := &vppinterfaces.SwInterfaceDump{
		SwIfIndex: reply.SwIfIndex,
	}

	stream := ch.SendMultiRequest(dumpReq)
	for {
		dumpReply := &vppinterfaces.SwInterfaceDetails{}
		stop, err := stream.ReceiveReply(dumpReply)
		if err != nil {
			return "", 0, fmt.Errorf("get interface name: %w", err)
		}
		if stop {
			break
		}

		if dumpReply.SwIfIndex == reply.SwIfIndex {
			v.logger.Info("Created VPP loopback", "config_name", name, "vpp_name", dumpReply.InterfaceName, "sw_if_index", reply.SwIfIndex)
			return dumpReply.InterfaceName, uint32(reply.SwIfIndex), nil
		}
	}

	v.logger.Warn("Could not determine VPP loopback name, using config name", "name", name, "sw_if_index", reply.SwIfIndex)
	return name, uint32(reply.SwIfIndex), nil
}

func (v *VPP) createLCPPair(vppIface, linuxIface string, hostType lcp.LcpItfHostType) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(vppIface)
	swIfIndex := interface_types.InterfaceIndex(idx)
	if err != nil {
		swIfIndex = interface_types.InterfaceIndex(^uint32(0))
		v.logger.Debug("VPP interface not found, will create from Linux interface",
			"vpp_iface", vppIface, "linux_iface", linuxIface)
	}

	req := &lcp.LcpItfPairAddDelV2{
		IsAdd:      true,
		SwIfIndex:  swIfIndex,
		HostIfName: linuxIface,
		HostIfType: hostType,
	}

	reply := &lcp.LcpItfPairAddDelV2Reply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		if reply.Retval == -81 {
			v.logger.Info("LCP pair already exists", "vpp_iface", vppIface, "linux_iface", linuxIface)
			return nil
		}
		return fmt.Errorf("create LCP pair: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("create LCP pair failed: retval=%d", reply.Retval)
	}

	if reply.HostSwIfIndex != 0 {
		rxReq := &vppinterfaces.SwInterfaceSetRxMode{
			SwIfIndex: reply.HostSwIfIndex,
			Mode:      interface_types.RX_MODE_API_INTERRUPT,
		}
		rxReply := &vppinterfaces.SwInterfaceSetRxModeReply{}
		if err := ch.SendRequest(rxReq).ReceiveReply(rxReply); err != nil {
			v.logger.Warn("Failed to set LCP tap RX mode to interrupt", "interface", linuxIface, "error", err)
		}
	}

	v.logger.Info("Created LCP pair", "vpp_iface", vppIface, "linux_iface", linuxIface, "host_type", hostType)
	return nil
}

func (v *VPP) setInterfaceState(name string, enabled bool) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(name)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	var flags interface_types.IfStatusFlags
	if enabled {
		flags = interface_types.IF_STATUS_API_FLAG_ADMIN_UP
	}

	req := &vppinterfaces.SwInterfaceSetFlags{
		SwIfIndex: interface_types.InterfaceIndex(idx),
		Flags:     flags,
	}

	reply := &vppinterfaces.SwInterfaceSetFlagsReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("set interface state: %w", err)
	}

	v.logger.Info("Set interface state", "interface", name, "enabled", enabled)
	return nil
}

func (v *VPP) bindInterfaceToVRF(vppIfName, linuxIfName, vrfName string, hasLCP bool) error {
	if v.vrfResolver == nil {
		return fmt.Errorf("VRF resolver not configured")
	}

	tableID, hasIPv4, hasIPv6, err := v.vrfResolver(vrfName)
	if err != nil {
		return fmt.Errorf("resolve VRF %q: %w", vrfName, err)
	}

	if err := v.setInterfaceTable(vppIfName, tableID, hasIPv4, hasIPv6); err != nil {
		return fmt.Errorf("set VPP table: %w", err)
	}

	if hasLCP {
		if err := v.setLinuxInterfaceVRF(linuxIfName, vrfName); err != nil {
			return fmt.Errorf("set Linux VRF: %w", err)
		}
	}

	v.logger.Info("Bound interface to VRF", "vpp_iface", vppIfName, "linux_iface", linuxIfName, "vrf", vrfName, "table_id", tableID)
	return nil
}

func (v *VPP) setInterfaceTable(name string, tableID uint32, ipv4, ipv6 bool) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(name)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}
	swIfIndex := interface_types.InterfaceIndex(idx)

	if ipv4 {
		req4 := &vppinterfaces.SwInterfaceSetTable{
			SwIfIndex: swIfIndex,
			IsIPv6:    false,
			VrfID:     tableID,
		}
		reply4 := &vppinterfaces.SwInterfaceSetTableReply{}
		if err := ch.SendRequest(req4).ReceiveReply(reply4); err != nil {
			return fmt.Errorf("set IPv4 table: %w", err)
		}
		if reply4.Retval != 0 {
			return fmt.Errorf("set IPv4 table failed: retval=%d", reply4.Retval)
		}
	}

	if ipv6 {
		req6 := &vppinterfaces.SwInterfaceSetTable{
			SwIfIndex: swIfIndex,
			IsIPv6:    true,
			VrfID:     tableID,
		}
		reply6 := &vppinterfaces.SwInterfaceSetTableReply{}
		if err := ch.SendRequest(req6).ReceiveReply(reply6); err != nil {
			return fmt.Errorf("set IPv6 table: %w", err)
		}
		if reply6.Retval != 0 {
			return fmt.Errorf("set IPv6 table failed: retval=%d", reply6.Retval)
		}
	}

	v.ifMgr.SetFIBTableID(uint32(swIfIndex), tableID)

	return nil
}

func (v *VPP) setLinuxInterfaceVRF(ifName, vrfName string) error {
	vrfLink, vrfH, err := v.findLink(vrfName)
	if err != nil {
		return fmt.Errorf("VRF device %q not found: %w", vrfName, err)
	}

	tapLink, tapH, err := v.findLink(ifName)
	if err != nil {
		return fmt.Errorf("interface %q not found: %w", ifName, err)
	}

	// Use the handle from whichever link was found in a namespace (prefer tap's handle)
	h := tapH
	if h == nil {
		h = vrfH
	}

	if h != nil {
		return h.LinkSetMaster(tapLink, vrfLink)
	}
	return netlink.LinkSetMaster(tapLink, vrfLink)
}

func (v *VPP) getLinuxInterfaceMTU(name string) (int, error) {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return 0, fmt.Errorf("interface %q not found: %w", name, err)
	}
	return link.Attrs().MTU, nil
}

func (v *VPP) setVPPInterfaceHWMtu(name string, mtu uint16) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(name)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	req := &vppinterfaces.HwInterfaceSetMtu{
		SwIfIndex: interface_types.InterfaceIndex(idx),
		Mtu:       mtu,
	}
	reply := &vppinterfaces.HwInterfaceSetMtuReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("set HW MTU: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("set HW MTU failed: retval=%d", reply.Retval)
	}

	return nil
}

func inferInterfaceType(cfg *interfaces.InterfaceConfig) string {
	if cfg.Type != "" {
		return cfg.Type
	}

	if cfg.Bond != nil {
		return "bond"
	}

	if len(cfg.Name) >= 4 && cfg.Name[:4] == "loop" {
		return "loopback"
	}

	return "physical"
}
