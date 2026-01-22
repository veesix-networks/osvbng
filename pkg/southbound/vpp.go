package southbound

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models/subscribers"
	"github.com/veesix-networks/osvbng/pkg/models/system"
	"github.com/vishvananda/netlink"
	"go.fd.io/govpp/api"
	"go.fd.io/govpp/core"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/arp_punt"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ethernet_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/fib_control"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/fib_types"
	interfaces "github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip_neighbor"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/l2"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/lcp"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/memif"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/mpls"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/openbng_accounting"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/punt"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/tapv2"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/vlib"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/vpe"
)

type VPP struct {
	conn         *core.Connection
	parentIface  string
	ifaceCache   map[string]interface_types.InterfaceIndex
	parentIfIdx  interface_types.InterfaceIndex
	parentIfMAC  net.HardwareAddr
	virtualMAC   net.HardwareAddr
	logger       *slog.Logger
	fibChan      api.Channel
	fibMux       sync.Mutex
	useDPDK      bool
}

type VPPConfig struct {
	Connection      *core.Connection
	ParentInterface string
	UseDPDK         bool
}

func NewVPP(cfg VPPConfig) (*VPP, error) {
	if cfg.Connection == nil {
		return nil, fmt.Errorf("VPP connection is required")
	}

	conn := cfg.Connection

	fibChan, err := conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create FIB API channel: %w", err)
	}

	v := &VPP{
		conn:        conn,
		parentIface: cfg.ParentInterface,
		ifaceCache:  make(map[string]interface_types.InterfaceIndex),
		logger:      logger.Component(logger.ComponentSouthbound),
		fibChan:     fibChan,
		useDPDK:     cfg.UseDPDK,
	}

	if err := v.resolveParentInterface(); err != nil {
		fibChan.Close()
		return nil, fmt.Errorf("resolve parent interface: %w", err)
	}

	v.logger.Info("Connected to VPP", "parent_interface", v.parentIface, "sw_if_index", v.parentIfIdx)

	return v, nil
}

func (v *VPP) CreateTAP(tapName string) (string, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return "", err
	}
	defer ch.Close()

	req := &tapv2.TapCreateV3{
		ID:            ^uint32(0),
		HostIfName:    tapName,
		HostIfNameSet: true,
		NumRxQueues:   1,
		TxRingSz:      1024,
		RxRingSz:      1024,
		HostMtuSet:    true,
		HostMtuSize:   1500,
		TapFlags:      0,
	}

	reply := &tapv2.TapCreateV3Reply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return "", fmt.Errorf("tap_create_v3: %w", err)
	}

	if reply.Retval != 0 {
		return "", fmt.Errorf("tap create failed with retval: %d", reply.Retval)
	}

	v.ifaceCache[tapName] = reply.SwIfIndex

	setUpReq := &interfaces.SwInterfaceSetFlags{
		SwIfIndex: reply.SwIfIndex,
		Flags:     interface_types.IF_STATUS_API_FLAG_ADMIN_UP,
	}

	setUpReply := &interfaces.SwInterfaceSetFlagsReply{}
	if err := ch.SendRequest(setUpReq).ReceiveReply(setUpReply); err != nil {
		return "", fmt.Errorf("set interface up: %w", err)
	}

	v.logger.Info("Created TAP interface", "interface", tapName, "sw_if_index", reply.SwIfIndex)

	return tapName, nil
}

func (v *VPP) SetL2CrossConnect(iface1, iface2 string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	idx1, ok := v.ifaceCache[iface1]
	if !ok {
		return fmt.Errorf("interface %s not found", iface1)
	}

	idx2, ok := v.ifaceCache[iface2]
	if !ok {
		return fmt.Errorf("interface %s not found", iface2)
	}

	req1 := &l2.SwInterfaceSetL2Xconnect{
		RxSwIfIndex: idx1,
		TxSwIfIndex: idx2,
		Enable:      true,
	}

	reply1 := &l2.SwInterfaceSetL2XconnectReply{}
	if err := ch.SendRequest(req1).ReceiveReply(reply1); err != nil {
		return fmt.Errorf("set xconnect %s->%s: %w", iface1, iface2, err)
	}

	v.logger.Info("Set L2 cross-connect (one-way)", "from", iface1, "to", iface2)

	return nil
}

func (v *VPP) Close() error {
	if v.fibChan != nil {
		v.fibChan.Close()
	}
	v.conn.Disconnect()
	return nil
}

func (v *VPP) GetVersion(ctx context.Context) (string, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return "", err
	}
	defer ch.Close()

	reply := &vpe.ShowVersionReply{}
	if err := ch.SendRequest(&vpe.ShowVersion{}).ReceiveReply(reply); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s %s", reply.Program, reply.Version), nil
}

func (v *VPP) resolveParentInterface() error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	req := &interfaces.SwInterfaceDump{}

	reqCtx := ch.SendMultiRequest(req)
	for {
		msg := &interfaces.SwInterfaceDetails{}
		stop, err := reqCtx.ReceiveReply(msg)
		if err != nil {
			return fmt.Errorf("receive interface details: %w", err)
		}
		if stop {
			break
		}

		v.logger.Info("Scanning VPP interface during parent resolution", "interface_name", msg.InterfaceName, "sw_if_index", msg.SwIfIndex, "mac", net.HardwareAddr(msg.L2Address[:6]).String())

		// Match exact name or with "host-" prefix (linux_nl plugin adds this automatically for AF_PACKET and XDP interfaces, DPDK we don't care)
		if msg.InterfaceName == v.parentIface || msg.InterfaceName == "host-"+v.parentIface {
			v.logger.Info("Resolved parent interface", "requested_name", v.parentIface, "matched_name", msg.InterfaceName, "sw_if_index", msg.SwIfIndex, "mac", net.HardwareAddr(msg.L2Address[:6]).String())
			v.parentIfIdx = msg.SwIfIndex
			v.ifaceCache[v.parentIface] = msg.SwIfIndex

			// If not using DPDK and VPP interface has null MAC, set it from Linux interface
			vppMAC := net.HardwareAddr(msg.L2Address[:6])
			if !v.useDPDK && vppMAC.String() == "00:00:00:00:00:00" {
				v.logger.Info("AF_PACKET interface has null MAC, setting from Linux interface", "interface", v.parentIface)
				if linuxIface, err := net.InterfaceByName(v.parentIface); err == nil {
					if err := v.SetInterfaceMAC(msg.SwIfIndex, linuxIface.HardwareAddr); err == nil {
						v.logger.Info("Set VPP interface MAC from Linux", "interface", v.parentIface, "mac", linuxIface.HardwareAddr.String())
						v.parentIfMAC = linuxIface.HardwareAddr
					} else {
						v.logger.Warn("Failed to set VPP interface MAC", "error", err, "interface", v.parentIface)
					}
				} else {
					v.logger.Warn("Failed to get Linux interface for MAC", "error", err, "interface", v.parentIface)
				}
			} else {
				// Cache the existing MAC
				v.parentIfMAC = vppMAC
			}

			return nil
		}
	}

	return fmt.Errorf("parent interface %s not found in VPP", v.parentIface)
}

func (v *VPP) CreateSVLAN(vlan uint16, ipv4 []string, ipv6 []string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	subIfName := fmt.Sprintf("%s.%d", v.parentIface, vlan)

	if idx, exists := v.ifaceCache[subIfName]; exists {
		v.logger.Info("S-VLAN sub-interface already exists", "interface", subIfName, "sw_if_index", idx)
		return nil
	}

	req := &interfaces.CreateSubif{
		SwIfIndex:   v.parentIfIdx,
		SubID:       uint32(vlan),
		SubIfFlags:  interface_types.SUB_IF_API_FLAG_ONE_TAG | interface_types.SUB_IF_API_FLAG_TWO_TAGS | interface_types.SUB_IF_API_FLAG_INNER_VLAN_ID_ANY,
		OuterVlanID: vlan,
	}

	reply := &interfaces.CreateSubifReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "-56") {
			v.logger.Info("S-VLAN sub-interface already exists in VPP", "interface", subIfName)
			idx, err := v.GetInterfaceIndex(subIfName)
			if err != nil {
				return fmt.Errorf("get existing subinterface index: %w", err)
			}
			v.ifaceCache[subIfName] = interface_types.InterfaceIndex(idx)
			return nil
		}
		return fmt.Errorf("create vlan sub-interface: %w", err)
	}

	v.ifaceCache[subIfName] = reply.SwIfIndex

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

	v.logger.Info("Created S-VLAN sub-interface", "interface", subIfName, "sw_if_index", reply.SwIfIndex)
	return nil
}

func (v *VPP) DeleteInterface(name string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	idx, ok := v.ifaceCache[name]
	if !ok {
		return nil
	}

	req := &interfaces.DeleteSubif{
		SwIfIndex: idx,
	}

	reply := &interfaces.DeleteSubifReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("delete interface: %w", err)
	}

	delete(v.ifaceCache, name)
	v.logger.Info("Deleted interface", "interface", name)
	return nil
}

func (v *VPP) AddNeighbor(ipAddr, macAddr, iface string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	idx, ok := v.ifaceCache[iface]
	if !ok {
		return fmt.Errorf("interface %s not found", iface)
	}

	parsedIP := net.ParseIP(ipAddr)
	if parsedIP == nil {
		return fmt.Errorf("invalid IP address: %s", ipAddr)
	}

	parsedMAC, err := net.ParseMAC(macAddr)
	if err != nil {
		return fmt.Errorf("invalid MAC address: %s", macAddr)
	}

	var ipAddress ip_types.Address
	if parsedIP.To4() != nil {
		ipAddress = ip_types.Address{
			Af: ip_types.ADDRESS_IP4,
			Un: ip_types.AddressUnionIP4(ip_types.IP4Address{
				parsedIP.To4()[0], parsedIP.To4()[1], parsedIP.To4()[2], parsedIP.To4()[3],
			}),
		}
	} else {
		var ip6 ip_types.IP6Address
		copy(ip6[:], parsedIP.To16())
		ipAddress = ip_types.Address{
			Af: ip_types.ADDRESS_IP6,
			Un: ip_types.AddressUnionIP6(ip6),
		}
	}

	var macAddress ethernet_types.MacAddress
	copy(macAddress[:], parsedMAC)

	req := &ip_neighbor.IPNeighborAddDel{
		IsAdd: true,
		Neighbor: ip_neighbor.IPNeighbor{
			SwIfIndex:  idx,
			Flags:      ip_neighbor.IP_API_NEIGHBOR_FLAG_STATIC,
			MacAddress: macAddress,
			IPAddress:  ipAddress,
		},
	}

	reply := &ip_neighbor.IPNeighborAddDelReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("add neighbor: %w", err)
	}

	v.logger.Info("Added neighbor", "ip", ipAddr, "mac", macAddr, "interface", iface)
	return nil
}

func (v *VPP) DeleteNeighbor(ipAddr, iface string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	idx, ok := v.ifaceCache[iface]
	if !ok {
		return fmt.Errorf("interface %s not found", iface)
	}

	parsedIP := net.ParseIP(ipAddr)
	if parsedIP == nil {
		return fmt.Errorf("invalid IP address: %s", ipAddr)
	}

	var ipAddress ip_types.Address
	if parsedIP.To4() != nil {
		ipAddress = ip_types.Address{
			Af: ip_types.ADDRESS_IP4,
			Un: ip_types.AddressUnionIP4(ip_types.IP4Address{
				parsedIP.To4()[0], parsedIP.To4()[1], parsedIP.To4()[2], parsedIP.To4()[3],
			}),
		}
	} else {
		var ip6 ip_types.IP6Address
		copy(ip6[:], parsedIP.To16())
		ipAddress = ip_types.Address{
			Af: ip_types.ADDRESS_IP6,
			Un: ip_types.AddressUnionIP6(ip6),
		}
	}

	req := &ip_neighbor.IPNeighborAddDel{
		IsAdd: false,
		Neighbor: ip_neighbor.IPNeighbor{
			SwIfIndex: idx,
			IPAddress: ipAddress,
		},
	}

	reply := &ip_neighbor.IPNeighborAddDelReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("delete neighbor: %w", err)
	}

	v.logger.Info("Deleted neighbor", "ip", ipAddr, "interface", iface)
	return nil
}

func (v *VPP) ApplyQoS(iface string, upMbps, downMbps int) error {
	v.logger.Info("QoS not yet implemented", "interface", iface, "up_mbps", upMbps, "down_mbps", downMbps)
	return nil
}

func (v *VPP) RemoveQoS(iface string) error {
	v.logger.Info("QoS removal not yet implemented", "interface", iface)
	return nil
}

func (v *VPP) AddRoute(prefix, nexthop, vrf string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	_, ipNet, err := net.ParseCIDR(prefix)
	if err != nil {
		return fmt.Errorf("invalid prefix: %w", err)
	}

	parsedNexthop := net.ParseIP(nexthop)
	if parsedNexthop == nil {
		return fmt.Errorf("invalid nexthop: %s", nexthop)
	}

	prefixLen, _ := ipNet.Mask.Size()

	var ipPrefix ip_types.Prefix
	var nhAddress ip_types.Address

	if parsedNexthop.To4() != nil {
		ipPrefix = ip_types.Prefix{
			Address: ip_types.Address{
				Af: ip_types.ADDRESS_IP4,
				Un: ip_types.AddressUnionIP4(ip_types.IP4Address{
					ipNet.IP.To4()[0], ipNet.IP.To4()[1], ipNet.IP.To4()[2], ipNet.IP.To4()[3],
				}),
			},
			Len: uint8(prefixLen),
		}

		nhAddress = ip_types.Address{
			Af: ip_types.ADDRESS_IP4,
			Un: ip_types.AddressUnionIP4(ip_types.IP4Address{
				parsedNexthop.To4()[0], parsedNexthop.To4()[1],
				parsedNexthop.To4()[2], parsedNexthop.To4()[3],
			}),
		}
	} else {
		var ip6 ip_types.IP6Address
		copy(ip6[:], ipNet.IP.To16())
		ipPrefix = ip_types.Prefix{
			Address: ip_types.Address{
				Af: ip_types.ADDRESS_IP6,
				Un: ip_types.AddressUnionIP6(ip6),
			},
			Len: uint8(prefixLen),
		}

		var nhIP6 ip_types.IP6Address
		copy(nhIP6[:], parsedNexthop.To16())
		nhAddress = ip_types.Address{
			Af: ip_types.ADDRESS_IP6,
			Un: ip_types.AddressUnionIP6(nhIP6),
		}
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
					Nh: fib_types.FibPathNh{
						Address: nhAddress.Un,
					},
				},
			},
		},
	}

	reply := &ip.IPRouteAddDelReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("add route: %w", err)
	}

	v.logger.Info("Added route", "prefix", prefix, "nexthop", nexthop)
	return nil
}

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

	v.logger.Info("Added local route", "prefix", prefix)
	return nil
}

func (v *VPP) DeleteRoute(prefix, vrf string) error {
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

	var ipPrefix ip_types.Prefix
	if ipNet.IP.To4() != nil {
		ipPrefix = ip_types.Prefix{
			Address: ip_types.Address{
				Af: ip_types.ADDRESS_IP4,
				Un: ip_types.AddressUnionIP4(ip_types.IP4Address{
					ipNet.IP.To4()[0], ipNet.IP.To4()[1], ipNet.IP.To4()[2], ipNet.IP.To4()[3],
				}),
			},
			Len: uint8(prefixLen),
		}
	} else {
		var ip6 ip_types.IP6Address
		copy(ip6[:], ipNet.IP.To16())
		ipPrefix = ip_types.Prefix{
			Address: ip_types.Address{
				Af: ip_types.ADDRESS_IP6,
				Un: ip_types.AddressUnionIP6(ip6),
			},
			Len: uint8(prefixLen),
		}
	}

	req := &ip.IPRouteAddDel{
		IsAdd: false,
		Route: ip.IPRoute{
			TableID: 0,
			Prefix:  ipPrefix,
		},
	}

	reply := &ip.IPRouteAddDelReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("delete route: %w", err)
	}

	v.logger.Info("Deleted route", "prefix", prefix)
	return nil
}

func (v *VPP) GetInterfaceIndex(name string) (int, error) {
	idx, ok := v.ifaceCache[name]
	if !ok {
		ch, err := v.conn.NewAPIChannel()
		if err != nil {
			return 0, err
		}
		defer ch.Close()

		req := &interfaces.SwInterfaceDump{
			NameFilterValid: true,
			NameFilter:      name,
		}

		v.logger.Info("Looking up interface index", "name", name)
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

			v.logger.Info("Checking interface from VPP", "requested_name", name, "vpp_interface_name", reply.InterfaceName, "sw_if_index", reply.SwIfIndex)
			// Match exact name or with "host-" prefix (linux_nl plugin adds this automatically for AF_PACKET and XDP interfaces)
			if reply.InterfaceName == name || reply.InterfaceName == "host-"+name {
				v.logger.Info("Found matching interface", "requested_name", name, "matched_name", reply.InterfaceName, "sw_if_index", reply.SwIfIndex)
				v.ifaceCache[name] = reply.SwIfIndex
				return int(reply.SwIfIndex), nil
			}
		}

		return 0, fmt.Errorf("interface %s not found", name)
	}
	v.logger.Info("Using cached interface index", "name", name, "sw_if_index", idx)
	return int(idx), nil
}

func (v *VPP) GetParentInterface() string {
	return v.parentIface
}

func (v *VPP) GetParentInterfaceMAC() net.HardwareAddr {
	return v.parentIfMAC
}

func (v *VPP) GetParentSwIfIndex() (uint32, error) {
	v.logger.Info("Getting parent interface index", "parent_interface_name", v.parentIface)
	idx, err := v.GetInterfaceIndex(v.parentIface)
	if err != nil {
		return 0, err
	}
	v.logger.Info("Got parent interface index", "parent_interface_name", v.parentIface, "sw_if_index", idx)
	return uint32(idx), nil
}

func (v *VPP) GetInterfaceMAC(swIfIndex uint32) (net.HardwareAddr, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &interfaces.SwInterfaceDump{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
	}

	stream := ch.SendMultiRequest(req)
	reply := &interfaces.SwInterfaceDetails{}
	stop, err := stream.ReceiveReply(reply)
	if err != nil {
		return nil, fmt.Errorf("dump interface: %w", err)
	}
	if stop {
		return nil, fmt.Errorf("interface %d not found", swIfIndex)
	}

	mac := net.HardwareAddr(reply.L2Address[:6])
	v.logger.Info("Got interface MAC from VPP", "sw_if_index", swIfIndex, "interface_name", reply.InterfaceName, "mac", mac.String())
	return mac, nil
}

func (v *VPP) SetInterfaceMAC(swIfIndex interface_types.InterfaceIndex, mac net.HardwareAddr) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
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
		return fmt.Errorf("set interface MAC: %w", err)
	}

	return nil
}

func (v *VPP) GetLCPHostInterface(vppIface string) (string, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return "", err
	}
	defer ch.Close()

	swIfIndex, ok := v.ifaceCache[vppIface]
	if !ok {
		return "", fmt.Errorf("interface %s not found", vppIface)
	}

	req := &lcp.LcpItfPairGet{
		Cursor: ^uint32(0),
	}

	stream := ch.SendMultiRequest(req)
	var hostIfName string
	found := false

	for {
		reply := &lcp.LcpItfPairDetails{}
		stop, err := stream.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			if strings.Contains(err.Error(), "lcp_itf_pair_get_reply") {
				break
			}
			return "", fmt.Errorf("get lcp pairs: %w", err)
		}

		if reply.PhySwIfIndex == swIfIndex && !found {
			hostIfName = reply.HostIfName
			found = true
		}
	}

	if !found {
		return "", fmt.Errorf("no LCP pair found for %s", vppIface)
	}

	v.logger.Info("Found LCP pair", "tap", hostIfName, "phy", vppIface)
	return hostIfName, nil
}

func (v *VPP) SetVirtualMAC(mac string) error {
	parsedMAC, err := net.ParseMAC(mac)
	if err != nil {
		return fmt.Errorf("invalid virtual MAC: %w", err)
	}
	v.virtualMAC = parsedMAC

	if v.parentIfIdx != 0 {
		return v.setInterfaceMAC(v.parentIfIdx, parsedMAC)
	}
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

	v.logger.Info("Set interface MAC", "sw_if_index", swIfIndex, "mac", mac.String())
	return nil
}

func (v *VPP) CreateLoopback(name string, ipv4 []string, ipv6 []string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	if idx, exists := v.ifaceCache[name]; exists {
		v.logger.Info("Loopback already exists in cache, reconciling IPs", "interface", name, "sw_if_index", idx)
		return v.reconcileLoopbackIPs(name, ipv4, ipv6)
	}

	dumpReq := &interfaces.SwInterfaceDump{
		NameFilterValid: true,
		NameFilter:      name,
	}
	reqCtx := ch.SendMultiRequest(dumpReq)
	for {
		msg := &interfaces.SwInterfaceDetails{}
		stop, err := reqCtx.ReceiveReply(msg)
		if err != nil {
			return fmt.Errorf("dump interface: %w", err)
		}
		if stop {
			break
		}
		if msg.InterfaceName == name {
			v.ifaceCache[name] = msg.SwIfIndex
			v.logger.Info("Loopback already exists in VPP, reconciling IPs", "interface", name, "sw_if_index", msg.SwIfIndex)
			return v.reconcileLoopbackIPs(name, ipv4, ipv6)
		}
	}

	req := &interfaces.CreateLoopback{}
	reply := &interfaces.CreateLoopbackReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("create loopback: %w", err)
	}

	v.ifaceCache[name] = reply.SwIfIndex

	renameReq := &interfaces.SwInterfaceSetInterfaceName{
		SwIfIndex: reply.SwIfIndex,
		Name:      name,
	}
	renameReply := &interfaces.SwInterfaceSetInterfaceNameReply{}
	if err := ch.SendRequest(renameReq).ReceiveReply(renameReply); err != nil {
		v.logger.Warn("Failed to rename loopback", "error", err)
	}

	setUpReq := &interfaces.SwInterfaceSetFlags{
		SwIfIndex: reply.SwIfIndex,
		Flags:     interface_types.IF_STATUS_API_FLAG_ADMIN_UP,
	}
	setUpReply := &interfaces.SwInterfaceSetFlagsReply{}
	if err := ch.SendRequest(setUpReq).ReceiveReply(setUpReply); err != nil {
		return fmt.Errorf("set loopback up: %w", err)
	}

	if v.virtualMAC != nil {
		if err := v.setInterfaceMAC(reply.SwIfIndex, v.virtualMAC); err != nil {
			v.logger.Warn("Failed to set virtual MAC on loopback", "error", err)
		}
	}

	lcpReq := &lcp.LcpItfPairAddDelV3{
		IsAdd:      true,
		SwIfIndex:  reply.SwIfIndex,
		HostIfName: name,
		HostIfType: lcp.LCP_API_ITF_HOST_TAP,
		Netns:      "",
	}
	lcpReply := &lcp.LcpItfPairAddDelV3Reply{}
	if err := ch.SendRequest(lcpReq).ReceiveReply(lcpReply); err != nil {
		v.logger.Warn("Failed to create linux-cp pair for loopback", "error", err)
	} else {
		v.logger.Info("Created linux-cp pair in default namespace", "interface", name)
	}

	link, err := netlink.LinkByName(name)
	if err != nil {
		return fmt.Errorf("find linux interface %s: %w", name, err)
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("set linux interface up: %w", err)
	}

	for _, cidr := range ipv4 {
		addr, err := netlink.ParseAddr(cidr)
		if err != nil {
			return fmt.Errorf("parse ipv4 %s: %w", cidr, err)
		}
		addr.IPNet.Mask = net.CIDRMask(32, 32)
		normalizedCIDR := addr.IPNet.String()

		if err := netlink.AddrAdd(link, addr); err != nil && !os.IsExist(err) {
			return fmt.Errorf("add ipv4 %s to linux interface: %w", normalizedCIDR, err)
		}
		v.logger.Info("Added IPv4 to Linux interface (nl plugin will sync to VPP)", "interface", name, "addr", normalizedCIDR)
	}

	for _, cidr := range ipv6 {
		addr, err := netlink.ParseAddr(cidr)
		if err != nil {
			v.logger.Warn("Failed to parse IPv6 address", "address", cidr, "error", err)
			continue
		}
		addr.IPNet.Mask = net.CIDRMask(128, 128)
		normalizedCIDR := addr.IPNet.String()

		if err := netlink.AddrAdd(link, addr); err != nil && !os.IsExist(err) {
			v.logger.Warn("Failed to add IPv6 address to linux interface", "address", normalizedCIDR, "error", err)
		} else {
			v.logger.Info("Added IPv6 to Linux interface (nl plugin will sync to VPP)", "interface", name, "addr", normalizedCIDR)
		}
	}

	v.logger.Info("Created loopback", "interface", name, "sw_if_index", reply.SwIfIndex)
	return nil
}

func (v *VPP) reconcileLoopbackIPs(name string, desiredIPv4 []string, desiredIPv6 []string) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		v.logger.Warn("Linux interface not found for reconciliation, skipping", "interface", name, "error", err)
		return nil
	}

	existingAddrs, err := netlink.AddrList(link, netlink.FAMILY_ALL)
	if err != nil {
		return fmt.Errorf("list addresses on %s: %w", name, err)
	}

	desiredV4Map := make(map[string]bool)
	for _, cidr := range desiredIPv4 {
		addr, err := netlink.ParseAddr(cidr)
		if err != nil {
			continue
		}
		addr.IPNet.Mask = net.CIDRMask(32, 32)
		desiredV4Map[addr.IPNet.String()] = true
	}
	desiredV6Map := make(map[string]bool)
	for _, cidr := range desiredIPv6 {
		addr, err := netlink.ParseAddr(cidr)
		if err != nil {
			continue
		}
		addr.IPNet.Mask = net.CIDRMask(128, 128)
		desiredV6Map[addr.IPNet.String()] = true
	}

	existingMap := make(map[string]netlink.Addr)
	for _, addr := range existingAddrs {
		existingMap[addr.IPNet.String()] = addr
	}

	for cidrStr, addr := range existingMap {
		if addr.IP.IsLinkLocalUnicast() {
			continue
		}

		isV4Desired := desiredV4Map[cidrStr]
		isV6Desired := desiredV6Map[cidrStr]

		if !isV4Desired && !isV6Desired {
			if err := netlink.AddrDel(link, &addr); err != nil {
				v.logger.Warn("Failed to remove extra IP from interface", "interface", name, "addr", cidrStr, "error", err)
			} else {
				v.logger.Info("Removed extra IP from Linux interface (not in config)", "interface", name, "addr", cidrStr)
			}
		}
	}

	swIfIndex, ok := v.ifaceCache[name]
	if !ok {
		return fmt.Errorf("interface %s not in cache", name)
	}

	vppAddrs, err := v.getInterfaceAddresses(swIfIndex)
	if err != nil {
		v.logger.Warn("Failed to get VPP addresses, skipping VPP reconciliation", "interface", name, "error", err)
		vppAddrs = make(map[string]bool)
	}

	for _, cidr := range desiredIPv4 {
		addr, err := netlink.ParseAddr(cidr)
		if err != nil {
			return fmt.Errorf("parse ipv4 %s: %w", cidr, err)
		}
		addr.IPNet.Mask = net.CIDRMask(32, 32)
		normalizedCIDR := addr.IPNet.String()

		if _, exists := existingMap[normalizedCIDR]; !exists {
			if err := netlink.AddrAdd(link, addr); err != nil {
				return fmt.Errorf("add ipv4 %s to linux: %w", normalizedCIDR, err)
			}
			v.logger.Info("Added IPv4 to Linux interface", "interface", name, "addr", normalizedCIDR)
		}

		if !vppAddrs[normalizedCIDR] {
			ch, _ := v.conn.NewAPIChannel()
			if err := v.addIPAddress(ch, swIfIndex, normalizedCIDR, false); err != nil {
				v.logger.Warn("Failed to add IPv4 to VPP", "addr", normalizedCIDR, "error", err)
			} else {
				v.logger.Info("Added IPv4 to VPP interface", "interface", name, "addr", normalizedCIDR)
			}
			ch.Close()
		}
	}

	for _, cidr := range desiredIPv6 {
		addr, err := netlink.ParseAddr(cidr)
		if err != nil {
			v.logger.Warn("Failed to parse IPv6 address", "address", cidr, "error", err)
			continue
		}
		addr.IPNet.Mask = net.CIDRMask(128, 128)
		normalizedCIDR := addr.IPNet.String()

		if _, exists := existingMap[normalizedCIDR]; !exists {
			if err := netlink.AddrAdd(link, addr); err != nil {
				v.logger.Warn("Failed to add IPv6 to linux", "address", normalizedCIDR, "error", err)
			} else {
				v.logger.Info("Added IPv6 to Linux interface", "interface", name, "addr", normalizedCIDR)
			}
		}

		if !vppAddrs[normalizedCIDR] {
			ch, _ := v.conn.NewAPIChannel()
			if err := v.addIPAddress(ch, swIfIndex, normalizedCIDR, true); err != nil {
				v.logger.Warn("Failed to add IPv6 to VPP", "addr", normalizedCIDR, "error", err)
			} else {
				v.logger.Info("Added IPv6 to VPP interface", "interface", name, "addr", normalizedCIDR)
			}
			ch.Close()
		}
	}

	v.logger.Info("IP reconciliation complete", "interface", name, "ipv4_count", len(desiredIPv4), "ipv6_count", len(desiredIPv6))
	return nil
}

func (v *VPP) SetUnnumbered(ifaceName, loopbackName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	ifaceIdx, ok := v.ifaceCache[ifaceName]
	if !ok {
		return fmt.Errorf("interface %s not found", ifaceName)
	}

	loopbackIdx, ok := v.ifaceCache[loopbackName]
	if !ok {
		idx, err := v.GetInterfaceIndex(loopbackName)
		if err != nil {
			return fmt.Errorf("loopback %s not found: %w", loopbackName, err)
		}
		loopbackIdx = interface_types.InterfaceIndex(idx)
	}

	req := &interfaces.SwInterfaceSetUnnumbered{
		SwIfIndex:           loopbackIdx,
		UnnumberedSwIfIndex: ifaceIdx,
		IsAdd:               true,
	}

	reply := &interfaces.SwInterfaceSetUnnumberedReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("set unnumbered: %w", err)
	}

	v.logger.Info("Set interface unnumbered", "interface", ifaceName, "loopback", loopbackName)
	return nil
}

func (v *VPP) RegisterPuntSocket(socketPath string, port uint16, iface string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	ifIdx, ok := v.ifaceCache[iface]
	if !ok {
		return fmt.Errorf("interface %s not found in cache", iface)
	}

	puntL4 := punt.PuntL4{
		Af:       ip_types.ADDRESS_IP4,
		Protocol: ip_types.IP_API_PROTO_UDP,
		Port:     port,
	}

	var puntUnion punt.PuntUnion
	puntUnion.SetL4(puntL4)

	req := &punt.PuntSocketRegister{
		HeaderVersion: 1,
		Punt: punt.Punt{
			Type: punt.PUNT_API_TYPE_L4,
			Punt: puntUnion,
		},
		Pathname: socketPath,
	}

	v.logger.Info("Registering punt socket",
		"path", socketPath,
		"port", port,
		"interface", iface,
		"sw_if_index", ifIdx)

	reply := &punt.PuntSocketRegisterReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("register punt socket API call: %w", err)
	}

	v.logger.Info("Punt socket register reply", "retval", reply.Retval, "pathname", reply.Pathname)

	if reply.Retval != 0 {
		return fmt.Errorf("punt socket register failed with retval: %d (pathname: %s)", reply.Retval, reply.Pathname)
	}

	dumpReq := &punt.PuntSocketDump{
		Type: punt.PUNT_API_TYPE_L4,
	}

	reqCtx := ch.SendMultiRequest(dumpReq)
	found := false
	for {
		msg := &punt.PuntSocketDetails{}
		stop, err := reqCtx.ReceiveReply(msg)
		if stop {
			break
		}
		if err != nil {
			v.logger.Warn("Failed to dump punt sockets", "error", err)
			break
		}

		if msg.Punt.Type == punt.PUNT_API_TYPE_L4 {
			l4 := msg.Punt.Punt.GetL4()
			v.logger.Info("Found punt socket", "af", l4.Af, "proto", l4.Protocol, "port", l4.Port, "path", msg.Pathname)
			if l4.Port == port && l4.Protocol == ip_types.IP_API_PROTO_UDP && l4.Af == ip_types.ADDRESS_IP4 {
				found = true
			}
		}
	}

	if !found {
		v.logger.Warn("Punt socket registered successfully but not found in dump - VPP may not actually punt", "port", port)
	} else {
		v.logger.Info("Verified punt socket registration", "port", port, "path", socketPath)
	}

	return nil
}

func (v *VPP) SetupCPEgressInterface(hostIfName, accessIface string) error {
	tapName, err := v.CreateTAP(hostIfName)
	if err != nil {
		return fmt.Errorf("create CP egress tap: %w", err)
	}

	v.logger.Info("CP egress interface configured", "tap", tapName)
	return nil
}

func (v *VPP) EnableDirectedBroadcast(ifaceName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	swIfIndex, ok := v.ifaceCache[ifaceName]
	if !ok {
		return fmt.Errorf("interface %s not found in cache", ifaceName)
	}

	req := &interfaces.SwInterfaceSetIPDirectedBroadcast{
		SwIfIndex: swIfIndex,
		Enable:    true,
	}

	reply := &interfaces.SwInterfaceSetIPDirectedBroadcastReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("enable directed broadcast API call: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("enable directed broadcast failed with retval: %d", reply.Retval)
	}

	v.logger.Info("Enabled directed broadcast", "interface", ifaceName)
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
	return ch.SendRequest(req).ReceiveReply(reply)
}

func (v *VPP) BuildL2Rewrite(dstMAC, srcMAC string, outerVLAN, innerVLAN uint16) []byte {
	dst, err := net.ParseMAC(dstMAC)
	if err != nil {
		return nil
	}
	src, err := net.ParseMAC(srcMAC)
	if err != nil {
		return nil
	}

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true}

	eth := &layers.Ethernet{
		SrcMAC: src,
		DstMAC: dst,
	}

	var layerStack []gopacket.SerializableLayer

	if outerVLAN > 0 && innerVLAN > 0 {
		eth.EthernetType = layers.EthernetTypeDot1Q
		dot1qOuter := &layers.Dot1Q{
			VLANIdentifier: outerVLAN,
			Type:           layers.EthernetTypeDot1Q,
		}
		dot1qInner := &layers.Dot1Q{
			VLANIdentifier: innerVLAN,
			Type:           layers.EthernetTypeIPv4,
		}
		layerStack = []gopacket.SerializableLayer{eth, dot1qOuter, dot1qInner}
	} else if outerVLAN > 0 {
		eth.EthernetType = layers.EthernetTypeDot1Q
		dot1q := &layers.Dot1Q{
			VLANIdentifier: outerVLAN,
			Type:           layers.EthernetTypeIPv4,
		}
		layerStack = []gopacket.SerializableLayer{eth, dot1q}
	} else {
		eth.EthernetType = layers.EthernetTypeIPv4
		layerStack = []gopacket.SerializableLayer{eth}
	}

	if err := gopacket.SerializeLayers(buf, opts, layerStack...); err != nil {
		return nil
	}

	bytes := buf.Bytes()

	expectedSize := 14
	if outerVLAN > 0 {
		expectedSize += 4
	}
	if innerVLAN > 0 {
		expectedSize += 4
	}

	if len(bytes) > expectedSize {
		bytes = bytes[:expectedSize]
	}

	v.logger.Debug("Built L2 rewrite",
		"dst_mac", dstMAC,
		"src_mac", srcMAC,
		"outer_vlan", outerVLAN,
		"inner_vlan", innerVLAN,
		"length", len(bytes),
		"bytes", fmt.Sprintf("%x", bytes))

	return bytes
}

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

func (v *VPP) toPrefix(ipAddr string, prefixLen int) (ip_types.Prefix, error) {
	ip := net.ParseIP(ipAddr)
	if ip == nil {
		return ip_types.Prefix{}, fmt.Errorf("invalid IP address: %s", ipAddr)
	}

	addr, err := v.toAddress(ip)
	if err != nil {
		return ip_types.Prefix{}, err
	}

	return ip_types.Prefix{
		Address: addr,
		Len:     uint8(prefixLen),
	}, nil
}

func (v *VPP) AddAdjacencyWithRewrite(ipAddr string, swIfIndex uint32, rewrite []byte) (uint32, error) {
	ip := net.ParseIP(ipAddr)
	if ip == nil {
		return 0, fmt.Errorf("invalid IP address: %s", ipAddr)
	}

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

	linkType := uint8(0)
	if ip.To4() == nil {
		linkType = 1
	}

	v.fibMux.Lock()
	defer v.fibMux.Unlock()

	req := &fib_control.FibControlAdjAddRewrite{
		SwIfIndex:  interface_types.InterfaceIndex(swIfIndex),
		NhAddr:     addr,
		LinkType:   linkType,
		RewriteLen: uint8(len(rewrite)),
		Rewrite:    make([]byte, 128),
	}
	copy(req.Rewrite, rewrite)

	v.logger.Debug("Calling VPP fib_control_adj_add_rewrite",
		"ip", ipAddr,
		"sw_if_index", swIfIndex,
		"link_type", linkType,
		"rewrite_len", len(rewrite))

	reply := &fib_control.FibControlAdjAddRewriteReply{}
	if err := v.fibChan.SendRequest(req).ReceiveReply(reply); err != nil {
		return 0, fmt.Errorf("add adjacency with rewrite: %w", err)
	}

	if reply.Retval != 0 {
		return 0, fmt.Errorf("add adjacency failed with retval: %d", reply.Retval)
	}

	v.logger.Debug("VPP adjacency created", "adj_index", reply.AdjIndex)
	return reply.AdjIndex, nil
}

func (v *VPP) UnlockAdjacency(adjIndex uint32) error {
	v.fibMux.Lock()
	defer v.fibMux.Unlock()

	req := &fib_control.FibControlAdjUnlock{
		AdjIndex: adjIndex,
	}

	reply := &fib_control.FibControlAdjUnlockReply{}
	if err := v.fibChan.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("unlock adjacency: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("unlock adjacency failed: %d", reply.Retval)
	}

	return nil
}

func (v *VPP) AddHostRoute(ipAddr string, adjIndex uint32, fibID uint32, swIfIndex uint32) error {
	ip := net.ParseIP(ipAddr)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", ipAddr)
	}

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

	prefix := ip_types.Prefix{
		Address: addr,
		Len:     32,
	}

	v.fibMux.Lock()
	defer v.fibMux.Unlock()

	req := &fib_control.FibControlAddHostRoute{
		TableID:   fibID,
		Prefix:    prefix,
		AdjIndex:  adjIndex,
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
	}

	v.logger.Debug("Calling VPP fib_control_add_host_route",
		"ip", ipAddr,
		"adj_index", adjIndex,
		"fib_id", fibID,
		"sw_if_index", swIfIndex)

	reply := &fib_control.FibControlAddHostRouteReply{}
	if err := v.fibChan.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("add host route: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("add host route failed with retval: %d", reply.Retval)
	}

	v.logger.Debug("VPP host route added", "ip", ipAddr)
	return nil
}

func (v *VPP) DeleteHostRoute(ipAddr string, fibID uint32) error {
	ip := net.ParseIP(ipAddr)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", ipAddr)
	}

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

	prefix := ip_types.Prefix{
		Address: addr,
		Len:     32,
	}

	v.fibMux.Lock()
	defer v.fibMux.Unlock()

	req := &fib_control.FibControlDelHostRoute{
		TableID: fibID,
		Prefix:  prefix,
	}

	reply := &fib_control.FibControlDelHostRouteReply{}
	if err := v.fibChan.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("delete host route: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("delete host route failed: %d", reply.Retval)
	}

	return nil
}

func (v *VPP) GetFIBIDForInterface(swIfIndex uint32) (uint32, error) {
	return 0, nil
}

func (v *VPP) EnableARPPunt(ifaceName, socketPath string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	req := &arp_punt.ArpPuntEnableDisable{
		SwIfIndex:  interface_types.InterfaceIndex(idx),
		SocketPath: socketPath,
		Enable:     true,
	}

	reply := &arp_punt.ArpPuntEnableDisableReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("enable arp punt: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("enable arp punt failed: retval=%d", reply.Retval)
	}

	v.logger.Info("Enabled ARP punt", "interface", ifaceName, "sw_if_index", idx, "socket", socketPath)
	return nil
}

func (v *VPP) DisableARPReply(ifaceName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}
	swIfIndex := interface_types.InterfaceIndex(idx)

	req := &interfaces.SwInterfaceSetFlags{
		SwIfIndex: swIfIndex,
		Flags:     interface_types.IF_STATUS_API_FLAG_ADMIN_UP,
	}

	reply := &interfaces.SwInterfaceSetFlagsReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("set interface flags: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("disable arp-reply failed: retval=%d", reply.Retval)
	}

	v.logger.Info("Disabled ARP reply", "interface", ifaceName, "sw_if_index", idx)
	return nil
}

func (v *VPP) EnableAccounting(ifaceName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	req := &openbng_accounting.OpenbngAcctInterfaceEnableDisable{
		SwIfIndex: interface_types.InterfaceIndex(idx),
		Enable:    true,
	}

	reply := &openbng_accounting.OpenbngAcctInterfaceEnableDisableReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("enable accounting: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("enable accounting failed: retval=%d", reply.Retval)
	}

	v.logger.Info("Enabled accounting", "interface", ifaceName, "sw_if_index", idx)
	return nil
}

func (v *VPP) AddAccountingSubscriber(ipAddr string, fibIndex uint32, swIfIndex uint32, sessionID string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	ip := net.ParseIP(ipAddr)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", ipAddr)
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return fmt.Errorf("not an IPv4 address: %s", ipAddr)
	}

	var ip4Addr ip_types.IP4Address
	copy(ip4Addr[:], ip4)

	if len(sessionID) != 8 {
		return fmt.Errorf("invalid session ID length: expected 8, got %d", len(sessionID))
	}
	var sessionIDBytes [8]byte
	copy(sessionIDBytes[:], sessionID)

	req := &openbng_accounting.OpenbngAcctSubscriberAdd{
		IP4Address: ip4Addr,
		FibIndex:   fibIndex,
		SwIfIndex:  interface_types.InterfaceIndex(swIfIndex),
		SessionID:  sessionIDBytes[:],
	}

	reply := &openbng_accounting.OpenbngAcctSubscriberAddReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("add subscriber: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("add subscriber failed: retval=%d", reply.Retval)
	}

	v.logger.Info("Added accounting subscriber", "ip", ipAddr, "fib_index", fibIndex, "sw_if_index", swIfIndex, "session_id", sessionID)
	return nil
}

func (v *VPP) DeleteAccountingSubscriber(ipAddr string, fibIndex uint32) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	ip := net.ParseIP(ipAddr)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", ipAddr)
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return fmt.Errorf("not an IPv4 address: %s", ipAddr)
	}

	var ip4Addr ip_types.IP4Address
	copy(ip4Addr[:], ip4)

	req := &openbng_accounting.OpenbngAcctSubscriberDel{
		IP4Address: ip4Addr,
		FibIndex:   fibIndex,
	}

	reply := &openbng_accounting.OpenbngAcctSubscriberDelReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("delete subscriber: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("delete subscriber failed: retval=%d", reply.Retval)
	}

	v.logger.Info("Deleted accounting subscriber", "ip", ipAddr, "fib_index", fibIndex)
	return nil
}

func (v *VPP) GetSubscriberStats(ctx context.Context) ([]subscribers.Statistics, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	stream := ch.SendMultiRequest(&openbng_accounting.OpenbngAcctSubscriberDump{})

	var stats []subscribers.Statistics
	receivedCount := 0

	for {
		details := &openbng_accounting.OpenbngAcctSubscriberDetails{}
		stop, err := stream.ReceiveReply(details)
		if err != nil || stop {
			break
		}

		receivedCount++
		v.logger.Debug("Received subscriber accounting details",
			"session_id", string(details.SessionID[:]),
			"ip", net.IP(details.IP4Address[:]).String(),
			"rx_packets", details.RxPackets,
			"tx_packets", details.TxPackets,
			"rx_bytes", details.RxBytes,
			"tx_bytes", details.TxBytes)

		if details.RxPackets > 0 || details.TxPackets > 0 {
			stats = append(stats, subscribers.Statistics{
				SessionID: string(details.SessionID[:]),
				IP:        net.IP(details.IP4Address[:]),
				RxPackets: details.RxPackets,
				RxBytes:   details.RxBytes,
				TxPackets: details.TxPackets,
				TxBytes:   details.TxBytes,
			})
		}
	}

	v.logger.Debug("GetSubscriberStats complete", "received", receivedCount, "filtered", len(stats))
	return stats, nil
}

func (v *VPP) CreateLCPPair(vppIface, linuxIface string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	swIfIndex, err := v.GetInterfaceIndex(vppIface)
	if err != nil {
		return fmt.Errorf("get interface index for %s: %w", vppIface, err)
	}

	req := &lcp.LcpItfPairAddDel{
		IsAdd:      true,
		SwIfIndex:  interface_types.InterfaceIndex(swIfIndex),
		HostIfName: linuxIface,
		HostIfType: lcp.LCP_API_ITF_HOST_TAP,
	}

	reply := &lcp.LcpItfPairAddDelReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("create LCP pair: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("create LCP pair failed: retval=%d", reply.Retval)
	}

	v.logger.Info("Created LCP pair", "vpp_iface", vppIface, "linux_iface", linuxIface)
	return nil
}

func (v *VPP) SetupMemifDataplane(memifID uint32, accessIface string, socketPath string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	if socketPath == "" {
		socketPath = "/run/osvbng/memif.sock"
	}
	socketID := uint32(1)

	socketReq := &memif.MemifSocketFilenameAddDelV2{
		IsAdd:          true,
		SocketID:       socketID,
		SocketFilename: socketPath,
	}
	socketReply := &memif.MemifSocketFilenameAddDelV2Reply{}
	if err := ch.SendRequest(socketReq).ReceiveReply(socketReply); err != nil {
		return fmt.Errorf("create memif socket: %w", err)
	}
	if socketReply.Retval != 0 {
		return fmt.Errorf("create memif socket failed: retval=%d", socketReply.Retval)
	}
	v.logger.Info("Created memif socket", "path", socketPath, "socket_id", socketID)

	memifReq := &memif.MemifCreateV2{
		Role:     memif.MEMIF_ROLE_API_MASTER,
		Mode:     memif.MEMIF_MODE_API_ETHERNET,
		ID:       memifID,
		SocketID: socketID,
	}

	memifReply := &memif.MemifCreateV2Reply{}
	if err := ch.SendRequest(memifReq).ReceiveReply(memifReply); err != nil {
		return fmt.Errorf("create memif: %w", err)
	}

	if memifReply.Retval != 0 {
		return fmt.Errorf("create memif failed: retval=%d", memifReply.Retval)
	}

	memifName := fmt.Sprintf("memif%d/%d", memifID, socketID)
	v.ifaceCache[memifName] = memifReply.SwIfIndex
	v.logger.Info("Created memif interface", "id", memifID, "name", memifName, "sw_if_index", memifReply.SwIfIndex)

	setStateReq := &interfaces.SwInterfaceSetFlags{
		SwIfIndex: memifReply.SwIfIndex,
		Flags:     interface_types.IF_STATUS_API_FLAG_ADMIN_UP,
	}
	setStateReply := &interfaces.SwInterfaceSetFlagsReply{}
	if err := ch.SendRequest(setStateReq).ReceiveReply(setStateReply); err != nil {
		return fmt.Errorf("set memif state up: %w", err)
	}

	if err := v.SetL2CrossConnect(memifName, accessIface); err != nil {
		return fmt.Errorf("setup L2 xconnect %s <-> %s: %w", memifName, accessIface, err)
	}

	if err := v.SetL2CrossConnect(accessIface, memifName); err != nil {
		return fmt.Errorf("setup L2 xconnect %s <-> %s: %w", accessIface, memifName, err)
	}

	accessIdx, ok := v.ifaceCache[accessIface]
	if ok {
		setAccessUpReq := &interfaces.SwInterfaceSetFlags{
			SwIfIndex: accessIdx,
			Flags:     interface_types.IF_STATUS_API_FLAG_ADMIN_UP,
		}
		setAccessUpReply := &interfaces.SwInterfaceSetFlagsReply{}
		if err := ch.SendRequest(setAccessUpReq).ReceiveReply(setAccessUpReply); err != nil {
			return fmt.Errorf("set access interface up: %w", err)
		}
		v.logger.Info("Set access interface up", "interface", accessIface)
	}

	v.logger.Info("Setup memif dataplane", "memif", memifName, "access_iface", accessIface)
	return nil
}

func (v *VPP) GetSystemThreads() ([]system.Thread, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return []system.Thread{}, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &vlib.ShowThreads{}
	reply := &vlib.ShowThreadsReply{}

	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return []system.Thread{}, fmt.Errorf("GetSystemThreads(): %w", err)
	}

	if reply.Retval != 0 {
		return []system.Thread{}, fmt.Errorf("failed to gather system threads: retval=%d", reply.Retval)
	}

	var systemThreads []system.Thread
	for _, t := range reply.ThreadData {
		systemThreads = append(systemThreads, system.Thread{
			ID:        t.ID,
			Name:      t.Name,
			Type:      t.Type,
			ProcessID: t.PID,
			CPUID:     t.CPUID,
			CPUCore:   t.Core,
			CPUSocket: t.CPUSocket,
		})
	}

	return systemThreads, nil
}

type IPTableInfo struct {
	TableID uint32 `json:"table_id"`
	Name    string `json:"name"`
	IsIPv6  bool   `json:"is_ipv6"`
}

type IPMTableInfo struct {
	TableID uint32
	Name    string
	IsIPv6  bool
}

func (v *VPP) GetIPTables() ([]*IPTableInfo, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &ip.IPTableDump{}
	reqCtx := ch.SendMultiRequest(req)

	var tables []*IPTableInfo
	for {
		reply := &ip.IPTableDetails{}
		stop, err := reqCtx.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("receive IP table details: %w", err)
		}

		tables = append(tables, &IPTableInfo{
			TableID: reply.Table.TableID,
			Name:    strings.TrimRight(reply.Table.Name, "\x00"),
			IsIPv6:  reply.Table.IsIP6,
		})
	}

	return tables, nil
}

func (v *VPP) GetIPMTables() ([]*IPMTableInfo, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &ip.IPMtableDump{}
	reqCtx := ch.SendMultiRequest(req)

	var tables []*IPMTableInfo
	for {
		reply := &ip.IPMtableDetails{}
		stop, err := reqCtx.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("receive IP mtable details: %w", err)
		}

		tables = append(tables, &IPMTableInfo{
			TableID: reply.Table.TableID,
			Name:    strings.TrimRight(reply.Table.Name, "\x00"),
			IsIPv6:  reply.Table.IsIP6,
		})
	}

	return tables, nil
}

func (v *VPP) GetMPLSTables() ([]*MPLSTableInfo, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &mpls.MplsTableDump{}
	reqCtx := ch.SendMultiRequest(req)

	var tables []*MPLSTableInfo
	for {
		reply := &mpls.MplsTableDetails{}
		stop, err := reqCtx.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("receive MPLS table details: %w", err)
		}

		tables = append(tables, &MPLSTableInfo{
			TableID: reply.MtTable.MtTableID,
			Name:    strings.TrimRight(reply.MtTable.MtName, "\x00"),
		})
	}

	return tables, nil
}

type MPLSTableInfo struct {
	TableID uint32
	Name    string
}

func (v *VPP) GetNextAvailableGlobalTableId() (uint32, error) {
	usedIDs := make(map[uint32]bool)

	usedIDs[0] = true

	ipTables, _ := v.GetIPTables()
	for _, t := range ipTables {
		usedIDs[t.TableID] = true
	}

	mTables, _ := v.GetIPMTables()
	for _, t := range mTables {
		usedIDs[t.TableID] = true
	}

	mplsTables, _ := v.GetMPLSTables()
	for _, t := range mplsTables {
		usedIDs[t.TableID] = true
	}

	for i := uint32(1); i < 4294967295; i++ {
		if !usedIDs[i] {
			return i, nil
		}
	}

	// Is this risky to return 0? 0 is the default... but we do return an error, but someone could just ignore the error
	return 0, fmt.Errorf("no available table IDs")
}
