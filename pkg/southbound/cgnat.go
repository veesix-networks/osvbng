// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package southbound

import (
	"net"
)

type CGNATMapping struct {
	PoolID         uint32
	SwIfIndex      uint32
	InsideIP       net.IP
	InsideVRFID    uint32
	OutsideIP      net.IP
	PortBlockStart uint16
	PortBlockEnd   uint16
	EnableFeature  bool
	ActiveSessions uint32
}

type CGNATDataplane interface {
	CGNATPoolAddDel(poolID uint32, mode uint8, addressPooling uint8,
		filtering uint8, blockSize uint16, maxBlocksPerSub uint8,
		maxSessionsPerSub uint32, portRangeStart uint16, portRangeEnd uint16,
		portReuseTimeout uint16, algBitmask uint8, timeouts [4]uint32,
		isAdd bool) error

	CGNATPoolAddInsidePrefix(poolID uint32, prefix net.IPNet, vrfID uint32, isAdd bool) error
	CGNATPoolAddOutsideAddress(poolID uint32, prefix net.IPNet, isAdd bool) error
	CGNATSetOutsideVRF(poolID uint32, vrfTableID uint32) error
	CGNATPoolUpdate(poolID uint32, maxSessions uint32, algBitmask uint8, timeouts [4]uint32) error

	CGNATAddDelSubscriberMapping(poolID uint32, swIfIndex uint32,
		insideIP net.IP, insideVRFID uint32, outsideIP net.IP,
		portStart uint16, portEnd uint16, enableFeature bool, isAdd bool) error

	CGNATAddDelSubscriberMappingAsync(poolID uint32, swIfIndex uint32,
		insideIP net.IP, insideVRFID uint32, outsideIP net.IP,
		portStart uint16, portEnd uint16, enableFeature bool, isAdd bool,
		callback func(error))

	CGNATAddSubscriberMappingBulk(poolID uint32, mappings []CGNATMapping) error

	CGNATEnableOnSession(poolID uint32, swIfIndex uint32, isEnable bool) error
	CGNATSetOutsideInterface(swIfIndex uint32, poolID uint32, isEnable bool) error

	CGNATAddDelBypass(prefix net.IPNet, vrfID uint32, isAdd bool) error

	CGNATDumpSubscriberMappings(poolID uint32) ([]CGNATMapping, error)
}
