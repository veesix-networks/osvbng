// Copyright 2026 The osvbng Authors
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

type CGNATPoolState struct {
	PoolID             uint32
	Mode               uint8
	AddressPooling     uint8
	Filtering          uint8
	BlockSize          uint16
	MaxBlocksPerSub    uint8
	MaxSessionsPerSub  uint32
	PortRangeStart     uint16
	PortRangeEnd       uint16
	PortReuseTimeout   uint16
	ALGBitmask         uint8
	Timeouts           [4]uint32
	OutsideVRFTableID  uint32
	ActiveMappings     uint32
}

type CGNATInsidePrefixState struct {
	PoolID uint32
	Prefix net.IPNet
	VRFID  uint32
}

type CGNATOutsideAddressState struct {
	PoolID uint32
	Prefix net.IPNet
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

	// CGNATAddSubscriberMappingBulk programs N subscriber mappings under one
	// poolID. Returns a per-mapping error slice (len == len(mappings); nil entry
	// = success) and an overall error for unrecoverable framing failures only.
	// Per-mapping -116 (entry already exists) is surfaced as success.
	CGNATAddSubscriberMappingBulk(poolID uint32, mappings []CGNATMapping) ([]error, error)

	CGNATEnableOnSession(poolID uint32, swIfIndex uint32, isEnable bool) error

	CGNATAddDelBypass(prefix net.IPNet, vrfID uint32, isAdd bool) error

	CGNATDumpSubscriberMappings(poolID uint32) ([]CGNATMapping, error)
	CGNATPoolDump() ([]CGNATPoolState, error)
	CGNATPoolInsidePrefixDump(poolID uint32) ([]CGNATInsidePrefixState, error)
	CGNATPoolOutsideAddressDump(poolID uint32) ([]CGNATOutsideAddressState, error)
}
