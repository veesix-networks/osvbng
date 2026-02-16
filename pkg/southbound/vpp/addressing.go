package vpp

import (
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/fib_control"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip_types"
	"go.fd.io/govpp/api"
	"net"
	interfaces "github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface"
)

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


