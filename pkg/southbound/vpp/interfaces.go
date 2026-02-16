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
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ethernet_types"
	vppinterfaces "github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/lcp"
	"github.com/vishvananda/netlink"
	"go.fd.io/govpp/api"
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

	req := &vppinterfaces.CreateSubif{
		SwIfIndex:   interface_types.InterfaceIndex(parentIdx),
		SubID:       uint32(vlan),
		SubIfFlags:  interface_types.SUB_IF_API_FLAG_ONE_TAG | interface_types.SUB_IF_API_FLAG_TWO_TAGS | interface_types.SUB_IF_API_FLAG_INNER_VLAN_ID_ANY,
		OuterVlanID: vlan,
	}

	reply := &vppinterfaces.CreateSubifReply{}
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

	setUpReq := &vppinterfaces.SwInterfaceSetFlags{
		SwIfIndex: reply.SwIfIndex,
		Flags:     interface_types.IF_STATUS_API_FLAG_ADMIN_UP,
	}
	setUpReply := &vppinterfaces.SwInterfaceSetFlagsReply{}
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

	req := &vppinterfaces.SwInterfaceAddDelAddress{
		SwIfIndex: swIfIndex,
		IsAdd:     true,
		Prefix:    ipPrefix,
	}

	reply := &vppinterfaces.SwInterfaceAddDelAddressReply{}
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

func (v *VPP) CreateInterface(cfg *interfaces.InterfaceConfig) error {
	ifType := inferInterfaceType(cfg)

	if ifType == "loopback" {
		return v.createLoopback(cfg)
	} else if ifType == "physical" {
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
			time.Sleep(100 * time.Millisecond)
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
		time.Sleep(100 * time.Millisecond)
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

		time.Sleep(100 * time.Millisecond)
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
	link, h, err := v.findLink(name)
	if err != nil {
		return fmt.Errorf("LCP interface %s not found: %w", name, err)
	}

	if h != nil {
		return h.LinkSetAlias(link, description)
	}
	return netlink.LinkSetAlias(link, description)
}

func (v *VPP) SetInterfaceMTU(name string, mtu int) error {
	link, h, err := v.findLink(name)
	if err != nil {
		return fmt.Errorf("interface %s not found: %w", name, err)
	}

	if h != nil {
		return h.LinkSetMTU(link, mtu)
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
	})

	rxModeReq := &vppinterfaces.SwInterfaceSetRxMode{
		SwIfIndex: afReply.SwIfIndex,
		Mode:      interface_types.RX_MODE_API_POLLING,
	}
	rxModeReply := &vppinterfaces.SwInterfaceSetRxModeReply{}
	if err := ch.SendRequest(rxModeReq).ReceiveReply(rxModeReply); err != nil {
		v.logger.Warn("Failed to set RX mode to polling", "interface", vppIfName, "error", err)
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

	req := &lcp.LcpItfPairAddDel{
		IsAdd:      true,
		SwIfIndex:  swIfIndex,
		HostIfName: linuxIface,
		HostIfType: hostType,
	}

	reply := &lcp.LcpItfPairAddDelReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("create LCP pair: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("create LCP pair failed: retval=%d", reply.Retval)
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

	if len(cfg.Name) >= 4 && cfg.Name[:4] == "loop" {
		return "loopback"
	}

	return "physical"
}
