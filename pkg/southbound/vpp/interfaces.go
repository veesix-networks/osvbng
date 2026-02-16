package vpp

import (
	"fmt"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ethernet_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip_types"
	"github.com/vishvananda/netlink"
	"go.fd.io/govpp/api"
	"net"
	"strings"
	interfaces "github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface"
)

func (v *VPP) CreateSVLAN(parentIface string, vlan uint16, ipv4 []string, ipv6 []string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	subIfName := fmt.Sprintf("%s.%d", parentIface, vlan)

	if iface := v.ifMgr.GetByName(subIfName); iface != nil {
		v.logger.Debug("S-VLAN sub-interface already exists", "interface", subIfName, "sw_if_index", iface.SwIfIndex)
		return nil
	}

	parentIdx, err := v.GetInterfaceIndex(parentIface)
	if err != nil {
		return fmt.Errorf("parent interface %s not found: %w", parentIface, err)
	}

	req := &interfaces.CreateSubif{
		SwIfIndex:   interface_types.InterfaceIndex(parentIdx),
		SubID:       uint32(vlan),
		SubIfFlags:  interface_types.SUB_IF_API_FLAG_ONE_TAG | interface_types.SUB_IF_API_FLAG_TWO_TAGS | interface_types.SUB_IF_API_FLAG_INNER_VLAN_ID_ANY,
		OuterVlanID: vlan,
	}

	reply := &interfaces.CreateSubifReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "-56") {
			v.logger.Debug("S-VLAN sub-interface already exists in VPP", "interface", subIfName)
			if err := v.LoadInterfaces(); err != nil {
				return fmt.Errorf("reload interfaces: %w", err)
			}
			return nil
		}
		return fmt.Errorf("create vlan sub-interface: %w", err)
	}

	setUpReq := &interfaces.SwInterfaceSetFlags{
		SwIfIndex: reply.SwIfIndex,
		Flags:     interface_types.IF_STATUS_API_FLAG_ADMIN_UP,
	}
	setUpReply := &interfaces.SwInterfaceSetFlagsReply{}
	if err := ch.SendRequest(setUpReq).ReceiveReply(setUpReply); err != nil {
		return fmt.Errorf("set interface up: %w", err)
	}

	for _, cidr := range ipv4 {
		if err := v.addIPAddress(ch, reply.SwIfIndex, cidr, false); err != nil {
			return fmt.Errorf("add ipv4 address %s: %w", cidr, err)
		}
	}

	for _, cidr := range ipv6 {
		if err := v.addIPAddress(ch, reply.SwIfIndex, cidr, true); err != nil {
			v.logger.Warn("Failed to add IPv6 address", "address", cidr, "error", err)
		}
	}

	var parentMAC []byte
	if parent := v.ifMgr.Get(uint32(parentIdx)); parent != nil {
		parentMAC = parent.MAC
	}

	v.ifMgr.Add(&ifmgr.Interface{
		SwIfIndex:    uint32(reply.SwIfIndex),
		SupSwIfIndex: uint32(parentIdx),
		Name:         subIfName,
		Type:         ifmgr.IfTypeSub,
		AdminUp:      true,
		OuterVlanID:  vlan,
		MAC:          parentMAC,
	})

	v.logger.Debug("Created S-VLAN sub-interface", "interface", subIfName, "sw_if_index", reply.SwIfIndex)
	return nil
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

	req := &interfaces.DeleteSubif{
		SwIfIndex: interface_types.InterfaceIndex(iface.SwIfIndex),
	}

	reply := &interfaces.DeleteSubifReply{}
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

	req := &interfaces.SwInterfaceDump{
		NameFilterValid: true,
		NameFilter:      name,
	}

	v.logger.Debug("Looking up interface index", "name", name)
	stream := ch.SendMultiRequest(req)
	for {
		reply := &interfaces.SwInterfaceDetails{}
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
			mac := reply.L2Address[:]
			if strings.HasPrefix(ifaceName, "host-") && isZeroMAC(mac) {
				linuxIfName := strings.TrimPrefix(ifaceName, "host-")
				if linuxIf, err := net.InterfaceByName(linuxIfName); err == nil {
					mac = linuxIf.HardwareAddr
				}
			}
			v.ifMgr.Add(&ifmgr.Interface{
				SwIfIndex:       uint32(reply.SwIfIndex),
				SupSwIfIndex:    reply.SupSwIfIndex,
				Name:            ifaceName,
				DevType:         strings.TrimRight(reply.InterfaceDevType, "\x00"),
				Type:            ifmgr.IfType(reply.Type),
				AdminUp:         reply.Flags&interface_types.IF_STATUS_API_FLAG_ADMIN_UP != 0,
				LinkUp:          reply.Flags&interface_types.IF_STATUS_API_FLAG_LINK_UP != 0,
				MTU:             reply.Mtu[0],
				MAC:             mac,
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
	// For af-packet interfaces, VPP may report a zero MAC. Set the Linux
	// interface MAC on the VPP side so ethernet-input accepts unicast traffic,
	// and enable Linux promiscuous mode for future SRG/VRRP virtual MACs.

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

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		v.logger.Warn("Could not find VPP interface for MAC sync", "interface", ifaceName, "error", err)
		return nil
	}

	linuxIf, err := net.InterfaceByName(ifaceName)
	if err != nil {
		v.logger.Warn("Could not get Linux interface MAC", "interface", ifaceName, "error", err)
		return nil
	}

	if len(linuxIf.HardwareAddr) >= 6 {
		if err := v.setInterfaceMAC(interface_types.InterfaceIndex(idx), linuxIf.HardwareAddr); err != nil {
			v.logger.Warn("Failed to set VPP interface MAC from Linux", "interface", ifaceName, "error", err)
		} else {
			v.logger.Debug("Synced VPP interface MAC from Linux", "interface", ifaceName, "mac", linuxIf.HardwareAddr)
		}
	}

	v.logger.Debug("Set interface promiscuous", "interface", ifaceName, "on", on)
	return nil
}


func (v *VPP) setInterfaceMAC(swIfIndex interface_types.InterfaceIndex, mac net.HardwareAddr) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	var macAddr ethernet_types.MacAddress
	copy(macAddr[:], mac)

	req := &interfaces.SwInterfaceSetMacAddress{
		SwIfIndex:  swIfIndex,
		MacAddress: macAddr,
	}

	reply := &interfaces.SwInterfaceSetMacAddressReply{}
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

	req := &interfaces.SwInterfaceSetUnnumbered{
		SwIfIndex:           interface_types.InterfaceIndex(loopback.SwIfIndex),
		UnnumberedSwIfIndex: interface_types.InterfaceIndex(iface.SwIfIndex),
		IsAdd:               true,
	}

	reply := &interfaces.SwInterfaceSetUnnumberedReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("set unnumbered: %w", err)
	}

	v.logger.Debug("Set interface unnumbered", "interface", ifaceName, "loopback", loopbackName)
	return nil
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


func (v *VPP) addIPAddress(ch api.Channel, swIfIndex interface_types.InterfaceIndex, cidr string, isIPv6 bool) error {
	addr, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("parse CIDR: %w", err)
	}

	prefixLen, _ := ipNet.Mask.Size()

	var ipPrefix ip_types.AddressWithPrefix
	if !isIPv6 {
		ipPrefix = ip_types.AddressWithPrefix{
			Address: ip_types.Address{
				Af: ip_types.ADDRESS_IP4,
				Un: ip_types.AddressUnionIP4(ip_types.IP4Address{
					addr.To4()[0], addr.To4()[1], addr.To4()[2], addr.To4()[3],
				}),
			},
			Len: uint8(prefixLen),
		}
	} else {
		var ip6 ip_types.IP6Address
		copy(ip6[:], addr.To16())
		ipPrefix = ip_types.AddressWithPrefix{
			Address: ip_types.Address{
				Af: ip_types.ADDRESS_IP6,
				Un: ip_types.AddressUnionIP6(ip6),
			},
			Len: uint8(prefixLen),
		}
	}

	req := &interfaces.SwInterfaceAddDelAddress{
		SwIfIndex: swIfIndex,
		IsAdd:     true,
		Prefix:    ipPrefix,
	}

	reply := &interfaces.SwInterfaceAddDelAddressReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return err
	}

	if isIPv6 {
		v.ifMgr.AddIPv6Address(uint32(swIfIndex), addr)
	} else {
		v.ifMgr.AddIPv4Address(uint32(swIfIndex), addr)
	}

	return nil
}


func (v *VPP) DumpInterfaces() ([]southbound.InterfaceInfo, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, err
	}
	defer ch.Close()

	req := &interfaces.SwInterfaceDump{
		SwIfIndex: interface_types.InterfaceIndex(^uint32(0)),
	}

	stream := ch.SendMultiRequest(req)
	var result []southbound.InterfaceInfo

	for {
		reply := &interfaces.SwInterfaceDetails{}
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

	req := &interfaces.SwInterfaceDump{
		SwIfIndex: interface_types.InterfaceIndex(^uint32(0)),
	}

	stream := ch.SendMultiRequest(req)
	v.ifMgr.Clear()

	for {
		reply := &interfaces.SwInterfaceDetails{}
		stop, err := stream.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return fmt.Errorf("load interfaces: %w", err)
		}

		ifaceName := strings.TrimRight(reply.InterfaceName, "\x00")
		mac := reply.L2Address[:]

		if strings.HasPrefix(ifaceName, "host-") && isZeroMAC(mac) {
			linuxIfName := strings.TrimPrefix(ifaceName, "host-")
			if linuxIf, err := net.InterfaceByName(linuxIfName); err == nil {
				mac = linuxIf.HardwareAddr
			}
		}

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
			MAC:             mac,
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


