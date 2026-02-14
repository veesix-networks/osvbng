package southbound

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"

	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models/system"
	"github.com/vishvananda/netlink"
	"go.fd.io/govpp/api"
	"go.fd.io/govpp/core"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
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
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/mfib_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/mpls"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip6_nd"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/osvbng_ipoe"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/osvbng_pppoe"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/osvbng_punt"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/punt"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/tapv2"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/vlib"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/vpe"
)

type VPP struct {
	conn        *core.Connection
	ifMgr       *ifmgr.Manager
	virtualMAC  net.HardwareAddr
	logger      *slog.Logger
	fibChan     api.Channel
	fibMux      sync.Mutex
	useDPDK     bool
	asyncWorker *AsyncWorker
	statsClient *StatsClient
}

type VPPConfig struct {
	Connection      *core.Connection
	IfMgr           *ifmgr.Manager
	UseDPDK         bool
	StatsSocketPath string
}

func NewVPP(cfg VPPConfig) (*VPP, error) {
	if cfg.Connection == nil {
		return nil, fmt.Errorf("VPP connection is required")
	}
	if cfg.IfMgr == nil {
		return nil, fmt.Errorf("IfMgr is required")
	}

	conn := cfg.Connection

	fibChan, err := conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create FIB API channel: %w", err)
	}

	asyncWorker, err := NewAsyncWorker(conn, DefaultAsyncWorkerConfig())
	if err != nil {
		fibChan.Close()
		return nil, fmt.Errorf("create async worker: %w", err)
	}

	statsClient := NewStatsClient(cfg.StatsSocketPath)
	if err := statsClient.Connect(); err != nil {
		fibChan.Close()
		return nil, fmt.Errorf("connect to stats: %w", err)
	}

	v := &VPP{
		conn:        conn,
		ifMgr:       cfg.IfMgr,
		logger:      logger.Get(logger.Southbound),
		fibChan:     fibChan,
		useDPDK:     cfg.UseDPDK,
		asyncWorker: asyncWorker,
		statsClient: statsClient,
	}

	if err := v.LoadInterfaces(); err != nil {
		statsClient.Disconnect()
		fibChan.Close()
		return nil, fmt.Errorf("load interfaces: %w", err)
	}

	if err := v.LoadIPState(); err != nil {
		v.logger.Warn("Failed to load IP state at startup", "error", err)
	}

	asyncWorker.Start()

	v.logger.Debug("Connected to VPP", "interfaces_loaded", len(v.ifMgr.List()))

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

	v.ifMgr.Add(&ifmgr.Interface{
		SwIfIndex:    uint32(reply.SwIfIndex),
		SupSwIfIndex: uint32(reply.SwIfIndex),
		Name:         tapName,
		DevType:      "tap",
		Type:         ifmgr.IfTypeHardware,
		AdminUp:      true,
	})

	setUpReq := &interfaces.SwInterfaceSetFlags{
		SwIfIndex: reply.SwIfIndex,
		Flags:     interface_types.IF_STATUS_API_FLAG_ADMIN_UP,
	}

	setUpReply := &interfaces.SwInterfaceSetFlagsReply{}
	if err := ch.SendRequest(setUpReq).ReceiveReply(setUpReply); err != nil {
		return "", fmt.Errorf("set interface up: %w", err)
	}

	v.logger.Debug("Created TAP interface", "interface", tapName, "sw_if_index", reply.SwIfIndex)

	return tapName, nil
}

func (v *VPP) SetL2CrossConnect(iface1, iface2 string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	if1 := v.ifMgr.GetByName(iface1)
	if if1 == nil {
		return fmt.Errorf("interface %s not found", iface1)
	}

	if2 := v.ifMgr.GetByName(iface2)
	if if2 == nil {
		return fmt.Errorf("interface %s not found", iface2)
	}

	req1 := &l2.SwInterfaceSetL2Xconnect{
		RxSwIfIndex: interface_types.InterfaceIndex(if1.SwIfIndex),
		TxSwIfIndex: interface_types.InterfaceIndex(if2.SwIfIndex),
		Enable:      true,
	}

	reply1 := &l2.SwInterfaceSetL2XconnectReply{}
	if err := ch.SendRequest(req1).ReceiveReply(reply1); err != nil {
		return fmt.Errorf("set xconnect %s->%s: %w", iface1, iface2, err)
	}

	v.logger.Debug("Set L2 cross-connect (one-way)", "from", iface1, "to", iface2)

	return nil
}

func (v *VPP) Close() error {
	if v.asyncWorker != nil {
		v.asyncWorker.Stop()
	}
	if v.statsClient != nil {
		v.statsClient.Disconnect()
	}
	if v.fibChan != nil {
		v.fibChan.Close()
	}
	v.conn.Disconnect()
	return nil
}

func (v *VPP) GetDataplaneStats() (*DataplaneStats, error) {
	return v.statsClient.GetAllStats()
}

func (v *VPP) GetSystemStats() (*SystemStats, error) {
	return v.statsClient.GetSystemStats()
}

func (v *VPP) GetMemoryStats() ([]MemoryStats, error) {
	return v.statsClient.GetMemoryStats()
}

func (v *VPP) GetInterfaceStats() ([]InterfaceStats, error) {
	return v.statsClient.GetInterfaceStats()
}

func (v *VPP) GetNodeStats() ([]NodeStats, error) {
	return v.statsClient.GetNodeStats()
}

func (v *VPP) GetErrorStats() ([]ErrorStats, error) {
	return v.statsClient.GetErrorStats()
}

func (v *VPP) GetBufferStats() ([]BufferStats, error) {
	return v.statsClient.GetBufferStats()
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

func (v *VPP) AddNeighbor(ipAddr, macAddr, ifaceName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	iface := v.ifMgr.GetByName(ifaceName)
	if iface == nil {
		return fmt.Errorf("interface %s not found", ifaceName)
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
			SwIfIndex:  interface_types.InterfaceIndex(iface.SwIfIndex),
			Flags:      ip_neighbor.IP_API_NEIGHBOR_FLAG_STATIC,
			MacAddress: macAddress,
			IPAddress:  ipAddress,
		},
	}

	reply := &ip_neighbor.IPNeighborAddDelReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("add neighbor: %w", err)
	}

	v.logger.Debug("Added neighbor", "ip", ipAddr, "mac", macAddr, "interface", iface)
	return nil
}

func (v *VPP) DeleteNeighbor(ipAddr, ifaceName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	iface := v.ifMgr.GetByName(ifaceName)
	if iface == nil {
		return fmt.Errorf("interface %s not found", ifaceName)
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
			SwIfIndex: interface_types.InterfaceIndex(iface.SwIfIndex),
			IPAddress: ipAddress,
		},
	}

	reply := &ip_neighbor.IPNeighborAddDelReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("delete neighbor: %w", err)
	}

	v.logger.Debug("Deleted neighbor", "ip", ipAddr, "interface", ifaceName)
	return nil
}

func (v *VPP) ApplyQoS(swIfIndex uint32, upMbps, downMbps int) error {
	v.logger.Debug("QoS not yet implemented", "sw_if_index", swIfIndex, "up_mbps", upMbps, "down_mbps", downMbps)
	return nil
}

func (v *VPP) RemoveQoS(swIfIndex uint32) error {
	v.logger.Debug("QoS removal not yet implemented", "sw_if_index", swIfIndex)
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

	v.logger.Debug("Added route", "prefix", prefix, "nexthop", nexthop)
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

	v.logger.Debug("Added local route", "prefix", prefix)
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

	v.logger.Debug("Deleted route", "prefix", prefix)
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
	v.logger.Debug("Got interface MAC from VPP", "sw_if_index", swIfIndex, "interface_name", reply.InterfaceName, "mac", mac.String())
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
		if err := v.SetInterfaceMAC(interface_types.InterfaceIndex(idx), linuxIf.HardwareAddr); err != nil {
			v.logger.Warn("Failed to set VPP interface MAC from Linux", "interface", ifaceName, "error", err)
		} else {
			v.logger.Debug("Synced VPP interface MAC from Linux", "interface", ifaceName, "mac", linuxIf.HardwareAddr)
		}
	}

	v.logger.Debug("Set interface promiscuous", "interface", ifaceName, "on", on)
	return nil
}

func (v *VPP) GetLCPHostInterface(vppIfaceName string) (string, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return "", err
	}
	defer ch.Close()

	iface := v.ifMgr.GetByName(vppIfaceName)
	if iface == nil {
		return "", fmt.Errorf("interface %s not found", vppIfaceName)
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

		if reply.PhySwIfIndex == interface_types.InterfaceIndex(iface.SwIfIndex) && !found {
			hostIfName = reply.HostIfName
			found = true
		}
	}

	if !found {
		return "", fmt.Errorf("no LCP pair found for %s", vppIfaceName)
	}

	v.logger.Debug("Found LCP pair", "tap", hostIfName, "phy", vppIfaceName)
	return hostIfName, nil
}

func (v *VPP) SetVirtualMAC(mac string) error {
	parsedMAC, err := net.ParseMAC(mac)
	if err != nil {
		return fmt.Errorf("invalid virtual MAC: %w", err)
	}
	v.virtualMAC = parsedMAC
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

func (v *VPP) CreateLoopback(name string, ipv4 []string, ipv6 []string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	if iface := v.ifMgr.GetByName(name); iface != nil {
		v.logger.Debug("Loopback already exists in cache, reconciling IPs", "interface", name, "sw_if_index", iface.SwIfIndex)
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
			v.ifMgr.Add(&ifmgr.Interface{
				SwIfIndex:    uint32(msg.SwIfIndex),
				SupSwIfIndex: msg.SupSwIfIndex,
				Name:         name,
				DevType:      "loopback",
				Type:         ifmgr.IfTypeHardware,
				AdminUp:      true,
			})
			v.logger.Debug("Loopback already exists in VPP, reconciling IPs", "interface", name, "sw_if_index", msg.SwIfIndex)
			return v.reconcileLoopbackIPs(name, ipv4, ipv6)
		}
	}

	req := &interfaces.CreateLoopback{}
	reply := &interfaces.CreateLoopbackReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("create loopback: %w", err)
	}

	v.ifMgr.Add(&ifmgr.Interface{
		SwIfIndex:    uint32(reply.SwIfIndex),
		SupSwIfIndex: uint32(reply.SwIfIndex),
		Name:         name,
		DevType:      "loopback",
		Type:         ifmgr.IfTypeHardware,
		AdminUp:      true,
	})

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
		v.logger.Debug("Created linux-cp pair in default namespace", "interface", name)
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
		v.ifMgr.AddIPv4Address(uint32(reply.SwIfIndex), addr.IP)
		v.logger.Debug("Added IPv4 to Linux interface (nl plugin will sync to VPP)", "interface", name, "addr", normalizedCIDR)
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
			v.ifMgr.AddIPv6Address(uint32(reply.SwIfIndex), addr.IP)
			v.logger.Debug("Added IPv6 to Linux interface (nl plugin will sync to VPP)", "interface", name, "addr", normalizedCIDR)
		}
	}

	v.logger.Debug("Created loopback", "interface", name, "sw_if_index", reply.SwIfIndex)
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
				v.logger.Debug("Removed extra IP from Linux interface (not in config)", "interface", name, "addr", cidrStr)
				if iface := v.ifMgr.GetByName(name); iface != nil {
					if addr.IP.To4() != nil {
						v.ifMgr.RemoveIPv4Address(iface.SwIfIndex, addr.IP)
					} else {
						v.ifMgr.RemoveIPv6Address(iface.SwIfIndex, addr.IP)
					}
				}
			}
		}
	}

	iface := v.ifMgr.GetByName(name)
	if iface == nil {
		return fmt.Errorf("interface %s not in cache", name)
	}

	vppAddrs, err := v.getInterfaceAddresses(interface_types.InterfaceIndex(iface.SwIfIndex))
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
			v.logger.Debug("Added IPv4 to Linux interface", "interface", name, "addr", normalizedCIDR)
		}

		if !vppAddrs[normalizedCIDR] {
			ch, _ := v.conn.NewAPIChannel()
			if err := v.addIPAddress(ch, interface_types.InterfaceIndex(iface.SwIfIndex), normalizedCIDR, false); err != nil {
				v.logger.Warn("Failed to add IPv4 to VPP", "addr", normalizedCIDR, "error", err)
			} else {
				v.logger.Debug("Added IPv4 to VPP interface", "interface", name, "addr", normalizedCIDR)
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
				v.logger.Debug("Added IPv6 to Linux interface", "interface", name, "addr", normalizedCIDR)
			}
		}

		if !vppAddrs[normalizedCIDR] {
			ch, _ := v.conn.NewAPIChannel()
			if err := v.addIPAddress(ch, interface_types.InterfaceIndex(iface.SwIfIndex), normalizedCIDR, true); err != nil {
				v.logger.Warn("Failed to add IPv6 to VPP", "addr", normalizedCIDR, "error", err)
			} else {
				v.logger.Debug("Added IPv6 to VPP interface", "interface", name, "addr", normalizedCIDR)
			}
			ch.Close()
		}
	}

	v.logger.Debug("IP reconciliation complete", "interface", name, "ipv4_count", len(desiredIPv4), "ipv6_count", len(desiredIPv6))
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

func (v *VPP) RegisterPuntSocket(socketPath string, port uint16, ifaceName string) error {
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

	v.logger.Debug("Registering punt socket",
		"path", socketPath,
		"port", port,
		"interface", ifaceName,
		"sw_if_index", iface.SwIfIndex)

	reply := &punt.PuntSocketRegisterReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("register punt socket API call: %w", err)
	}

	v.logger.Debug("Punt socket register reply", "retval", reply.Retval, "pathname", reply.Pathname)

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
			v.logger.Debug("Found punt socket", "af", l4.Af, "proto", l4.Protocol, "port", l4.Port, "path", msg.Pathname)
			if l4.Port == port && l4.Protocol == ip_types.IP_API_PROTO_UDP && l4.Af == ip_types.ADDRESS_IP4 {
				found = true
			}
		}
	}

	if !found {
		v.logger.Warn("Punt socket registered successfully but not found in dump - VPP may not actually punt", "port", port)
	} else {
		v.logger.Debug("Verified punt socket registration", "port", port, "path", socketPath)
	}

	return nil
}

func (v *VPP) SetupCPEgressInterface(hostIfName, accessIface string) error {
	tapName, err := v.CreateTAP(hostIfName)
	if err != nil {
		return fmt.Errorf("create CP egress tap: %w", err)
	}

	v.logger.Debug("CP egress interface configured", "tap", tapName)
	return nil
}

func (v *VPP) EnableDirectedBroadcast(ifaceName string) error {
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

	req := &interfaces.SwInterfaceSetIPDirectedBroadcast{
		SwIfIndex: interface_types.InterfaceIndex(iface.SwIfIndex),
		Enable:    true,
	}

	reply := &interfaces.SwInterfaceSetIPDirectedBroadcastReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("enable directed broadcast API call: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("enable directed broadcast failed with retval: %d", reply.Retval)
	}

	v.logger.Debug("Enabled directed broadcast", "interface", ifaceName)
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
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return 0, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &interfaces.SwInterfaceGetTable{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		IsIPv6:    false,
	}

	reply := &interfaces.SwInterfaceGetTableReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return 0, fmt.Errorf("get interface table: %w", err)
	}

	if reply.Retval != 0 {
		return 0, fmt.Errorf("get interface table failed: retval=%d", reply.Retval)
	}

	return reply.VrfID, nil
}

func (v *VPP) EnableARPPunt(ifaceName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	req := &osvbng_punt.OsvbngPuntEnableDisable{
		SwIfIndex: interface_types.InterfaceIndex(idx),
		Protocol:  2,
		Enable:    true,
	}

	reply := &osvbng_punt.OsvbngPuntEnableDisableReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("enable arp punt: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("enable arp punt failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Enabled ARP punt", "interface", ifaceName, "sw_if_index", idx)
	return nil
}

func (v *VPP) EnableDHCPv4Punt(ifaceName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	req := &osvbng_punt.OsvbngPuntEnableDisable{
		SwIfIndex: interface_types.InterfaceIndex(idx),
		Protocol:  0,
		Enable:    true,
	}

	reply := &osvbng_punt.OsvbngPuntEnableDisableReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("enable dhcp punt: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("enable dhcp punt failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Enabled DHCP punt", "interface", ifaceName, "sw_if_index", idx)
	return nil
}

func (v *VPP) EnableDHCPv6Punt(ifaceName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	req := &osvbng_punt.OsvbngPuntEnableDisable{
		SwIfIndex: interface_types.InterfaceIndex(idx),
		Protocol:  1, // DHCPv6
		Enable:    true,
	}

	reply := &osvbng_punt.OsvbngPuntEnableDisableReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("enable dhcpv6 punt: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("enable dhcpv6 punt failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Enabled DHCPv6 punt", "interface", ifaceName, "sw_if_index", idx)
	return nil
}

func (v *VPP) EnableIPv6NDPunt(ifaceName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	req := &osvbng_punt.OsvbngPuntEnableDisable{
		SwIfIndex: interface_types.InterfaceIndex(idx),
		Protocol:  5, // IPv6 ND
		Enable:    true,
	}

	reply := &osvbng_punt.OsvbngPuntEnableDisableReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("enable ipv6 nd punt: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("enable ipv6 nd punt failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Enabled IPv6 ND punt", "interface", ifaceName, "sw_if_index", idx)
	return nil
}

func (v *VPP) EnableL2TPPunt(ifaceName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	req := &osvbng_punt.OsvbngPuntEnableDisable{
		SwIfIndex: interface_types.InterfaceIndex(idx),
		Protocol:  6, // L2TP
		Enable:    true,
	}

	reply := &osvbng_punt.OsvbngPuntEnableDisableReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("enable l2tp punt: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("enable l2tp punt failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Enabled L2TP punt", "interface", ifaceName, "sw_if_index", idx)
	return nil
}

type IPv6RAConfig struct {
	Managed        bool   // M flag
	Other          bool   // O flag
	RouterLifetime uint32
	MaxInterval    uint32
	MinInterval    uint32
}

type IPv6RAPrefixConfig struct {
	Prefix            string
	OnLink            bool // L flag
	Autonomous        bool // A flag
	ValidLifetime     uint32
	PreferredLifetime uint32
}

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

func (v *VPP) EnableIPv6ByIndex(swIfIndex uint32) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &ip.SwInterfaceIP6EnableDisable{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		Enable:    true,
	}

	reply := &ip.SwInterfaceIP6EnableDisableReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		var vppErr api.VPPApiError
		if errors.As(err, &vppErr) && vppErr == -81 {
			v.logger.Debug("IPv6 already enabled on interface", "sw_if_index", swIfIndex)
			return nil
		}
		return fmt.Errorf("enable ipv6: %w", err)
	}

	v.logger.Debug("Enabled IPv6 on interface", "sw_if_index", swIfIndex)
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

func (v *VPP) ConfigureIPv6RA(ifaceName string, config IPv6RAConfig) error {
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

func (v *VPP) AddIPv6RAPrefix(ifaceName string, config IPv6RAPrefixConfig) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	_, ipNet, err := net.ParseCIDR(config.Prefix)
	if err != nil {
		return fmt.Errorf("parse prefix: %w", err)
	}

	prefixLen, _ := ipNet.Mask.Size()
	var ip6Addr ip_types.IP6Address
	copy(ip6Addr[:], ipNet.IP.To16())

	req := &ip6_nd.SwInterfaceIP6ndRaPrefix{
		SwIfIndex: interface_types.InterfaceIndex(idx),
		Prefix: ip_types.Prefix{
			Address: ip_types.Address{
				Af: ip_types.ADDRESS_IP6,
				Un: ip_types.AddressUnionIP6(ip6Addr),
			},
			Len: uint8(prefixLen),
		},
		OffLink:      !config.OnLink,
		NoAutoconfig: !config.Autonomous,
		ValLifetime:  config.ValidLifetime,
		PrefLifetime: config.PreferredLifetime,
		IsNo:         false,
	}

	reply := &ip6_nd.SwInterfaceIP6ndRaPrefixReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("add ra prefix: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("add ra prefix failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Added IPv6 RA prefix", "interface", ifaceName,
		"prefix", config.Prefix, "onlink", config.OnLink, "autonomous", config.Autonomous)
	return nil
}

func (v *VPP) DisableIPv6RA(ifaceName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	req := &ip6_nd.SwInterfaceIP6ndRaConfig{
		SwIfIndex: interface_types.InterfaceIndex(idx),
		Suppress:  1,
		IsNo:      true,
	}

	reply := &ip6_nd.SwInterfaceIP6ndRaConfigReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("disable ipv6 ra: %w", err)
	}

	v.logger.Debug("Disabled IPv6 RA", "interface", ifaceName)
	return nil
}

func (v *VPP) ConfigureIPv6RAByIndex(swIfIndex uint32, config IPv6RAConfig) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

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
		SwIfIndex:       interface_types.InterfaceIndex(swIfIndex),
		Suppress:        0,
		Managed:         managed,
		Other:           other,
		LlOption:        1,
		SendUnicast:     1,
		Cease:           0,
		IsNo:            false,
		DefaultRouter:   1,
		MaxInterval:     config.MaxInterval,
		MinInterval:     config.MinInterval,
		Lifetime:        lifetime,
		InitialCount:    3,
		InitialInterval: 16,
	}

	reply := &ip6_nd.SwInterfaceIP6ndRaConfigReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("configure ipv6 ra: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("configure ipv6 ra failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Configured IPv6 RA", "sw_if_index", swIfIndex,
		"managed", config.Managed, "other", config.Other, "lifetime", lifetime)
	return nil
}

func (v *VPP) AddIPv6RAPrefixByIndex(swIfIndex uint32, config IPv6RAPrefixConfig) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	_, ipNet, err := net.ParseCIDR(config.Prefix)
	if err != nil {
		return fmt.Errorf("parse prefix: %w", err)
	}

	prefixLen, _ := ipNet.Mask.Size()
	var ip6Addr ip_types.IP6Address
	copy(ip6Addr[:], ipNet.IP.To16())

	req := &ip6_nd.SwInterfaceIP6ndRaPrefix{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		Prefix: ip_types.Prefix{
			Address: ip_types.Address{
				Af: ip_types.ADDRESS_IP6,
				Un: ip_types.AddressUnionIP6(ip6Addr),
			},
			Len: uint8(prefixLen),
		},
		OffLink:      !config.OnLink,
		NoAutoconfig: !config.Autonomous,
		ValLifetime:  config.ValidLifetime,
		PrefLifetime: config.PreferredLifetime,
		IsNo:         false,
	}

	reply := &ip6_nd.SwInterfaceIP6ndRaPrefixReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("add ra prefix: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("add ra prefix failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Added IPv6 RA prefix", "sw_if_index", swIfIndex,
		"prefix", config.Prefix, "onlink", config.OnLink, "autonomous", config.Autonomous)
	return nil
}

func (v *VPP) EnablePPPoEPunt(ifaceName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	for _, protocol := range []uint8{3, 4} {
		req := &osvbng_punt.OsvbngPuntEnableDisable{
			SwIfIndex: interface_types.InterfaceIndex(idx),
			Protocol:  protocol,
			Enable:    true,
		}

		reply := &osvbng_punt.OsvbngPuntEnableDisableReply{}
		if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
			return fmt.Errorf("enable pppoe punt (protocol %d): %w", protocol, err)
		}

		if reply.Retval != 0 {
			return fmt.Errorf("enable pppoe punt (protocol %d) failed: retval=%d", protocol, reply.Retval)
		}
	}

	v.logger.Debug("Enabled PPPoE punt", "interface", ifaceName, "sw_if_index", idx)
	return nil
}

func (v *VPP) AddPPPoESession(sessionID uint16, clientIP net.IP, clientMAC net.HardwareAddr, localMAC net.HardwareAddr, encapIfIndex uint32, outerVLAN uint16, innerVLAN uint16, decapVrfID uint32) (uint32, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return 0, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	clientAddr, err := v.toAddress(clientIP)
	if err != nil {
		return 0, fmt.Errorf("convert client IP: %w", err)
	}

	var clientMacAddr ethernet_types.MacAddress
	copy(clientMacAddr[:], clientMAC)

	var localMacAddr ethernet_types.MacAddress
	copy(localMacAddr[:], localMAC)

	req := &osvbng_pppoe.OsvbngPppoeAddDelSession{
		IsAdd:        true,
		SessionID:    sessionID,
		ClientIP:     clientAddr,
		DecapVrfID:   decapVrfID,
		ClientMac:    clientMacAddr,
		LocalMac:     localMacAddr,
		EncapIfIndex: interface_types.InterfaceIndex(encapIfIndex),
		OuterVlan:    outerVLAN,
		InnerVlan:    innerVLAN,
	}

	reply := &osvbng_pppoe.OsvbngPppoeAddDelSessionReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return 0, fmt.Errorf("add pppoe session: %w", err)
	}

	if reply.Retval != 0 {
		return 0, fmt.Errorf("add pppoe session failed: retval=%d", reply.Retval)
	}

	swIdx := uint32(reply.SwIfIndex)
	v.ifMgr.Add(&ifmgr.Interface{
		SwIfIndex:    swIdx,
		SupSwIfIndex: encapIfIndex,
		Name:         fmt.Sprintf("pppoe-session-%d", sessionID),
		Type:         ifmgr.IfTypeP2P,
		AdminUp:      true,
		FIBTableID:   decapVrfID,
	})

	v.logger.Debug("Added PPPoE session to VPP",
		"session_id", sessionID,
		"client_ip", clientIP.String(),
		"client_mac", clientMAC.String(),
		"local_mac", localMAC.String(),
		"encap_if_index", encapIfIndex,
		"outer_vlan", outerVLAN,
		"inner_vlan", innerVLAN,
		"sw_if_index", reply.SwIfIndex)

	return swIdx, nil
}

func (v *VPP) DeletePPPoESession(sessionID uint16, clientIP net.IP, clientMAC net.HardwareAddr) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	clientAddr, err := v.toAddress(clientIP)
	if err != nil {
		return fmt.Errorf("convert client IP: %w", err)
	}

	var clientMacAddr ethernet_types.MacAddress
	copy(clientMacAddr[:], clientMAC)

	// For delete, VPP only uses (client_mac, session_id) for bihash lookup
	// and client_ip for FIB removal. Other fields are ignored.
	req := &osvbng_pppoe.OsvbngPppoeAddDelSession{
		IsAdd:     false,
		SessionID: sessionID,
		ClientIP:  clientAddr,
		ClientMac: clientMacAddr,
	}

	reply := &osvbng_pppoe.OsvbngPppoeAddDelSessionReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("delete pppoe session: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("delete pppoe session failed: retval=%d", reply.Retval)
	}

	v.ifMgr.Remove(uint32(reply.SwIfIndex))

	v.logger.Debug("Deleted PPPoE session from VPP",
		"session_id", sessionID,
		"client_ip", clientIP.String())

	return nil
}

func (v *VPP) IPoEEnableDisable(ifaceName string, enable bool) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	idx, err := v.GetInterfaceIndex(ifaceName)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	req := &osvbng_ipoe.OsvbngIpoeEnableDisable{
		SwIfIndex: interface_types.InterfaceIndex(idx),
		Enable:    enable,
	}

	reply := &osvbng_ipoe.OsvbngIpoeEnableDisableReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("ipoe enable/disable: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("ipoe enable/disable failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("IPoE enable/disable", "interface", ifaceName, "sw_if_index", idx, "enable", enable)
	return nil
}

func (v *VPP) AddIPoESession(clientMAC net.HardwareAddr, localMAC net.HardwareAddr, encapIfIndex uint32, outerVLAN uint16, innerVLAN uint16, decapVrfID uint32) (uint32, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return 0, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	var clientMacAddr ethernet_types.MacAddress
	copy(clientMacAddr[:], clientMAC)

	var localMacAddr ethernet_types.MacAddress
	copy(localMacAddr[:], localMAC)

	req := &osvbng_ipoe.OsvbngIpoeAddDelSession{
		IsAdd:        true,
		EncapIfIndex: interface_types.InterfaceIndex(encapIfIndex),
		ClientMac:    clientMacAddr,
		LocalMac:     localMacAddr,
		OuterVlan:    outerVLAN,
		InnerVlan:    innerVLAN,
		DecapVrfID:   decapVrfID,
	}

	reply := &osvbng_ipoe.OsvbngIpoeAddDelSessionReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return 0, fmt.Errorf("add ipoe session: %w", err)
	}

	if reply.Retval != 0 {
		return 0, fmt.Errorf("add ipoe session failed: retval=%d", reply.Retval)
	}

	swIdx := uint32(reply.SwIfIndex)
	v.ifMgr.Add(&ifmgr.Interface{
		SwIfIndex:    swIdx,
		SupSwIfIndex: encapIfIndex,
		Name:         fmt.Sprintf("ipoe-session-%s", clientMAC.String()),
		Type:         ifmgr.IfTypeP2P,
		AdminUp:      true,
		FIBTableID:   decapVrfID,
	})

	v.logger.Debug("Added IPoE session",
		"client_mac", clientMAC.String(),
		"local_mac", localMAC.String(),
		"encap_if_index", encapIfIndex,
		"outer_vlan", outerVLAN,
		"inner_vlan", innerVLAN,
		"sw_if_index", reply.SwIfIndex)

	return swIdx, nil
}

func (v *VPP) DeleteIPoESession(clientMAC net.HardwareAddr, encapIfIndex uint32, innerVLAN uint16) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	var clientMacAddr ethernet_types.MacAddress
	copy(clientMacAddr[:], clientMAC)

	req := &osvbng_ipoe.OsvbngIpoeAddDelSession{
		IsAdd:        false,
		EncapIfIndex: interface_types.InterfaceIndex(encapIfIndex),
		ClientMac:    clientMacAddr,
		InnerVlan:    innerVLAN,
	}

	reply := &osvbng_ipoe.OsvbngIpoeAddDelSessionReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("delete ipoe session: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("delete ipoe session failed: retval=%d", reply.Retval)
	}

	v.ifMgr.Remove(uint32(reply.SwIfIndex))

	v.logger.Debug("Deleted IPoE session", "client_mac", clientMAC.String(), "encap_if_index", encapIfIndex, "inner_vlan", innerVLAN)
	return nil
}

func (v *VPP) IPoESetSessionIPv4(swIfIndex uint32, clientIP net.IP, isAdd bool) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	ip4 := clientIP.To4()
	if ip4 == nil {
		return fmt.Errorf("invalid IPv4 address: %s", clientIP)
	}

	var clientAddr ip_types.IP4Address
	copy(clientAddr[:], ip4)

	req := &osvbng_ipoe.OsvbngIpoeSetSessionIPv4{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		ClientIP:  clientAddr,
		IsAdd:     isAdd,
	}

	reply := &osvbng_ipoe.OsvbngIpoeSetSessionIPv4Reply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("set ipoe session ipv4: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("set ipoe session ipv4 failed: retval=%d", reply.Retval)
	}

	if isAdd {
		v.ifMgr.AddIPv4Address(swIfIndex, clientIP)
	} else {
		v.ifMgr.RemoveIPv4Address(swIfIndex, clientIP)
	}

	v.logger.Debug("Set IPoE session IPv4", "sw_if_index", swIfIndex, "client_ip", clientIP.String(), "is_add", isAdd)
	return nil
}

func (v *VPP) IPoESetSessionIPv6(swIfIndex uint32, clientIP net.IP, isAdd bool) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	ip6 := clientIP.To16()
	if ip6 == nil || clientIP.To4() != nil {
		return fmt.Errorf("invalid IPv6 address: %s", clientIP)
	}

	var clientAddr ip_types.IP6Address
	copy(clientAddr[:], ip6)

	req := &osvbng_ipoe.OsvbngIpoeSetSessionIPv6{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		ClientIP:  clientAddr,
		IsAdd:     isAdd,
	}

	reply := &osvbng_ipoe.OsvbngIpoeSetSessionIPv6Reply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("set ipoe session ipv6: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("set ipoe session ipv6 failed: retval=%d", reply.Retval)
	}

	if isAdd {
		v.ifMgr.AddIPv6Address(swIfIndex, clientIP)
	} else {
		v.ifMgr.RemoveIPv6Address(swIfIndex, clientIP)
	}

	v.logger.Debug("Set IPoE session IPv6", "sw_if_index", swIfIndex, "client_ip", clientIP.String(), "is_add", isAdd)
	return nil
}

func (v *VPP) IPoESetDelegatedPrefix(swIfIndex uint32, prefix net.IPNet, nextHop net.IP, isAdd bool) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	prefixLen, _ := prefix.Mask.Size()
	ip6 := prefix.IP.To16()
	if ip6 == nil || prefix.IP.To4() != nil {
		return fmt.Errorf("invalid IPv6 prefix: %s", prefix.String())
	}

	var prefixAddr ip_types.IP6Address
	copy(prefixAddr[:], ip6)

	var nextHopAddr ip_types.IP6Address
	if nextHop != nil {
		nh6 := nextHop.To16()
		if nh6 == nil {
			return fmt.Errorf("invalid IPv6 next-hop: %s", nextHop)
		}
		copy(nextHopAddr[:], nh6)
	}

	req := &osvbng_ipoe.OsvbngIpoeSetDelegatedPrefix{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		Prefix: ip_types.Prefix{
			Address: ip_types.Address{
				Af: ip_types.ADDRESS_IP6,
				Un: ip_types.AddressUnionIP6(prefixAddr),
			},
			Len: uint8(prefixLen),
		},
		NextHop: nextHopAddr,
		IsAdd:   isAdd,
	}

	reply := &osvbng_ipoe.OsvbngIpoeSetDelegatedPrefixReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("set ipoe delegated prefix: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("set ipoe delegated prefix failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Set IPoE delegated prefix", "sw_if_index", swIfIndex, "prefix", prefix.String(), "next_hop", nextHop, "is_add", isAdd)
	return nil
}

func (v *VPP) AddIPoESessionAsync(clientMAC net.HardwareAddr, localMAC net.HardwareAddr, encapIfIndex uint32, outerVLAN uint16, innerVLAN uint16, decapVrfID uint32, callback func(uint32, error)) {
	var clientMacAddr ethernet_types.MacAddress
	copy(clientMacAddr[:], clientMAC)

	var localMacAddr ethernet_types.MacAddress
	copy(localMacAddr[:], localMAC)

	req := &osvbng_ipoe.OsvbngIpoeAddDelSession{
		IsAdd:        true,
		EncapIfIndex: interface_types.InterfaceIndex(encapIfIndex),
		ClientMac:    clientMacAddr,
		LocalMac:     localMacAddr,
		OuterVlan:    outerVLAN,
		InnerVlan:    innerVLAN,
		DecapVrfID:   decapVrfID,
	}

	v.asyncWorker.SendAsync(req, func(reply api.Message, err error) {
		if err != nil {
			callback(0, err)
			return
		}
		r, ok := reply.(*osvbng_ipoe.OsvbngIpoeAddDelSessionReply)
		if !ok {
			callback(0, fmt.Errorf("unexpected reply type: %T", reply))
			return
		}
		if r.Retval != 0 {
			callback(0, fmt.Errorf("VPP error: retval=%d", r.Retval))
			return
		}
		swIdx := uint32(r.SwIfIndex)
		v.ifMgr.Add(&ifmgr.Interface{
			SwIfIndex:    swIdx,
			SupSwIfIndex: encapIfIndex,
			Name:         fmt.Sprintf("ipoe-session-%s", clientMAC.String()),
			Type:         ifmgr.IfTypeP2P,
			AdminUp:      true,
			FIBTableID:   decapVrfID,
		})
		v.logger.Debug("Added IPoE session (async)",
			"client_mac", clientMAC.String(),
			"local_mac", localMAC.String(),
			"encap_if_index", encapIfIndex,
			"outer_vlan", outerVLAN,
			"inner_vlan", innerVLAN,
			"sw_if_index", r.SwIfIndex)
		callback(swIdx, nil)
	})
}

func (v *VPP) DeleteIPoESessionAsync(clientMAC net.HardwareAddr, encapIfIndex uint32, innerVLAN uint16, callback func(error)) {
	var clientMacAddr ethernet_types.MacAddress
	copy(clientMacAddr[:], clientMAC)

	req := &osvbng_ipoe.OsvbngIpoeAddDelSession{
		IsAdd:        false,
		EncapIfIndex: interface_types.InterfaceIndex(encapIfIndex),
		ClientMac:    clientMacAddr,
		InnerVlan:    innerVLAN,
	}

	v.asyncWorker.SendAsync(req, func(reply api.Message, err error) {
		if err != nil {
			callback(err)
			return
		}
		r, ok := reply.(*osvbng_ipoe.OsvbngIpoeAddDelSessionReply)
		if !ok {
			callback(fmt.Errorf("unexpected reply type: %T", reply))
			return
		}
		if r.Retval != 0 {
			callback(fmt.Errorf("VPP error: retval=%d", r.Retval))
			return
		}
		v.ifMgr.Remove(uint32(r.SwIfIndex))
		v.logger.Debug("Deleted IPoE session (async)", "client_mac", clientMAC.String(), "encap_if_index", encapIfIndex, "inner_vlan", innerVLAN)
		callback(nil)
	})
}

func (v *VPP) IPoESetSessionIPv4Async(swIfIndex uint32, clientIP net.IP, isAdd bool, callback func(error)) {
	ip4 := clientIP.To4()
	if ip4 == nil {
		callback(fmt.Errorf("invalid IPv4 address: %s", clientIP))
		return
	}

	var clientAddr ip_types.IP4Address
	copy(clientAddr[:], ip4)

	req := &osvbng_ipoe.OsvbngIpoeSetSessionIPv4{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		ClientIP:  clientAddr,
		IsAdd:     isAdd,
	}

	v.asyncWorker.SendAsync(req, func(reply api.Message, err error) {
		if err != nil {
			callback(err)
			return
		}
		r, ok := reply.(*osvbng_ipoe.OsvbngIpoeSetSessionIPv4Reply)
		if !ok {
			callback(fmt.Errorf("unexpected reply type: %T", reply))
			return
		}
		if r.Retval != 0 {
			callback(fmt.Errorf("VPP error: retval=%d", r.Retval))
			return
		}
		if isAdd {
			v.ifMgr.AddIPv4Address(swIfIndex, clientIP)
		} else {
			v.ifMgr.RemoveIPv4Address(swIfIndex, clientIP)
		}
		v.logger.Debug("Set IPoE session IPv4 (async)", "sw_if_index", swIfIndex, "client_ip", clientIP.String(), "is_add", isAdd)
		callback(nil)
	})
}

func (v *VPP) IPoESetSessionIPv6Async(swIfIndex uint32, clientIP net.IP, isAdd bool, callback func(error)) {
	ip6 := clientIP.To16()
	if ip6 == nil || clientIP.To4() != nil {
		callback(fmt.Errorf("invalid IPv6 address: %s", clientIP))
		return
	}

	var clientAddr ip_types.IP6Address
	copy(clientAddr[:], ip6)

	req := &osvbng_ipoe.OsvbngIpoeSetSessionIPv6{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		ClientIP:  clientAddr,
		IsAdd:     isAdd,
	}

	v.asyncWorker.SendAsync(req, func(reply api.Message, err error) {
		if err != nil {
			callback(err)
			return
		}
		r, ok := reply.(*osvbng_ipoe.OsvbngIpoeSetSessionIPv6Reply)
		if !ok {
			callback(fmt.Errorf("unexpected reply type: %T", reply))
			return
		}
		if r.Retval != 0 {
			callback(fmt.Errorf("VPP error: retval=%d", r.Retval))
			return
		}
		if isAdd {
			v.ifMgr.AddIPv6Address(swIfIndex, clientIP)
		} else {
			v.ifMgr.RemoveIPv6Address(swIfIndex, clientIP)
		}
		v.logger.Debug("Set IPoE session IPv6 (async)", "sw_if_index", swIfIndex, "client_ip", clientIP.String(), "is_add", isAdd)
		callback(nil)
	})
}

func (v *VPP) IPoESetDelegatedPrefixAsync(swIfIndex uint32, prefix net.IPNet, nextHop net.IP, isAdd bool, callback func(error)) {
	prefixLen, _ := prefix.Mask.Size()
	ip6 := prefix.IP.To16()
	if ip6 == nil || prefix.IP.To4() != nil {
		callback(fmt.Errorf("invalid IPv6 prefix: %s", prefix.String()))
		return
	}

	var prefixAddr ip_types.IP6Address
	copy(prefixAddr[:], ip6)

	var nextHopAddr ip_types.IP6Address
	if nextHop != nil {
		nh6 := nextHop.To16()
		if nh6 == nil {
			callback(fmt.Errorf("invalid IPv6 next-hop: %s", nextHop))
			return
		}
		copy(nextHopAddr[:], nh6)
	}

	req := &osvbng_ipoe.OsvbngIpoeSetDelegatedPrefix{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		Prefix: ip_types.Prefix{
			Address: ip_types.Address{
				Af: ip_types.ADDRESS_IP6,
				Un: ip_types.AddressUnionIP6(prefixAddr),
			},
			Len: uint8(prefixLen),
		},
		NextHop: nextHopAddr,
		IsAdd:   isAdd,
	}

	v.asyncWorker.SendAsync(req, func(reply api.Message, err error) {
		if err != nil {
			callback(err)
			return
		}
		r, ok := reply.(*osvbng_ipoe.OsvbngIpoeSetDelegatedPrefixReply)
		if !ok {
			callback(fmt.Errorf("unexpected reply type: %T", reply))
			return
		}
		if r.Retval != 0 {
			callback(fmt.Errorf("VPP error: retval=%d", r.Retval))
			return
		}
		v.logger.Debug("Set IPoE delegated prefix (async)", "sw_if_index", swIfIndex, "prefix", prefix.String(), "next_hop", nextHop, "is_add", isAdd)
		callback(nil)
	})
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

	v.logger.Debug("Disabled ARP reply", "interface", ifaceName, "sw_if_index", idx)
	return nil
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

	v.logger.Debug("Created LCP pair", "vpp_iface", vppIface, "linux_iface", linuxIface)
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
	memifName := fmt.Sprintf("memif%d/%d", socketID, memifID)

	if idx, err := v.GetInterfaceIndex(memifName); err == nil {
		v.logger.Debug("Memif already exists in VPP", "name", memifName, "sw_if_index", idx)
		return nil
	}

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
	v.logger.Debug("Created memif socket", "path", socketPath, "socket_id", socketID)

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

	v.ifMgr.Add(&ifmgr.Interface{
		SwIfIndex:    uint32(memifReply.SwIfIndex),
		SupSwIfIndex: uint32(memifReply.SwIfIndex),
		Name:         memifName,
		DevType:      "memif",
		Type:         ifmgr.IfTypeHardware,
		AdminUp:      true,
	})
	v.logger.Debug("Created memif interface", "id", memifID, "name", memifName, "sw_if_index", memifReply.SwIfIndex)

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

	if accessIfaceInfo := v.ifMgr.GetByName(accessIface); accessIfaceInfo != nil {
		setAccessUpReq := &interfaces.SwInterfaceSetFlags{
			SwIfIndex: interface_types.InterfaceIndex(accessIfaceInfo.SwIfIndex),
			Flags:     interface_types.IF_STATUS_API_FLAG_ADMIN_UP,
		}
		setAccessUpReply := &interfaces.SwInterfaceSetFlagsReply{}
		if err := ch.SendRequest(setAccessUpReq).ReceiveReply(setAccessUpReply); err != nil {
			return fmt.Errorf("set access interface up: %w", err)
		}
		v.logger.Debug("Set access interface up", "interface", accessIface)
	}

	v.logger.Debug("Setup memif dataplane", "memif", memifName, "access_iface", accessIface)
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

func (v *VPP) AddIPTable(tableID uint32, isIPv6 bool, name string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &ip.IPTableAddDel{
		IsAdd: true,
		Table: ip.IPTable{
			TableID: tableID,
			IsIP6:   isIPv6,
			Name:    name,
		},
	}

	reply := &ip.IPTableAddDelReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("add IP table %d (ipv6=%v): %w", tableID, isIPv6, err)
	}

	return nil
}

func (v *VPP) DelIPTable(tableID uint32, isIPv6 bool) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &ip.IPTableAddDel{
		IsAdd: false,
		Table: ip.IPTable{
			TableID: tableID,
			IsIP6:   isIPv6,
		},
	}

	reply := &ip.IPTableAddDelReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("delete IP table %d (ipv6=%v): %w", tableID, isIPv6, err)
	}

	return nil
}

func (v *VPP) AddPPPoESessionAsync(sessionID uint16, clientIP net.IP, clientMAC net.HardwareAddr, localMAC net.HardwareAddr, encapIfIndex uint32, outerVLAN uint16, innerVLAN uint16, decapVrfID uint32, callback func(uint32, error)) {
	clientAddr, err := v.toAddress(clientIP)
	if err != nil {
		callback(0, fmt.Errorf("convert client IP: %w", err))
		return
	}

	var clientMacAddr ethernet_types.MacAddress
	copy(clientMacAddr[:], clientMAC)

	var localMacAddr ethernet_types.MacAddress
	copy(localMacAddr[:], localMAC)

	req := &osvbng_pppoe.OsvbngPppoeAddDelSession{
		IsAdd:        true,
		SessionID:    sessionID,
		ClientIP:     clientAddr,
		DecapVrfID:   decapVrfID,
		ClientMac:    clientMacAddr,
		LocalMac:     localMacAddr,
		EncapIfIndex: interface_types.InterfaceIndex(encapIfIndex),
		OuterVlan:    outerVLAN,
		InnerVlan:    innerVLAN,
	}

	v.asyncWorker.SendAsync(req, func(reply api.Message, err error) {
		if err != nil {
			callback(0, err)
			return
		}
		r, ok := reply.(*osvbng_pppoe.OsvbngPppoeAddDelSessionReply)
		if !ok {
			callback(0, fmt.Errorf("unexpected reply type: %T", reply))
			return
		}
		if r.Retval != 0 {
			callback(0, fmt.Errorf("VPP error: retval=%d", r.Retval))
			return
		}
		swIdx := uint32(r.SwIfIndex)
		v.ifMgr.Add(&ifmgr.Interface{
			SwIfIndex:    swIdx,
			SupSwIfIndex: encapIfIndex,
			Name:         fmt.Sprintf("pppoe-session-%d", sessionID),
			Type:         ifmgr.IfTypeP2P,
			AdminUp:      true,
			FIBTableID:   decapVrfID,
		})
		v.logger.Debug("Added PPPoE session to VPP (async)",
			"session_id", sessionID,
			"client_ip", clientIP.String(),
			"sw_if_index", r.SwIfIndex)
		callback(swIdx, nil)
	})
}

func (v *VPP) DeletePPPoESessionAsync(sessionID uint16, clientIP net.IP, clientMAC net.HardwareAddr, callback func(error)) {
	clientAddr, err := v.toAddress(clientIP)
	if err != nil {
		callback(fmt.Errorf("convert client IP: %w", err))
		return
	}

	var clientMacAddr ethernet_types.MacAddress
	copy(clientMacAddr[:], clientMAC)

	req := &osvbng_pppoe.OsvbngPppoeAddDelSession{
		IsAdd:     false,
		SessionID: sessionID,
		ClientIP:  clientAddr,
		ClientMac: clientMacAddr,
	}

	v.asyncWorker.SendAsync(req, func(reply api.Message, err error) {
		if err != nil {
			callback(err)
			return
		}
		r, ok := reply.(*osvbng_pppoe.OsvbngPppoeAddDelSessionReply)
		if !ok {
			callback(fmt.Errorf("unexpected reply type: %T", reply))
			return
		}
		if r.Retval != 0 {
			callback(fmt.Errorf("VPP error: retval=%d", r.Retval))
			return
		}
		v.ifMgr.Remove(uint32(r.SwIfIndex))
		v.logger.Debug("Deleted PPPoE session from VPP (async)",
			"session_id", sessionID,
			"client_ip", clientIP.String())
		callback(nil)
	})
}

func (v *VPP) AddAdjacencyWithRewriteAsync(ipAddr string, swIfIndex uint32, rewrite []byte, callback func(adjIndex uint32, err error)) {
	ip := net.ParseIP(ipAddr)
	if ip == nil {
		callback(0, fmt.Errorf("invalid IP address: %s", ipAddr))
		return
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

	req := &fib_control.FibControlAdjAddRewrite{
		SwIfIndex:  interface_types.InterfaceIndex(swIfIndex),
		NhAddr:     addr,
		LinkType:   linkType,
		RewriteLen: uint8(len(rewrite)),
		Rewrite:    make([]byte, 128),
	}
	copy(req.Rewrite, rewrite)

	v.asyncWorker.SendAsync(req, func(reply api.Message, err error) {
		if err != nil {
			callback(0, err)
			return
		}
		r := reply.(*fib_control.FibControlAdjAddRewriteReply)
		if r.Retval != 0 {
			callback(0, fmt.Errorf("add adjacency failed with retval: %d", r.Retval))
			return
		}
		v.logger.Debug("VPP adjacency created (async)", "adj_index", r.AdjIndex)
		callback(r.AdjIndex, nil)
	})
}

func (v *VPP) AddHostRouteAsync(ipAddr string, adjIndex uint32, fibID uint32, swIfIndex uint32, callback func(error)) {
	ip := net.ParseIP(ipAddr)
	if ip == nil {
		callback(fmt.Errorf("invalid IP address: %s", ipAddr))
		return
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

	req := &fib_control.FibControlAddHostRoute{
		TableID:   fibID,
		Prefix:    prefix,
		AdjIndex:  adjIndex,
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
	}

	v.asyncWorker.SendAsync(req, func(reply api.Message, err error) {
		if err != nil {
			callback(err)
			return
		}
		r := reply.(*fib_control.FibControlAddHostRouteReply)
		if r.Retval != 0 {
			callback(fmt.Errorf("add host route failed with retval: %d", r.Retval))
			return
		}
		v.logger.Debug("VPP host route added (async)", "ip", ipAddr)
		callback(nil)
	})
}

func (v *VPP) DeleteHostRouteAsync(ipAddr string, fibID uint32, callback func(error)) {
	ip := net.ParseIP(ipAddr)
	if ip == nil {
		callback(fmt.Errorf("invalid IP address: %s", ipAddr))
		return
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

	req := &fib_control.FibControlDelHostRoute{
		TableID: fibID,
		Prefix:  prefix,
	}

	v.asyncWorker.SendAsync(req, func(reply api.Message, err error) {
		if err != nil {
			callback(err)
			return
		}
		r := reply.(*fib_control.FibControlDelHostRouteReply)
		if r.Retval != 0 {
			callback(fmt.Errorf("delete host route failed with retval: %d", r.Retval))
			return
		}
		v.logger.Debug("VPP host route deleted (async)", "ip", ipAddr)
		callback(nil)
	})
}

type InterfaceInfo struct {
	SwIfIndex    uint32
	Name         string
	AdminUp      bool
	LinkUp       bool
	MTU          uint32
	OuterVlanID  uint16
	InnerVlanID  uint16
	SupSwIfIndex uint32
}

type IPAddressInfo struct {
	SwIfIndex uint32
	Address   string
	IsIPv6    bool
}

type UnnumberedInfo struct {
	SwIfIndex   uint32
	IPSwIfIndex uint32
}

type IPv6RAInfo struct {
	SwIfIndex          uint32
	Managed            bool
	Other              bool
	RouterLifetimeSecs uint16
	MaxIntervalSecs    float64
	MinIntervalSecs    float64
	SendRadv           bool
}

type PuntRegistration struct {
	SwIfIndex uint32
	Protocol  uint8
}

type PuntStats struct {
	Protocol       uint8
	PacketsPunted  uint64
	PacketsDropped uint64
	PacketsPoliced uint64
	PolicerRate    float64
	PolicerBurst   uint32
}

type MrouteInfo struct {
	TableID    uint32
	GrpAddress net.IP
	SrcAddress net.IP
	IsIPv6     bool
}

func (v *VPP) DumpInterfaces() ([]InterfaceInfo, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, err
	}
	defer ch.Close()

	req := &interfaces.SwInterfaceDump{
		SwIfIndex: interface_types.InterfaceIndex(^uint32(0)),
	}

	stream := ch.SendMultiRequest(req)
	var result []InterfaceInfo

	for {
		reply := &interfaces.SwInterfaceDetails{}
		stop, err := stream.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("dump interfaces: %w", err)
		}

		info := InterfaceInfo{
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
	addrs, err := v.DumpIPAddresses()
	if err != nil {
		return fmt.Errorf("dump IP addresses: %w", err)
	}

	seenIndices := make(map[uint32]bool)
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
		seenIndices[info.SwIfIndex] = true
	}

	for idx := range seenIndices {
		tableID, err := v.GetFIBIDForInterface(idx)
		if err != nil {
			v.logger.Warn("Failed to get FIB table for interface", "sw_if_index", idx, "error", err)
			continue
		}
		v.ifMgr.SetFIBTableID(idx, tableID)
	}

	v.logger.Debug("Loaded IP state into ifMgr", "addresses", len(addrs), "interfaces", len(seenIndices))
	return nil
}

func (v *VPP) GetIfMgr() *ifmgr.Manager {
	return v.ifMgr
}

func isZeroMAC(mac []byte) bool {
	for _, b := range mac {
		if b != 0 {
			return false
		}
	}
	return true
}

func (v *VPP) DumpIPAddresses() ([]IPAddressInfo, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, err
	}
	defer ch.Close()

	var result []IPAddressInfo
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
		result = append(result, IPAddressInfo{
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
		result = append(result, IPAddressInfo{
			SwIfIndex: uint32(reply.SwIfIndex),
			Address:   addr,
			IsIPv6:    true,
		})
	}

	v.logger.Debug("Dumped IP addresses", "count", len(result))
	return result, nil
}

func (v *VPP) DumpUnnumbered() ([]UnnumberedInfo, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, err
	}
	defer ch.Close()

	req := &ip.IPUnnumberedDump{
		SwIfIndex: interface_types.InterfaceIndex(^uint32(0)),
	}

	stream := ch.SendMultiRequest(req)
	var result []UnnumberedInfo

	for {
		reply := &ip.IPUnnumberedDetails{}
		stop, err := stream.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("dump unnumbered: %w", err)
		}

		info := UnnumberedInfo{
			SwIfIndex:   uint32(reply.SwIfIndex),
			IPSwIfIndex: uint32(reply.IPSwIfIndex),
		}
		result = append(result, info)
	}

	v.logger.Debug("Dumped unnumbered", "count", len(result))
	return result, nil
}

func (v *VPP) IsIPv6EnabledByIndex(swIfIndex uint32) bool {
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
		if v.IsIPv6EnabledByIndex(iface.SwIfIndex) {
			result = append(result, iface.SwIfIndex)
		}
	}

	v.logger.Debug("Dumped IPv6 enabled interfaces", "count", len(result))
	return result, nil
}

func (v *VPP) DumpIPv6RA() ([]IPv6RAInfo, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, err
	}
	defer ch.Close()

	req := &ip6_nd.SwInterfaceIP6ndRaDump{
		SwIfIndex: interface_types.InterfaceIndex(^uint32(0)),
	}

	stream := ch.SendMultiRequest(req)
	var result []IPv6RAInfo

	for {
		reply := &ip6_nd.SwInterfaceIP6ndRaDetails{}
		stop, err := stream.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("dump ipv6 ra: %w", err)
		}

		info := IPv6RAInfo{
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

func (v *VPP) DumpPuntRegistrations() ([]PuntRegistration, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, err
	}
	defer ch.Close()

	req := &osvbng_punt.OsvbngPuntRegistrationDump{}

	stream := ch.SendMultiRequest(req)
	var result []PuntRegistration

	for {
		reply := &osvbng_punt.OsvbngPuntRegistrationDetails{}
		stop, err := stream.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("dump punt registrations: %w", err)
		}

		info := PuntRegistration{
			SwIfIndex: uint32(reply.SwIfIndex),
			Protocol:  reply.Protocol,
		}
		result = append(result, info)
	}

	v.logger.Debug("Dumped punt registrations", "count", len(result))
	return result, nil
}

func (v *VPP) GetPuntStats() ([]PuntStats, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, err
	}
	defer ch.Close()

	req := &osvbng_punt.OsvbngPuntStatsDump{}
	stream := ch.SendMultiRequest(req)
	var result []PuntStats

	for {
		reply := &osvbng_punt.OsvbngPuntStatsDetails{}
		stop, err := stream.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("dump punt stats: %w", err)
		}

		stats := PuntStats{
			Protocol:       reply.Protocol,
			PacketsPunted:  reply.PacketsPunted,
			PacketsDropped: reply.PacketsDropped,
			PacketsPoliced: reply.PacketsPoliced,
			PolicerRate:    reply.PolicerRate,
			PolicerBurst:   reply.PolicerBurst,
		}
		result = append(result, stats)
	}

	return result, nil
}

func (v *VPP) ConfigurePuntPolicer(protocol uint8, rate float64, burst uint32) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}
	defer ch.Close()

	req := &osvbng_punt.OsvbngPuntPolicerConfigure{
		Protocol: protocol,
		Rate:     rate,
		Burst:    burst,
	}

	reply := &osvbng_punt.OsvbngPuntPolicerConfigureReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("configure punt policer: %w", err)
	}

	if reply.Retval != 0 {
		return fmt.Errorf("configure punt policer failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Configured punt policer", "protocol", protocol, "rate", rate, "burst", burst)
	return nil
}

func (v *VPP) DumpMroutes() ([]MrouteInfo, error) {
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
	var result []MrouteInfo

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

		info := MrouteInfo{
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
