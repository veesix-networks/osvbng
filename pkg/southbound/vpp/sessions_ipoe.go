package vpp

import (
	"fmt"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ethernet_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/osvbng_ipoe"
	"go.fd.io/govpp/api"
	"net"
)

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


