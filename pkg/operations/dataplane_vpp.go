package operations

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/af_packet"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ethernet_types"
	vppinterfaces "github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/lcp"
	"github.com/vishvananda/netlink"
	"go.fd.io/govpp/core"
)

type VPPDataplane struct {
	conn        *core.Connection
	ifaceCache  map[string]interface_types.InterfaceIndex
	logger      *slog.Logger
	vrfResolver func(string) (uint32, error)
	ifMgr       *ifmgr.Manager
}

func NewVPPDataplane(conn *core.Connection) *VPPDataplane {
	return &VPPDataplane{
		conn:       conn,
		ifaceCache: make(map[string]interface_types.InterfaceIndex),
		logger:     logger.Get(logger.Dataplane),
	}
}

func (d *VPPDataplane) SetVRFResolver(resolver func(string) (uint32, error)) {
	d.vrfResolver = resolver
}

func (d *VPPDataplane) SetIfMgr(m *ifmgr.Manager) {
	d.ifMgr = m
}

func (d *VPPDataplane) CreateInterface(cfg *interfaces.InterfaceConfig) error {
	ifType := inferInterfaceType(cfg)

	if ifType == "loopback" {
		return d.createLoopback(cfg)
	} else if ifType == "physical" {
		return d.createPhysicalInterface(cfg)
	}

	return fmt.Errorf("unknown interface type for %s", cfg.Name)
}

func (d *VPPDataplane) createPhysicalInterface(cfg *interfaces.InterfaceConfig) error {
	// DPDK interface exist already so we don't need to build host
	if _, err := d.getInterfaceIndex(cfg.Name); err == nil {
		d.logger.Info("Interface already exists in VPP, skipping creation", "interface", cfg.Name)
		if cfg.Enabled {
			if err := d.setInterfaceState(cfg.Name, true); err != nil {
				d.logger.Warn("Failed to set interface up", "interface", cfg.Name, "error", err)
			}
		}
		return nil
	}

	vppIfName, err := d.createVPPHostInterface(cfg.Name)
	if err != nil {
		if idx, lookupErr := d.getInterfaceIndex("host-" + cfg.Name); lookupErr == nil {
			d.logger.Info("Host-interface already exists in VPP, skipping creation", "interface", cfg.Name)
			d.ifaceCache[cfg.Name] = idx
			vppIfName = cfg.Name
		} else {
			return fmt.Errorf("create VPP host-interface: %w", err)
		}
	}

	if cfg.Enabled {
		if err := d.setInterfaceState(vppIfName, true); err != nil {
			d.logger.Warn("Failed to set interface up", "interface", vppIfName, "error", err)
		}
	}

	if cfg.Description != "" {
		d.SetInterfaceDescription(cfg.Name, cfg.Description)
	}

	return nil
}

func (d *VPPDataplane) createLoopback(cfg *interfaces.InterfaceConfig) error {
	// Check if loopback already exists in VPP
	if _, err := d.getInterfaceIndex(cfg.Name); err == nil {
		d.logger.Info("Loopback already exists in VPP, skipping creation", "interface", cfg.Name)
		if cfg.Enabled {
			if err := d.setInterfaceState(cfg.Name, true); err != nil {
				d.logger.Warn("Failed to set interface up", "interface", cfg.Name, "error", err)
			}
		}
		return nil
	}

	vppIfName, err := d.createVPPLoopback(cfg.Name)
	if err != nil {
		return fmt.Errorf("create VPP loopback: %w", err)
	}

	if cfg.LCP {
		if err := d.createLCPPair(vppIfName, cfg.Name, lcp.LCP_API_ITF_HOST_TAP); err != nil {
			return fmt.Errorf("create LCP pair: %w", err)
		}

		time.Sleep(100 * time.Millisecond)
		if cfg.Description != "" {
			d.SetInterfaceDescription(cfg.Name, cfg.Description)
		}
	}

	if cfg.VRF != "" {
		if err := d.bindInterfaceToVRF(vppIfName, cfg.Name, cfg.VRF, cfg.LCP); err != nil {
			return fmt.Errorf("bind to VRF: %w", err)
		}
	}

	if cfg.Enabled {
		if err := d.setInterfaceState(vppIfName, true); err != nil {
			d.logger.Warn("Failed to set interface up", "interface", vppIfName, "error", err)
		}
	}

	return nil
}

func (d *VPPDataplane) DeleteInterface(name string) error {
	// TODO: Implement interface deletion
	return fmt.Errorf("DeleteInterface not yet implemented")
}

func (d *VPPDataplane) SetInterfaceDescription(name, description string) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return fmt.Errorf("LCP interface %s not found: %w", name, err)
	}

	return netlink.LinkSetAlias(link, description)
}

func (d *VPPDataplane) SetInterfaceMTU(name string, mtu int) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return fmt.Errorf("interface %s not found: %w", name, err)
	}

	return netlink.LinkSetMTU(link, mtu)
}

func (d *VPPDataplane) SetInterfaceEnabled(name string, enabled bool) error {
	_, err := d.getInterfaceIndex(name)
	if err != nil {
		return fmt.Errorf("VPP interface %s not found", name)
	}

	vppIfName := name

	return d.setInterfaceState(vppIfName, enabled)
}

func (d *VPPDataplane) AddIPv4Address(ifName, address string) error {
	link, err := netlink.LinkByName(ifName)
	if err != nil {
		return fmt.Errorf("get interface %s: %w", ifName, err)
	}

	addr, err := netlink.ParseAddr(address)
	if err != nil {
		return fmt.Errorf("parse address %s: %w", address, err)
	}

	if err := netlink.AddrAdd(link, addr); err != nil {
		if err.Error() == "file exists" {
			d.logger.Info("IPv4 address already exists", "interface", ifName, "address", address)
			return nil
		}
		return fmt.Errorf("add address: %w", err)
	}

	if d.ifMgr != nil {
		if swIfIndex, ok := d.ifMgr.GetSwIfIndex(ifName); ok {
			d.ifMgr.AddIPv4Address(swIfIndex, addr.IP)
		}
	}

	d.logger.Info("Added IPv4 address", "interface", ifName, "address", address)
	return nil
}

func (d *VPPDataplane) DelIPv4Address(ifName, address string) error {
	link, err := netlink.LinkByName(ifName)
	if err != nil {
		return fmt.Errorf("get interface %s: %w", ifName, err)
	}

	addr, err := netlink.ParseAddr(address)
	if err != nil {
		return fmt.Errorf("parse address %s: %w", address, err)
	}

	if err := netlink.AddrDel(link, addr); err != nil {
		return fmt.Errorf("del address: %w", err)
	}

	if d.ifMgr != nil {
		if swIfIndex, ok := d.ifMgr.GetSwIfIndex(ifName); ok {
			d.ifMgr.RemoveIPv4Address(swIfIndex, addr.IP)
		}
	}

	d.logger.Info("Deleted IPv4 address", "interface", ifName, "address", address)
	return nil
}

func (d *VPPDataplane) AddIPv6Address(ifName, address string) error {
	link, err := netlink.LinkByName(ifName)
	if err != nil {
		return fmt.Errorf("get interface %s: %w", ifName, err)
	}

	addr, err := netlink.ParseAddr(address)
	if err != nil {
		return fmt.Errorf("parse address %s: %w", address, err)
	}

	if err := netlink.AddrAdd(link, addr); err != nil {
		if err.Error() == "file exists" {
			d.logger.Info("IPv6 address already exists", "interface", ifName, "address", address)
			return nil
		}
		return fmt.Errorf("add address: %w", err)
	}

	if d.ifMgr != nil {
		if swIfIndex, ok := d.ifMgr.GetSwIfIndex(ifName); ok {
			d.ifMgr.AddIPv6Address(swIfIndex, addr.IP)
		}
	}

	d.logger.Info("Added IPv6 address", "interface", ifName, "address", address)
	return nil
}

func (d *VPPDataplane) DelIPv6Address(ifName, address string) error {
	link, err := netlink.LinkByName(ifName)
	if err != nil {
		return fmt.Errorf("get interface %s: %w", ifName, err)
	}

	addr, err := netlink.ParseAddr(address)
	if err != nil {
		return fmt.Errorf("parse address %s: %w", address, err)
	}

	if err := netlink.AddrDel(link, addr); err != nil {
		return fmt.Errorf("del address: %w", err)
	}

	if d.ifMgr != nil {
		if swIfIndex, ok := d.ifMgr.GetSwIfIndex(ifName); ok {
			d.ifMgr.RemoveIPv6Address(swIfIndex, addr.IP)
		}
	}

	d.logger.Info("Deleted IPv6 address", "interface", ifName, "address", address)
	return nil
}

// Internal VPP helper methods

func (d *VPPDataplane) createVPPHostInterface(linuxIface string) (string, error) {
	ch, err := d.conn.NewAPIChannel()
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

	vppIfName := linuxIface
	d.ifaceCache[vppIfName] = afReply.SwIfIndex

	rxModeReq := &vppinterfaces.SwInterfaceSetRxMode{
		SwIfIndex: afReply.SwIfIndex,
		Mode:      interface_types.RX_MODE_API_POLLING,
	}
	rxModeReply := &vppinterfaces.SwInterfaceSetRxModeReply{}
	if err := ch.SendRequest(rxModeReq).ReceiveReply(rxModeReply); err != nil {
		d.logger.Warn("Failed to set RX mode to polling", "interface", vppIfName, "error", err)
	}

	d.logger.Info("Created VPP host-interface", "linux_iface", linuxIface, "vpp_iface", vppIfName, "sw_if_index", afReply.SwIfIndex)

	return vppIfName, nil
}

func (d *VPPDataplane) createVPPLoopback(name string) (string, error) {
	ch, err := d.conn.NewAPIChannel()
	if err != nil {
		return "", fmt.Errorf("create API channel: %w", err)
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
		return "", fmt.Errorf("create loopback: %w", err)
	}

	if reply.Retval != 0 {
		return "", fmt.Errorf("create loopback failed: retval=%d", reply.Retval)
	}

	dumpReq := &vppinterfaces.SwInterfaceDump{
		SwIfIndex: reply.SwIfIndex,
	}

	stream := ch.SendMultiRequest(dumpReq)
	for {
		dumpReply := &vppinterfaces.SwInterfaceDetails{}
		stop, err := stream.ReceiveReply(dumpReply)
		if err != nil {
			return "", fmt.Errorf("get interface name: %w", err)
		}
		if stop {
			break
		}

		if dumpReply.SwIfIndex == reply.SwIfIndex {
			d.ifaceCache[dumpReply.InterfaceName] = reply.SwIfIndex
			d.logger.Info("Created VPP loopback", "config_name", name, "vpp_name", dumpReply.InterfaceName, "sw_if_index", reply.SwIfIndex)
			return dumpReply.InterfaceName, nil
		}
	}

	d.ifaceCache[name] = reply.SwIfIndex
	d.logger.Warn("Could not determine VPP loopback name, using config name", "name", name, "sw_if_index", reply.SwIfIndex)
	return name, nil
}

func (d *VPPDataplane) createLCPPair(vppIface, linuxIface string, hostType lcp.LcpItfHostType) error {
	ch, err := d.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	swIfIndex, err := d.getInterfaceIndex(vppIface)
	if err != nil {
		swIfIndex = interface_types.InterfaceIndex(^uint32(0))
		d.logger.Debug("VPP interface not found, will create from Linux interface",
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

	d.logger.Info("Created LCP pair", "vpp_iface", vppIface, "linux_iface", linuxIface, "host_type", hostType)
	return nil
}

func (d *VPPDataplane) getInterfaceIndex(name string) (interface_types.InterfaceIndex, error) {
	if idx, ok := d.ifaceCache[name]; ok {
		return idx, nil
	}

	ch, err := d.conn.NewAPIChannel()
	if err != nil {
		return 0, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &vppinterfaces.SwInterfaceDump{
		NameFilterValid: true,
		NameFilter:      name,
	}

	stream := ch.SendMultiRequest(req)
	for {
		reply := &vppinterfaces.SwInterfaceDetails{}
		stop, err := stream.ReceiveReply(reply)
		if err != nil {
			return 0, fmt.Errorf("receive reply: %w", err)
		}
		if stop {
			break
		}

		if reply.InterfaceName == name {
			d.ifaceCache[name] = reply.SwIfIndex
			return reply.SwIfIndex, nil
		}
	}

	return 0, fmt.Errorf("interface %s not found in VPP", name)
}

func (d *VPPDataplane) setInterfaceState(name string, enabled bool) error {
	ch, err := d.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	swIfIndex, err := d.getInterfaceIndex(name)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	var flags interface_types.IfStatusFlags
	if enabled {
		flags = interface_types.IF_STATUS_API_FLAG_ADMIN_UP
	}

	req := &vppinterfaces.SwInterfaceSetFlags{
		SwIfIndex: swIfIndex,
		Flags:     flags,
	}

	reply := &vppinterfaces.SwInterfaceSetFlagsReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("set interface state: %w", err)
	}

	d.logger.Info("Set interface state", "interface", name, "enabled", enabled)
	return nil
}

func (d *VPPDataplane) bindInterfaceToVRF(vppIfName, linuxIfName, vrfName string, hasLCP bool) error {
	if d.vrfResolver == nil {
		return fmt.Errorf("VRF resolver not configured")
	}

	tableID, err := d.vrfResolver(vrfName)
	if err != nil {
		return fmt.Errorf("resolve VRF %q: %w", vrfName, err)
	}

	if err := d.setInterfaceTable(vppIfName, tableID); err != nil {
		return fmt.Errorf("set VPP table: %w", err)
	}

	if hasLCP {
		if err := d.setLinuxInterfaceVRF(linuxIfName, vrfName); err != nil {
			return fmt.Errorf("set Linux VRF: %w", err)
		}
	}

	d.logger.Info("Bound interface to VRF", "vpp_iface", vppIfName, "linux_iface", linuxIfName, "vrf", vrfName, "table_id", tableID)
	return nil
}

func (d *VPPDataplane) setInterfaceTable(name string, tableID uint32) error {
	ch, err := d.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	swIfIndex, err := d.getInterfaceIndex(name)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	// Set IPv4 table
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

	// Set IPv6 table
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

	if d.ifMgr != nil {
		d.ifMgr.SetFIBTableID(uint32(swIfIndex), tableID)
	}

	return nil
}

func (d *VPPDataplane) setLinuxInterfaceVRF(ifName, vrfName string) error {
	vrfLink, err := netlink.LinkByName(vrfName)
	if err != nil {
		return fmt.Errorf("VRF device %q not found: %w", vrfName, err)
	}

	tapLink, err := netlink.LinkByName(ifName)
	if err != nil {
		return fmt.Errorf("interface %q not found: %w", ifName, err)
	}

	if err := netlink.LinkSetMaster(tapLink, vrfLink); err != nil {
		return fmt.Errorf("enslave %q to VRF %q: %w", ifName, vrfName, err)
	}

	return nil
}

func inferInterfaceType(cfg *interfaces.InterfaceConfig) string {
	if cfg.Type != "" {
		return cfg.Type
	}

	if cfg.Name == "loop100" || cfg.Name[:4] == "loop" {
		return "loopback"
	}

	return "physical"
}
