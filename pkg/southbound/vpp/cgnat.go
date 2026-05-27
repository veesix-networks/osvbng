// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package vpp

import (
	"fmt"
	"net"

	"go.fd.io/govpp/api"

	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/osvbng_cgnat"
)

var _ southbound.CGNATDataplane = (*VPP)(nil)

func ip4Addr(ip net.IP) ip_types.IP4Address {
	var addr ip_types.IP4Address
	copy(addr[:], ip.To4())
	return addr
}

func ip4FromAddr(addr ip_types.IP4Address) net.IP {
	return net.IP(addr[:]).To4()
}

func prefixFromIPNet(ipNet net.IPNet) ip_types.Prefix {
	ones, _ := ipNet.Mask.Size()
	var addr ip_types.IP4Address
	copy(addr[:], ipNet.IP.To4())
	return ip_types.Prefix{
		Address: ip_types.Address{
			Af: ip_types.ADDRESS_IP4,
			Un: ip_types.AddressUnionIP4(addr),
		},
		Len: uint8(ones),
	}
}

func (v *VPP) CGNATPoolAddDel(poolID uint32, mode uint8, addressPooling uint8,
	filtering uint8, blockSize uint16, maxBlocksPerSub uint8,
	maxSessionsPerSub uint32, portRangeStart uint16, portRangeEnd uint16,
	portReuseTimeout uint16, algBitmask uint8, timeouts [4]uint32,
	isAdd bool) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &osvbng_cgnat.OsvbngCgnatPoolAddDel{
		IsAdd:             isAdd,
		PoolID:            poolID,
		Mode:              osvbng_cgnat.OsvbngCgnatPoolMode(mode),
		AddressPooling:    osvbng_cgnat.OsvbngCgnatAddressPooling(addressPooling),
		Filtering:         osvbng_cgnat.OsvbngCgnatFiltering(filtering),
		BlockSize:         blockSize,
		MaxBlocksPerSub:   maxBlocksPerSub,
		MaxSessionsPerSub: maxSessionsPerSub,
		PortRangeStart:    portRangeStart,
		PortRangeEnd:      portRangeEnd,
		PortReuseTimeout:  portReuseTimeout,
		AlgBitmask:        algBitmask,
		Timeouts: osvbng_cgnat.OsvbngCgnatTimeouts{
			TCPEstablished: timeouts[0],
			TCPTransitory:  timeouts[1],
			UDP:            timeouts[2],
			ICMP:           timeouts[3],
		},
	}

	reply := &osvbng_cgnat.OsvbngCgnatPoolAddDelReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("pool add/del: %w", err)
	}

	if isAdd && reply.Retval == retvalEntryNeedsRefresh {
		v.logger.Info("CGNAT pool params hard-drifted; replacing",
			"pool_id", poolID)
		delReq := *req
		delReq.IsAdd = false
		delReply := &osvbng_cgnat.OsvbngCgnatPoolAddDelReply{}
		if err := ch.SendRequest(&delReq).ReceiveReply(delReply); err != nil {
			return fmt.Errorf("pool refresh: del: %w", err)
		}
		if delReply.Retval != 0 {
			return fmt.Errorf("pool refresh: del retval=%d", delReply.Retval)
		}
		reply = &osvbng_cgnat.OsvbngCgnatPoolAddDelReply{}
		if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
			return fmt.Errorf("pool refresh: re-add: %w", err)
		}
		if reply.Retval != 0 {
			return fmt.Errorf("pool refresh: re-add retval=%d", reply.Retval)
		}
		return nil
	}

	if reply.Retval != 0 {
		return fmt.Errorf("pool add/del failed: retval=%d", reply.Retval)
	}
	return nil
}

func (v *VPP) CGNATPoolAddInsidePrefix(poolID uint32, prefix net.IPNet, vrfID uint32, isAdd bool) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &osvbng_cgnat.OsvbngCgnatPoolAddDelInsidePrefix{
		IsAdd:  isAdd,
		PoolID: poolID,
		Prefix: prefixFromIPNet(prefix),
		VrfID:  vrfID,
	}

	reply := &osvbng_cgnat.OsvbngCgnatPoolAddDelInsidePrefixReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("pool inside prefix: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("pool inside prefix failed: retval=%d", reply.Retval)
	}
	return nil
}

func (v *VPP) CGNATPoolAddOutsideAddress(poolID uint32, prefix net.IPNet, isAdd bool) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &osvbng_cgnat.OsvbngCgnatPoolAddDelOutsideAddress{
		IsAdd:  isAdd,
		PoolID: poolID,
		Prefix: prefixFromIPNet(prefix),
	}

	reply := &osvbng_cgnat.OsvbngCgnatPoolAddDelOutsideAddressReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("pool outside address: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("pool outside address failed: retval=%d", reply.Retval)
	}
	return nil
}

func (v *VPP) CGNATSetOutsideVRF(poolID uint32, vrfTableID uint32) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &osvbng_cgnat.OsvbngCgnatSetOutsideFib{
		PoolID: poolID,
		VrfID:  vrfTableID,
	}

	reply := &osvbng_cgnat.OsvbngCgnatSetOutsideFibReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("set outside VRF: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("set outside VRF failed: retval=%d", reply.Retval)
	}
	return nil
}

func (v *VPP) CGNATPoolUpdate(poolID uint32, maxSessions uint32, algBitmask uint8, timeouts [4]uint32) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &osvbng_cgnat.OsvbngCgnatPoolUpdate{
		PoolID:            poolID,
		MaxSessionsPerSub: maxSessions,
		AlgBitmask:        algBitmask,
		Timeouts: osvbng_cgnat.OsvbngCgnatTimeouts{
			TCPEstablished: timeouts[0],
			TCPTransitory:  timeouts[1],
			UDP:            timeouts[2],
			ICMP:           timeouts[3],
		},
	}

	reply := &osvbng_cgnat.OsvbngCgnatPoolUpdateReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("pool update: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("pool update failed: retval=%d", reply.Retval)
	}
	return nil
}

func (v *VPP) CGNATAddDelSubscriberMapping(poolID uint32, swIfIndex uint32,
	insideIP net.IP, insideVRFID uint32, outsideIP net.IP,
	portStart uint16, portEnd uint16, enableFeature bool, isAdd bool) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &osvbng_cgnat.OsvbngCgnatAddDelSubscriberMapping{
		IsAdd:          isAdd,
		PoolID:         poolID,
		SwIfIndex:      interface_types.InterfaceIndex(swIfIndex),
		InsideIP:       ip4Addr(insideIP),
		InsideVrfID:    insideVRFID,
		OutsideIP:      ip4Addr(outsideIP),
		PortBlockStart: portStart,
		PortBlockEnd:   portEnd,
		EnableFeature:  enableFeature,
	}

	reply := &osvbng_cgnat.OsvbngCgnatAddDelSubscriberMappingReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("subscriber mapping: %w", err)
	}

	if isAdd && reply.Retval == retvalEntryNeedsRefresh {
		v.logger.Info("CGNAT mapping exists with drifted mutable inputs; refreshing",
			"pool_id", poolID,
			"inside_ip", insideIP.String(),
			"inside_vrf", insideVRFID)

		delReq := &osvbng_cgnat.OsvbngCgnatAddDelSubscriberMapping{
			IsAdd:         false,
			PoolID:        poolID,
			SwIfIndex:     interface_types.InterfaceIndex(swIfIndex),
			InsideIP:      ip4Addr(insideIP),
			InsideVrfID:   insideVRFID,
			EnableFeature: enableFeature,
		}
		delReply := &osvbng_cgnat.OsvbngCgnatAddDelSubscriberMappingReply{}
		if err := ch.SendRequest(delReq).ReceiveReply(delReply); err != nil {
			return fmt.Errorf("refresh cgnat mapping: del: %w", err)
		}
		if delReply.Retval != 0 {
			return fmt.Errorf("refresh cgnat mapping: del retval=%d", delReply.Retval)
		}

		reply = &osvbng_cgnat.OsvbngCgnatAddDelSubscriberMappingReply{}
		if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
			return fmt.Errorf("refresh cgnat mapping: re-add: %w", err)
		}
		if reply.Retval != 0 {
			return fmt.Errorf("refresh cgnat mapping: re-add retval=%d", reply.Retval)
		}
		return nil
	}

	if reply.Retval != 0 {
		return fmt.Errorf("subscriber mapping failed: retval=%d", reply.Retval)
	}
	return nil
}

func (v *VPP) CGNATAddDelSubscriberMappingAsync(poolID uint32, swIfIndex uint32,
	insideIP net.IP, insideVRFID uint32, outsideIP net.IP,
	portStart uint16, portEnd uint16, enableFeature bool, isAdd bool,
	callback func(error)) {

	req := &osvbng_cgnat.OsvbngCgnatAddDelSubscriberMapping{
		IsAdd:          isAdd,
		PoolID:         poolID,
		SwIfIndex:      interface_types.InterfaceIndex(swIfIndex),
		InsideIP:       ip4Addr(insideIP),
		InsideVrfID:    insideVRFID,
		OutsideIP:      ip4Addr(outsideIP),
		PortBlockStart: portStart,
		PortBlockEnd:   portEnd,
		EnableFeature:  enableFeature,
	}

	v.asyncWorker.SendAsync(req, func(reply api.Message, err error) {
		if err != nil {
			callback(err)
			return
		}
		rmp := reply.(*osvbng_cgnat.OsvbngCgnatAddDelSubscriberMappingReply)
		if isAdd && rmp.Retval == retvalEntryNeedsRefresh {
			v.logger.Info("CGNAT mapping exists with drifted mutable inputs; refreshing (async)",
				"pool_id", poolID,
				"inside_ip", insideIP.String(),
				"inside_vrf", insideVRFID)

			delReq := &osvbng_cgnat.OsvbngCgnatAddDelSubscriberMapping{
				IsAdd:         false,
				PoolID:        poolID,
				SwIfIndex:     interface_types.InterfaceIndex(swIfIndex),
				InsideIP:      ip4Addr(insideIP),
				InsideVrfID:   insideVRFID,
				EnableFeature: enableFeature,
			}
			v.asyncWorker.SendAsync(delReq, func(delReply api.Message, delErr error) {
				if delErr != nil {
					callback(fmt.Errorf("refresh cgnat mapping: del: %w", delErr))
					return
				}
				dr, ok := delReply.(*osvbng_cgnat.OsvbngCgnatAddDelSubscriberMappingReply)
				if !ok {
					callback(fmt.Errorf("refresh cgnat mapping: del unexpected reply type: %T", delReply))
					return
				}
				if dr.Retval != 0 {
					callback(fmt.Errorf("refresh cgnat mapping: del retval=%d", dr.Retval))
					return
				}
				v.asyncWorker.SendAsync(req, func(reReply api.Message, reErr error) {
					if reErr != nil {
						callback(fmt.Errorf("refresh cgnat mapping: re-add: %w", reErr))
						return
					}
					rr, ok := reReply.(*osvbng_cgnat.OsvbngCgnatAddDelSubscriberMappingReply)
					if !ok {
						callback(fmt.Errorf("refresh cgnat mapping: re-add unexpected reply type: %T", reReply))
						return
					}
					if rr.Retval != 0 {
						callback(fmt.Errorf("refresh cgnat mapping: re-add retval=%d", rr.Retval))
						return
					}
					callback(nil)
				})
			})
			return
		}
		if rmp.Retval != 0 {
			callback(fmt.Errorf("subscriber mapping failed: retval=%d", rmp.Retval))
			return
		}
		callback(nil)
	})
}

func (v *VPP) CGNATAddSubscriberMappingBulk(poolID uint32, mappings []southbound.CGNATMapping) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	entries := make([]osvbng_cgnat.OsvbngCgnatBulkMappingEntry, len(mappings))
	for i, m := range mappings {
		entries[i] = osvbng_cgnat.OsvbngCgnatBulkMappingEntry{
			SwIfIndex:      interface_types.InterfaceIndex(m.SwIfIndex),
			InsideIP:       ip4Addr(m.InsideIP),
			InsideVrfID:    m.InsideVRFID,
			OutsideIP:      ip4Addr(m.OutsideIP),
			PortBlockStart: m.PortBlockStart,
			PortBlockEnd:   m.PortBlockEnd,
			EnableFeature:  m.EnableFeature,
		}
	}

	req := &osvbng_cgnat.OsvbngCgnatAddSubscriberMappingBulk{
		PoolID:   poolID,
		Count:    uint32(len(entries)),
		Mappings: entries,
	}

	reply := &osvbng_cgnat.OsvbngCgnatAddSubscriberMappingBulkReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("bulk mapping: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("bulk mapping failed: retval=%d", reply.Retval)
	}
	return nil
}

func (v *VPP) CGNATEnableOnSession(poolID uint32, swIfIndex uint32, isEnable bool) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &osvbng_cgnat.OsvbngCgnatEnableOnSession{
		IsEnable:  isEnable,
		PoolID:    poolID,
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
	}

	reply := &osvbng_cgnat.OsvbngCgnatEnableOnSessionReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("enable on session: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("enable on session failed: retval=%d", reply.Retval)
	}
	return nil
}

func (v *VPP) CGNATAddDelBypass(prefix net.IPNet, vrfID uint32, isAdd bool) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &osvbng_cgnat.OsvbngCgnatAddDelBypass{
		IsAdd:       isAdd,
		Prefix:      prefixFromIPNet(prefix),
		InsideVrfID: vrfID,
	}

	reply := &osvbng_cgnat.OsvbngCgnatAddDelBypassReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("bypass: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("bypass failed: retval=%d", reply.Retval)
	}
	return nil
}

func (v *VPP) CGNATPoolDump() ([]southbound.CGNATPoolState, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &osvbng_cgnat.OsvbngCgnatPoolDump{PoolID: ^uint32(0)}
	var results []southbound.CGNATPoolState
	multi := ch.SendMultiRequest(req)
	for {
		d := &osvbng_cgnat.OsvbngCgnatPoolDetails{}
		stop, err := multi.ReceiveReply(d)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("receive pool details: %w", err)
		}
		results = append(results, southbound.CGNATPoolState{
			PoolID:            d.PoolID,
			Mode:              uint8(d.Mode),
			AddressPooling:    uint8(d.AddressPooling),
			Filtering:         uint8(d.Filtering),
			BlockSize:         d.BlockSize,
			MaxBlocksPerSub:   d.MaxBlocksPerSub,
			MaxSessionsPerSub: d.MaxSessionsPerSub,
			PortRangeStart:    d.PortRangeStart,
			PortRangeEnd:      d.PortRangeEnd,
			PortReuseTimeout:  d.PortReuseTimeout,
			ALGBitmask:        d.AlgBitmask,
			Timeouts: [4]uint32{
				d.Timeouts.TCPEstablished,
				d.Timeouts.TCPTransitory,
				d.Timeouts.UDP,
				d.Timeouts.ICMP,
			},
			OutsideVRFTableID: d.OutsideVrfTableID,
			ActiveMappings:    d.ActiveMappings,
		})
	}
	return results, nil
}

func (v *VPP) CGNATPoolInsidePrefixDump(poolID uint32) ([]southbound.CGNATInsidePrefixState, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	filter := poolID
	if filter == 0 {
		filter = ^uint32(0)
	}
	req := &osvbng_cgnat.OsvbngCgnatPoolInsidePrefixDump{PoolID: filter}
	var results []southbound.CGNATInsidePrefixState
	multi := ch.SendMultiRequest(req)
	for {
		d := &osvbng_cgnat.OsvbngCgnatPoolInsidePrefixDetails{}
		stop, err := multi.ReceiveReply(d)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("receive inside-prefix details: %w", err)
		}
		var v4 net.IP
		if d.Prefix.Address.Af == ip_types.ADDRESS_IP4 {
			a := d.Prefix.Address.Un.GetIP4()
			v4 = net.IP(a[:]).To4()
		}
		results = append(results, southbound.CGNATInsidePrefixState{
			PoolID: d.PoolID,
			Prefix: net.IPNet{IP: v4, Mask: net.CIDRMask(int(d.Prefix.Len), 32)},
			VRFID:  d.VrfID,
		})
	}
	return results, nil
}

func (v *VPP) CGNATPoolOutsideAddressDump(poolID uint32) ([]southbound.CGNATOutsideAddressState, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	filter := poolID
	if filter == 0 {
		filter = ^uint32(0)
	}
	req := &osvbng_cgnat.OsvbngCgnatPoolOutsideAddressDump{PoolID: filter}
	var results []southbound.CGNATOutsideAddressState
	multi := ch.SendMultiRequest(req)
	for {
		d := &osvbng_cgnat.OsvbngCgnatPoolOutsideAddressDetails{}
		stop, err := multi.ReceiveReply(d)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("receive outside-address details: %w", err)
		}
		var v4 net.IP
		if d.Prefix.Address.Af == ip_types.ADDRESS_IP4 {
			a := d.Prefix.Address.Un.GetIP4()
			v4 = net.IP(a[:]).To4()
		}
		results = append(results, southbound.CGNATOutsideAddressState{
			PoolID: d.PoolID,
			Prefix: net.IPNet{IP: v4, Mask: net.CIDRMask(int(d.Prefix.Len), 32)},
		})
	}
	return results, nil
}

func (v *VPP) CGNATDumpSubscriberMappings(poolID uint32) ([]southbound.CGNATMapping, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &osvbng_cgnat.OsvbngCgnatSubscriberMappingDump{
		PoolID: poolID,
	}

	var results []southbound.CGNATMapping
	multi := ch.SendMultiRequest(req)
	for {
		details := &osvbng_cgnat.OsvbngCgnatSubscriberMappingDetails{}
		stop, err := multi.ReceiveReply(details)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("receive mapping details: %w", err)
		}
		results = append(results, southbound.CGNATMapping{
			PoolID:         details.PoolID,
			SwIfIndex:      uint32(details.SwIfIndex),
			InsideIP:       ip4FromAddr(details.InsideIP),
			InsideVRFID:    details.InsideVrfID,
			OutsideIP:      ip4FromAddr(details.OutsideIP),
			PortBlockStart: details.PortBlockStart,
			PortBlockEnd:   details.PortBlockEnd,
			ActiveSessions: details.ActiveSessions,
		})
	}
	return results, nil
}
