// Copyright 2025 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package vpp

import (
	"fmt"
	"net"

	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ethernet_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/osvbng_srg"
)

var _ southbound.SRGDataplane = (*VPP)(nil)

func (v *VPP) AddSRG(srgName string, virtualMAC net.HardwareAddr, swIfIndices []uint32) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	var mac ethernet_types.MacAddress
	copy(mac[:], virtualMAC)

	indices := make([]interface_types.InterfaceIndex, len(swIfIndices))
	for i, idx := range swIfIndices {
		indices[i] = interface_types.InterfaceIndex(idx)
	}

	req := &osvbng_srg.OsvbngSrgAddDel{
		IsAdd:       true,
		SrgName:     srgName,
		VirtualMac:  mac,
		SwIfCount:   uint32(len(swIfIndices)),
		SwIfIndices: indices,
	}

	reply := &osvbng_srg.OsvbngSrgAddDelReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("add SRG: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("add SRG failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Added SRG", "name", srgName, "virtual_mac", virtualMAC, "interfaces", len(swIfIndices))
	return nil
}

func (v *VPP) DelSRG(srgName string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &osvbng_srg.OsvbngSrgAddDel{
		IsAdd:   false,
		SrgName: srgName,
	}

	reply := &osvbng_srg.OsvbngSrgAddDelReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("del SRG: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("del SRG failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Deleted SRG", "name", srgName)
	return nil
}

func (v *VPP) SetSRGState(srgName string, isActive bool) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &osvbng_srg.OsvbngSrgSetState{
		SrgName:  srgName,
		IsActive: isActive,
	}

	reply := &osvbng_srg.OsvbngSrgSetStateReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("set SRG state: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("set SRG state failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Set SRG state", "name", srgName, "active", isActive)
	return nil
}

func (v *VPP) SendSRGGarp(srgName string, entries []southbound.SRGGarpEntry) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	apiEntries := make([]osvbng_srg.OsvbngSrgGarpEntry, len(entries))
	for i, e := range entries {
		apiEntries[i].SwIfIndex = interface_types.InterfaceIndex(e.SwIfIndex)

		if ip4 := e.IP.To4(); ip4 != nil {
			var addr ip_types.IP4Address
			copy(addr[:], ip4)
			apiEntries[i].IPAddress = ip_types.Address{
				Af: ip_types.ADDRESS_IP4,
				Un: ip_types.AddressUnionIP4(addr),
			}
		} else {
			var addr ip_types.IP6Address
			copy(addr[:], e.IP.To16())
			apiEntries[i].IPAddress = ip_types.Address{
				Af: ip_types.ADDRESS_IP6,
				Un: ip_types.AddressUnionIP6(addr),
			}
		}
	}

	req := &osvbng_srg.OsvbngSrgSendGarp{
		SrgName: srgName,
		Count:   uint32(len(entries)),
		Entries: apiEntries,
	}

	reply := &osvbng_srg.OsvbngSrgSendGarpReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("send SRG GARP: %w", err)
	}
	if reply.Retval != 0 {
		return fmt.Errorf("send SRG GARP failed: retval=%d", reply.Retval)
	}

	v.logger.Debug("Sent SRG GARP/NA", "name", srgName, "entries", len(entries))
	return nil
}

func (v *VPP) GetSRGCounters(srgName string) ([]southbound.SRGCounters, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &osvbng_srg.OsvbngSrgCounterDump{
		SrgName: srgName,
	}

	var results []southbound.SRGCounters
	multi := ch.SendMultiRequest(req)
	for {
		details := &osvbng_srg.OsvbngSrgCounterDetails{}
		stop, err := multi.ReceiveReply(details)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("receive SRG counter details: %w", err)
		}
		results = append(results, southbound.SRGCounters{
			SRGName:    details.Counters.SrgName,
			GarpSent:   details.Counters.GarpSent,
			NaSent:     details.Counters.NaSent,
			MacAdds:    details.Counters.MacAdds,
			MacRemoves: details.Counters.MacRemoves,
		})
	}

	return results, nil
}

func (v *VPP) SRGPluginAvailable() bool {
	_, err := v.GetSRGCounters("")
	return err == nil
}
