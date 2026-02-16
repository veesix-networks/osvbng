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
	"github.com/vishvananda/netns"
	"go.fd.io/govpp/core"
)

type VPPDataplane struct {
	conn        *core.Connection
	ifaceCache  map[string]interface_types.InterfaceIndex
	logger      *slog.Logger
	vrfResolver func(string) (uint32, bool, bool, error)
	ifMgr       *ifmgr.Manager
	lcpNs       *netlink.Handle
}

func NewVPPDataplane(conn *core.Connection) *VPPDataplane {
	return &VPPDataplane{
		conn:       conn,
		ifaceCache: make(map[string]interface_types.InterfaceIndex),
		logger:     logger.Get(logger.Dataplane),
	}
}

func (d *VPPDataplane) SetVRFResolver(resolver func(string) (uint32, bool, bool, error)) {
	d.vrfResolver = resolver
}

func (d *VPPDataplane) SetIfMgr(m *ifmgr.Manager) {
	d.ifMgr = m
}

func (d *VPPDataplane) SetLCPNetNs(nsName string) error {
	nsHandle, err := netns.GetFromName(nsName)
	if err != nil {
		return fmt.Errorf("get netns %q: %w", nsName, err)
	}

	h, err := netlink.NewHandleAt(nsHandle)
	if err != nil {
		nsHandle.Close()
		return fmt.Errorf("create netlink handle for netns %q: %w", nsName, err)
	}

	lo, err := h.LinkByName("lo")
	if err == nil {
		if err := h.LinkSetUp(lo); err != nil {
			d.logger.Warn("Failed to bring up loopback in LCP namespace", "netns", nsName, "error", err)
		}
	}

	d.lcpNs = h
	d.logger.Info("LCP namespace configured", "netns", nsName)
	return nil
}

func (d *VPPDataplane) findLink(name string) (netlink.Link, *netlink.Handle, error) {
	if d.lcpNs != nil {
		if link, err := d.lcpNs.LinkByName(name); err == nil {
			return link, d.lcpNs, nil
		}
	}

	link, err := netlink.LinkByName(name)
	if err != nil {
		return nil, nil, fmt.Errorf("interface %q not found: %w", name, err)
	}
	return link, nil, nil
}

func (d *VPPDataplane) addrAdd(h *netlink.Handle, link netlink.Link, addr *netlink.Addr) error {
	if h != nil {
		return h.AddrAdd(link, addr)
	}
	return netlink.AddrAdd(link, addr)
}

func (d *VPPDataplane) addrDel(h *netlink.Handle, link netlink.Link, addr *netlink.Addr) error {
	if h != nil {
		return h.AddrDel(link, addr)
	}
	return netlink.AddrDel(link, addr)
}

func (d *VPPDataplane) renameVPPInterface(oldName, newName string) error {
	ch, err := d.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	swIfIndex, err := d.getInterfaceIndex(oldName)
	if err != nil {
		return fmt.Errorf("get interface index for %q: %w", oldName, err)
	}

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

	delete(d.ifaceCache, oldName)
	d.ifaceCache[newName] = swIfIndex

	if d.ifMgr != nil {
		d.ifMgr.Rename(oldName, newName)
	}

	d.logger.Info("Renamed VPP interface", "old_name", oldName, "new_name", newName)
	return nil
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
	// DPDK path: interface already exists in VPP (no AF_PACKET creation needed)
	if _, err := d.getInterfaceIndex(cfg.Name); err == nil {
		d.logger.Info("Interface already exists in VPP, skipping creation", "interface", cfg.Name)
		if cfg.Enabled {
			if err := d.setInterfaceState(cfg.Name, true); err != nil {
				d.logger.Warn("Failed to set interface up", "interface", cfg.Name, "error", err)
			}
		}

		if cfg.LCP {
			if err := d.createLCPPair(cfg.Name, cfg.Name, lcp.LCP_API_ITF_HOST_TAP); err != nil {
				return fmt.Errorf("create LCP pair: %w", err)
			}
			time.Sleep(100 * time.Millisecond)
		}

		if cfg.Description != "" {
			d.SetInterfaceDescription(cfg.Name, cfg.Description)
		}

		if cfg.VRF != "" {
			if err := d.bindInterfaceToVRF(cfg.Name, cfg.Name, cfg.VRF, cfg.LCP); err != nil {
				return fmt.Errorf("bind to VRF: %w", err)
			}
		}

		return nil
	}

	// AF_PACKET path: create host-interface, then rename
	vppIfName, err := d.createVPPHostInterface(cfg.Name)
	if err != nil {
		if idx, lookupErr := d.getInterfaceIndex("host-" + cfg.Name); lookupErr == nil {
			d.logger.Info("Host-interface already exists in VPP, skipping creation", "interface", cfg.Name)
			d.ifaceCache["host-"+cfg.Name] = idx
			if d.ifMgr != nil {
				d.ifMgr.Add(&ifmgr.Interface{
					SwIfIndex:    uint32(idx),
					SupSwIfIndex: uint32(idx),
					Name:         "host-" + cfg.Name,
					DevType:      "af_packet",
					Type:         ifmgr.IfTypeHardware,
				})
			}
			vppIfName = "host-" + cfg.Name
		} else {
			return fmt.Errorf("create VPP host-interface: %w", err)
		}
	}

	// Rename VPP interface from "host-ethX" to "ethX"
	if err := d.renameVPPInterface(vppIfName, cfg.Name); err != nil {
		d.logger.Warn("Failed to rename VPP interface, continuing with original name",
			"old_name", vppIfName, "new_name", cfg.Name, "error", err)
	} else {
		vppIfName = cfg.Name
	}

	// Match VPP interface MTU to underlying Linux interface
	if hostMTU, err := d.getLinuxInterfaceMTU(cfg.Name); err == nil && hostMTU > 0 {
		if err := d.setVPPInterfaceHWMtu(vppIfName, uint16(hostMTU)); err != nil {
			d.logger.Warn("Failed to set VPP interface MTU", "interface", vppIfName, "mtu", hostMTU, "error", err)
		} else {
			d.logger.Info("Set VPP interface MTU to match host", "interface", vppIfName, "mtu", hostMTU)
		}
	}

	if cfg.Enabled {
		if err := d.setInterfaceState(vppIfName, true); err != nil {
			d.logger.Warn("Failed to set interface up", "interface", vppIfName, "error", err)
		}
	}

	if cfg.LCP {
		if err := d.createLCPPair(vppIfName, cfg.Name, lcp.LCP_API_ITF_HOST_TAP); err != nil {
			return fmt.Errorf("create LCP pair: %w", err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	if cfg.Description != "" {
		d.SetInterfaceDescription(cfg.Name, cfg.Description)
	}

	if cfg.VRF != "" {
		if err := d.bindInterfaceToVRF(vppIfName, cfg.Name, cfg.VRF, cfg.LCP); err != nil {
			return fmt.Errorf("bind to VRF: %w", err)
		}
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

	if swIfIndex, ok := d.ifaceCache[vppIfName]; ok && d.ifMgr != nil {
		d.ifMgr.Add(&ifmgr.Interface{
			SwIfIndex:    uint32(swIfIndex),
			SupSwIfIndex: uint32(swIfIndex),
			Name:         vppIfName,
			DevType:      "loopback",
			Type:         ifmgr.IfTypeHardware,
		})
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
	link, h, err := d.findLink(name)
	if err != nil {
		return fmt.Errorf("LCP interface %s not found: %w", name, err)
	}

	if h != nil {
		return h.LinkSetAlias(link, description)
	}
	return netlink.LinkSetAlias(link, description)
}

func (d *VPPDataplane) getLinuxInterfaceMTU(name string) (int, error) {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return 0, fmt.Errorf("interface %q not found: %w", name, err)
	}
	return link.Attrs().MTU, nil
}

func (d *VPPDataplane) setVPPInterfaceHWMtu(name string, mtu uint16) error {
	ch, err := d.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	swIfIndex, err := d.getInterfaceIndex(name)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

	req := &vppinterfaces.HwInterfaceSetMtu{
		SwIfIndex: swIfIndex,
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

func (d *VPPDataplane) SetInterfaceMTU(name string, mtu int) error {
	link, h, err := d.findLink(name)
	if err != nil {
		return fmt.Errorf("interface %s not found: %w", name, err)
	}

	if h != nil {
		return h.LinkSetMTU(link, mtu)
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
	link, h, err := d.findLink(ifName)
	if err != nil {
		return fmt.Errorf("get interface %s: %w", ifName, err)
	}

	addr, err := netlink.ParseAddr(address)
	if err != nil {
		return fmt.Errorf("parse address %s: %w", address, err)
	}

	if err := d.addrAdd(h, link, addr); err != nil {
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
	link, h, err := d.findLink(ifName)
	if err != nil {
		return fmt.Errorf("get interface %s: %w", ifName, err)
	}

	addr, err := netlink.ParseAddr(address)
	if err != nil {
		return fmt.Errorf("parse address %s: %w", address, err)
	}

	if err := d.addrDel(h, link, addr); err != nil {
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
	link, h, err := d.findLink(ifName)
	if err != nil {
		return fmt.Errorf("get interface %s: %w", ifName, err)
	}

	addr, err := netlink.ParseAddr(address)
	if err != nil {
		return fmt.Errorf("parse address %s: %w", address, err)
	}

	if err := d.addrAdd(h, link, addr); err != nil {
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
	link, h, err := d.findLink(ifName)
	if err != nil {
		return fmt.Errorf("get interface %s: %w", ifName, err)
	}

	addr, err := netlink.ParseAddr(address)
	if err != nil {
		return fmt.Errorf("parse address %s: %w", address, err)
	}

	if err := d.addrDel(h, link, addr); err != nil {
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

	vppIfName := "host-" + linuxIface
	d.ifaceCache[vppIfName] = afReply.SwIfIndex

	if d.ifMgr != nil {
		d.ifMgr.Add(&ifmgr.Interface{
			SwIfIndex:    uint32(afReply.SwIfIndex),
			SupSwIfIndex: uint32(afReply.SwIfIndex),
			Name:         vppIfName,
			DevType:      "af_packet",
			Type:         ifmgr.IfTypeHardware,
		})
	}

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

	tableID, hasIPv4, hasIPv6, err := d.vrfResolver(vrfName)
	if err != nil {
		return fmt.Errorf("resolve VRF %q: %w", vrfName, err)
	}

	if err := d.setInterfaceTable(vppIfName, tableID, hasIPv4, hasIPv6); err != nil {
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

func (d *VPPDataplane) setInterfaceTable(name string, tableID uint32, ipv4, ipv6 bool) error {
	ch, err := d.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	swIfIndex, err := d.getInterfaceIndex(name)
	if err != nil {
		return fmt.Errorf("get interface index: %w", err)
	}

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

	if d.ifMgr != nil {
		d.ifMgr.SetFIBTableID(uint32(swIfIndex), tableID)
	}

	return nil
}

func (d *VPPDataplane) setLinuxInterfaceVRF(ifName, vrfName string) error {
	vrfLink, vrfH, err := d.findLink(vrfName)
	if err != nil {
		return fmt.Errorf("VRF device %q not found: %w", vrfName, err)
	}

	tapLink, tapH, err := d.findLink(ifName)
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

func inferInterfaceType(cfg *interfaces.InterfaceConfig) string {
	if cfg.Type != "" {
		return cfg.Type
	}

	if cfg.Name == "loop100" || cfg.Name[:4] == "loop" {
		return "loopback"
	}

	return "physical"
}
